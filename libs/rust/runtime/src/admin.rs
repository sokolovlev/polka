//! Admin HTTP server: `/healthz` (liveness: the process answers), `/readyz`
//! (readiness: dependencies answer and the service is not draining) and
//! `/version`. Metrics are added here in stage 4.

use std::future::Future;
use std::net::SocketAddr;
use std::pin::Pin;
use std::sync::Arc;
use std::time::Duration;

use axum::extract::State;
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::routing::get;
use axum::{Json, Router};
use serde_json::{json, Map, Value};

use crate::lifecycle::{Readiness, ShutdownToken};
use crate::BuildInfo;

type CheckFuture = Pin<Box<dyn Future<Output = Result<(), String>> + Send>>;

/// A readiness check for one dependency (database, Kafka, ...).
#[derive(Clone)]
pub struct Check {
    name: &'static str,
    check: Arc<dyn Fn() -> CheckFuture + Send + Sync>,
}

impl Check {
    pub fn new<F, Fut>(name: &'static str, check: F) -> Self
    where
        F: Fn() -> Fut + Send + Sync + 'static,
        Fut: Future<Output = Result<(), String>> + Send + 'static,
    {
        Self {
            name,
            check: Arc::new(move || Box::pin(check())),
        }
    }
}

const CHECK_TIMEOUT: Duration = Duration::from_secs(2);

#[derive(Clone)]
struct AdminState {
    info: BuildInfo,
    readiness: Readiness,
    checks: Arc<Vec<Check>>,
}

/// Builds the admin routes.
pub fn router(info: BuildInfo, readiness: Readiness, checks: Vec<Check>) -> Router {
    Router::new()
        .route(
            "/healthz",
            get(|| async { Json(json!({ "status": "ok" })) }),
        )
        .route("/readyz", get(readyz))
        .route("/version", get(version))
        .with_state(AdminState {
            info,
            readiness,
            checks: Arc::new(checks),
        })
}

async fn readyz(State(st): State<AdminState>) -> Response {
    if st.readiness.draining() {
        return (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(json!({ "status": "draining" })),
        )
            .into_response();
    }
    let mut failed = Map::new();
    for c in st.checks.iter() {
        let res = tokio::time::timeout(CHECK_TIMEOUT, (c.check)()).await;
        let err = match res {
            Ok(Ok(())) => continue,
            Ok(Err(e)) => e,
            Err(_) => "timeout".to_owned(),
        };
        failed.insert(c.name.to_owned(), Value::String(err));
    }
    if failed.is_empty() {
        Json(json!({ "status": "ok" })).into_response()
    } else {
        (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(json!({ "status": "unavailable", "failed": failed })),
        )
            .into_response()
    }
}

async fn version(State(st): State<AdminState>) -> Json<BuildInfo> {
    Json(st.info)
}

/// Serves `router` on `addr` until the shutdown token fires.
pub async fn serve(addr: SocketAddr, router: Router, token: ShutdownToken) -> std::io::Result<()> {
    let listener = tokio::net::TcpListener::bind(addr).await?;
    tracing::info!(%addr, "admin server listening");
    axum::serve(listener, router)
        .with_graceful_shutdown(token.wait())
        .await
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::Body;
    use axum::http::Request;
    use http_body_util::BodyExt;
    use tower::ServiceExt;

    // Explicit values: build_info! reads POLKA_VERSION at compile time, and
    // tests must not depend on how they were built.
    fn info() -> BuildInfo {
        BuildInfo {
            service: "inventory",
            version: "1.2.3",
            commit: "abc1234",
        }
    }

    async fn call(router: Router, path: &str) -> (StatusCode, Value) {
        let resp = router
            .oneshot(Request::get(path).body(Body::empty()).unwrap())
            .await
            .unwrap();
        let status = resp.status();
        let body = resp.into_body().collect().await.unwrap().to_bytes();
        (status, serde_json::from_slice(&body).unwrap())
    }

    #[tokio::test]
    async fn healthz_ignores_checks() {
        let failing = Check::new("db", || async { Err("down".to_owned()) });
        let (status, _) = call(
            router(info(), Readiness::default(), vec![failing]),
            "/healthz",
        )
        .await;
        assert_eq!(status, StatusCode::OK);
    }

    #[tokio::test]
    async fn readyz_reports_failed_checks() {
        let ok = Check::new("kafka", || async { Ok(()) });
        let failing = Check::new("db", || async { Err("down".to_owned()) });

        let (status, _) = call(
            router(info(), Readiness::default(), vec![ok.clone()]),
            "/readyz",
        )
        .await;
        assert_eq!(status, StatusCode::OK);

        let (status, body) = call(
            router(info(), Readiness::default(), vec![ok, failing]),
            "/readyz",
        )
        .await;
        assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
        assert_eq!(body["failed"], json!({ "db": "down" }));
    }

    #[tokio::test]
    async fn readyz_is_unavailable_while_draining() {
        let (readiness, _token, trigger) = crate::lifecycle::new();
        crate::lifecycle::run(trigger, async {}, Duration::from_secs(1), vec![])
            .await
            .unwrap();
        let (status, body) = call(router(info(), readiness, vec![]), "/readyz").await;
        assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
        assert_eq!(body["status"], "draining");
    }

    #[tokio::test]
    async fn version_returns_build_info() {
        let (status, body) = call(router(info(), Readiness::default(), vec![]), "/version").await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(body["service"], "inventory");
        assert_eq!(body["version"], "1.2.3");
        assert_eq!(body["commit"], "abc1234");
    }
}

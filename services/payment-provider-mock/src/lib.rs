//! Fake acquiring provider (spec 4.9). Stage 0 serves only the service
//! routes; payments, refunds, the payment page and the failure settings are
//! added in stage 3.

use std::net::SocketAddr;

use axum::Router;
use polka_runtime::config::Validate;
use polka_runtime::lifecycle::Readiness;
use polka_runtime::{admin, BuildInfo};
use serde::Deserialize;

/// Mock configuration. Health routes share the public port: the mock is an
/// external system stand-in, not a Polka service with an admin port.
#[derive(Debug, Clone, Deserialize)]
pub struct MockConfig {
    #[serde(default = "default_http_addr")]
    pub http_addr: SocketAddr,
}

impl Validate for MockConfig {
    fn validate(&self) -> Result<(), String> {
        Ok(())
    }
}

fn default_http_addr() -> SocketAddr {
    SocketAddr::from(([0, 0, 0, 0], 8080))
}

/// All HTTP routes of the mock.
pub fn router(info: BuildInfo, readiness: Readiness) -> Router {
    admin::router(info, readiness, vec![])
}

#[cfg(test)]
mod tests {
    #![allow(clippy::unwrap_used)]

    use super::*;
    use axum::body::Body;
    use axum::http::{Request, StatusCode};
    use tower::ServiceExt;

    #[tokio::test]
    async fn serves_health_routes() {
        let app = router(
            polka_runtime::build_info!("payment-provider-mock"),
            Readiness::default(),
        );
        for path in ["/healthz", "/readyz", "/version"] {
            let resp = app
                .clone()
                .oneshot(Request::get(path).body(Body::empty()).unwrap())
                .await
                .unwrap();
            assert_eq!(resp.status(), StatusCode::OK, "{path}");
        }
    }
}

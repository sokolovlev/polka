//! Wires the common parts of a Rust gRPC service: config, logging, gRPC
//! server with health, admin HTTP server and graceful shutdown. A service's
//! `main` stays a few lines until it gains dependencies (database, Kafka).

use std::future::Future;
use std::net::SocketAddr;
use std::time::Duration;

use tokio::net::TcpListener;

use serde::Deserialize;

use crate::admin;
use crate::config::{self, Common, Validate};
use crate::grpc::{self, Health};
use crate::lifecycle::{self, Component};
use crate::{logging, probe, BuildInfo};

/// gRPC listen address (`GRPC_ADDR`, default `0.0.0.0:9090`).
#[derive(Debug, Clone, Deserialize)]
pub struct GrpcConfig {
    #[serde(default = "default_grpc_addr")]
    pub grpc_addr: SocketAddr,
}

impl Validate for GrpcConfig {
    fn validate(&self) -> Result<(), String> {
        Ok(())
    }
}

fn default_grpc_addr() -> SocketAddr {
    SocketAddr::from(([0, 0, 0, 0], 9090))
}

/// Runs a gRPC service and returns the process exit code. `add` registers
/// the service implementations on the health builder; `services` are their
/// full names, reported NOT_SERVING on shutdown.
pub async fn main_grpc<F, Fut>(info: BuildInfo, services: Vec<&'static str>, add: F) -> i32
where
    F: FnOnce(Health) -> Fut,
    Fut: Future<Output = Health>,
{
    let common: Common = match config::load() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("{e}");
            return 2;
        }
    };
    if probe::is_healthcheck(&std::env::args().collect::<Vec<_>>()) {
        return probe::run(common.admin_addr).await;
    }
    let grpc_cfg: GrpcConfig = match config::load() {
        Ok(c) => c,
        Err(e) => {
            eprintln!("{e}");
            return 2;
        }
    };
    logging::init(&common.log_level);

    match run_grpc(info, common, grpc_cfg, services, add, lifecycle::signal()).await {
        Ok(()) => 0,
        Err(e) => {
            tracing::error!(error = %e, "service stopped with error");
            1
        }
    }
}

/// Error from [`run_grpc`].
#[derive(Debug, thiserror::Error)]
pub enum ServiceError {
    #[error(transparent)]
    Grpc(#[from] grpc::GrpcError),
    #[error("admin server: {0}")]
    Admin(std::io::Error),
    #[error(transparent)]
    Lifecycle(#[from] lifecycle::LifecycleError),
}

/// Binds the configured addresses and serves until `stop` resolves.
pub async fn run_grpc<F, Fut, S>(
    info: BuildInfo,
    common: Common,
    grpc_cfg: GrpcConfig,
    services: Vec<&'static str>,
    add: F,
    stop: S,
) -> Result<(), ServiceError>
where
    F: FnOnce(Health) -> Fut,
    Fut: Future<Output = Health>,
    S: Future<Output = ()>,
{
    let listeners = Listeners {
        grpc: grpc::bind(grpc_cfg.grpc_addr).await?,
        admin: TcpListener::bind(common.admin_addr)
            .await
            .map_err(ServiceError::Admin)?,
    };
    serve_grpc(
        info,
        common.shutdown_timeout(),
        listeners,
        services,
        add,
        stop,
    )
    .await
}

/// Already bound sockets. Tests bind `127.0.0.1:0` themselves and pass the
/// listeners in, so there is no window in which another process can take the
/// port (the race of "find a free port, close it, bind it again").
pub struct Listeners {
    pub grpc: TcpListener,
    pub admin: TcpListener,
}

/// Serves on the given listeners until `stop` resolves.
pub async fn serve_grpc<F, Fut, S>(
    info: BuildInfo,
    shutdown_timeout: Duration,
    listeners: Listeners,
    services: Vec<&'static str>,
    add: F,
    stop: S,
) -> Result<(), ServiceError>
where
    F: FnOnce(Health) -> Fut,
    Fut: Future<Output = Health>,
    S: Future<Output = ()>,
{
    let (readiness, token, trigger) = lifecycle::new();
    let (routes, reporter) = add(Health::new()).await.into_parts();
    tracing::info!(
        grpc = ?listeners.grpc.local_addr().ok(),
        admin = ?listeners.admin.local_addr().ok(),
        version = info.version,
        commit = info.commit,
        "listening"
    );

    let router = admin::router(info, readiness, vec![]);
    let admin_token = token.clone();
    let admin_listener = listeners.admin;
    let components = vec![
        Component::new(
            "grpc",
            grpc::serve(listeners.grpc, routes, reporter, services, token),
        ),
        Component::new("admin", async move {
            axum::serve(admin_listener, router)
                .with_graceful_shutdown(admin_token.wait())
                .await
        }),
    ];
    lifecycle::run(trigger, stop, shutdown_timeout, components).await?;
    Ok(())
}

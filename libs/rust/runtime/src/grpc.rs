//! gRPC server with the standard health service (grpc.health.v1, used by the
//! Kubernetes gRPC probe, spec 6.1, 9.3).

use std::net::SocketAddr;

use tokio::net::TcpListener;
use tokio_stream::wrappers::TcpListenerStream;
use tonic::server::NamedService;
use tonic::service::Routes;
use tonic_health::server::HealthReporter;
use tonic_health::ServingStatus;

use crate::lifecycle::ShutdownToken;

/// Error from [`serve`].
#[derive(Debug, thiserror::Error)]
pub enum GrpcError {
    #[error("bind {addr}: {source}")]
    Bind {
        addr: SocketAddr,
        source: std::io::Error,
    },
    #[error("grpc server: {0}")]
    Transport(#[from] tonic::transport::Error),
}

/// Health service plus its reporter. Mark each service SERVING with
/// [`Health::serving`] after adding it.
pub struct Health {
    pub reporter: HealthReporter,
    routes: Routes,
}

impl Health {
    pub fn new() -> Self {
        let (reporter, service) = tonic_health::server::health_reporter();
        Self {
            reporter,
            routes: Routes::new(service),
        }
    }

    /// Adds a service and reports it SERVING.
    pub async fn serving<S>(mut self, svc: S) -> Self
    where
        S: tonic::codegen::Service<
                tonic::codegen::http::Request<tonic::body::Body>,
                Response = tonic::codegen::http::Response<tonic::body::Body>,
                Error = std::convert::Infallible,
            > + NamedService
            + Clone
            + Send
            + Sync
            + 'static,
        S::Future: Send + 'static,
    {
        self.reporter.set_serving::<S>().await;
        self.routes = self.routes.add_service(svc);
        self
    }

    /// Splits into the routes to serve and the reporter (to drain on shutdown).
    pub fn into_parts(self) -> (Routes, HealthReporter) {
        (self.routes, self.reporter)
    }
}

impl Default for Health {
    fn default() -> Self {
        Self::new()
    }
}

/// Binds `addr` (so a busy port fails at startup) and returns the listener.
pub async fn bind(addr: SocketAddr) -> Result<TcpListener, GrpcError> {
    TcpListener::bind(addr)
        .await
        .map_err(|source| GrpcError::Bind { addr, source })
}

/// Serves until the token fires. Health turns NOT_SERVING first (overall
/// status "" and every named service), then in-flight calls finish.
pub async fn serve(
    listener: TcpListener,
    routes: Routes,
    reporter: HealthReporter,
    services: Vec<&'static str>,
    token: ShutdownToken,
) -> Result<(), GrpcError> {
    let shutdown = async move {
        token.wait().await;
        reporter
            .set_service_status("", ServingStatus::NotServing)
            .await;
        for name in services {
            reporter
                .set_service_status(name, ServingStatus::NotServing)
                .await;
        }
    };
    tonic::transport::Server::builder()
        .add_routes(routes)
        .serve_with_incoming_shutdown(TcpListenerStream::new(listener), shutdown)
        .await?;
    Ok(())
}

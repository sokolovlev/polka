//! Shared runtime for Polka's Rust services: configuration from environment
//! variables (spec 8.4), JSON logging (9.4), the admin HTTP server with
//! `/healthz`, `/readyz`, `/version` (6.5, 9.1, 9.3) and graceful shutdown
//! (8.2).

pub mod admin;
pub mod config;
pub mod grpc;
pub mod lifecycle;
pub mod logging;
pub mod probe;
pub mod service;

/// Version and commit baked into the binary (spec 9.1).
#[derive(Debug, Clone, serde::Serialize, PartialEq, Eq)]
pub struct BuildInfo {
    pub service: &'static str,
    pub version: &'static str,
    pub commit: &'static str,
}

/// Builds a [`BuildInfo`] from `POLKA_VERSION` and `POLKA_COMMIT`, read at
/// compile time of the calling crate (the Docker build passes them as build
/// args). Falls back to `dev` / `unknown`.
#[macro_export]
macro_rules! build_info {
    ($service:expr) => {
        $crate::BuildInfo {
            service: $service,
            version: match option_env!("POLKA_VERSION") {
                Some(v) => v,
                None => "dev",
            },
            commit: match option_env!("POLKA_COMMIT") {
                Some(c) => c,
                None => "unknown",
            },
        }
    };
}

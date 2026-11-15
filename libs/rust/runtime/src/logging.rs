//! Structured JSON logs to stdout (spec 9.4).

use tracing_subscriber::{fmt, EnvFilter};

/// Initialises the global JSON logger. `filter` is a `tracing` filter string
/// such as `info`; an invalid one falls back to `info`. Calling it twice is a
/// no-op.
pub fn init(filter: &str) {
    let filter = EnvFilter::try_new(filter).unwrap_or_else(|_| EnvFilter::new("info"));
    let _ = fmt()
        .json()
        .with_env_filter(filter)
        .with_current_span(false)
        .with_target(true)
        .try_init();
}

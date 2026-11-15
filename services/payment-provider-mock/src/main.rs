use payment_provider_mock::{router, MockConfig};
use polka_runtime::config::{self, Common};
use polka_runtime::lifecycle::{self, Component};
use polka_runtime::{logging, probe};

#[tokio::main]
async fn main() {
    std::process::exit(run().await);
}

async fn run() -> i32 {
    let (common, cfg) = match (config::load::<Common>(), config::load::<MockConfig>()) {
        (Ok(c), Ok(m)) => (c, m),
        (Err(e), _) | (_, Err(e)) => {
            eprintln!("{e}");
            return 2;
        }
    };
    if probe::is_healthcheck(&std::env::args().collect::<Vec<_>>()) {
        return probe::run(cfg.http_addr).await;
    }
    logging::init(&common.log_level);

    let (readiness, token, trigger) = lifecycle::new();
    let info = polka_runtime::build_info!("payment-provider-mock");
    let listener = match tokio::net::TcpListener::bind(cfg.http_addr).await {
        Ok(l) => l,
        Err(e) => {
            tracing::error!(error = %e, addr = %cfg.http_addr, "bind");
            return 1;
        }
    };
    tracing::info!(http = %cfg.http_addr, version = info.version, commit = info.commit, "listening");

    let app = router(info, readiness);
    let http = Component::new("http", async move {
        axum::serve(listener, app)
            .with_graceful_shutdown(token.wait())
            .await
    });
    match lifecycle::run(
        trigger,
        lifecycle::signal(),
        common.shutdown_timeout(),
        vec![http],
    )
    .await
    {
        Ok(()) => 0,
        Err(e) => {
            tracing::error!(error = %e, "service stopped with error");
            1
        }
    }
}

//! Graceful shutdown (spec 8.2).
//!
//! On SIGTERM (or when any component fails):
//! 1. readiness flips to "not ready", so Kubernetes stops routing traffic;
//! 2. the [`ShutdownToken`] fires: servers stop accepting and finish
//!    in-flight requests (`with_graceful_shutdown`, `serve_with_shutdown`);
//! 3. [`run`] waits for all components, at most `timeout`.

use std::future::Future;
use std::pin::Pin;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Duration;

use tokio::sync::watch;
use tokio::task::JoinSet;

/// Error from a component or from shutdown itself.
#[derive(Debug, thiserror::Error)]
pub enum LifecycleError {
    #[error("component {name} failed: {message}")]
    Component { name: &'static str, message: String },
    #[error("component {0} panicked")]
    Panicked(String),
    #[error("shutdown did not finish within {0:?}")]
    Timeout(Duration),
}

/// Shared by `/readyz` and the gRPC health service.
#[derive(Debug, Clone, Default)]
pub struct Readiness(Arc<AtomicBool>);

impl Readiness {
    pub fn draining(&self) -> bool {
        self.0.load(Ordering::Relaxed)
    }

    fn start_draining(&self) {
        self.0.store(true, Ordering::Relaxed);
    }
}

/// Resolves when shutdown starts. Cheap to clone.
#[derive(Debug, Clone)]
pub struct ShutdownToken(watch::Receiver<bool>);

impl ShutdownToken {
    /// Waits until shutdown starts.
    pub async fn wait(mut self) {
        // An Err means the sender is gone, which also means shutdown.
        let _ = self.0.wait_for(|stopping| *stopping).await;
    }
}

/// A named long-running component.
pub struct Component {
    name: &'static str,
    fut: Pin<Box<dyn Future<Output = Result<(), String>> + Send>>,
}

impl Component {
    /// `fut` must finish on its own after the shutdown token fires.
    pub fn new<F, E>(name: &'static str, fut: F) -> Self
    where
        F: Future<Output = Result<(), E>> + Send + 'static,
        E: std::fmt::Display,
    {
        Self {
            name,
            fut: Box::pin(async move { fut.await.map_err(|e| e.to_string()) }),
        }
    }
}

/// Creates the shared readiness flag and shutdown token, and the trigger
/// passed to [`run`].
pub fn new() -> (Readiness, ShutdownToken, Trigger) {
    let (tx, rx) = watch::channel(false);
    let readiness = Readiness::default();
    (
        readiness.clone(),
        ShutdownToken(rx),
        Trigger { tx, readiness },
    )
}

/// Starts shutdown. Owned by [`run`].
#[derive(Debug)]
pub struct Trigger {
    tx: watch::Sender<bool>,
    readiness: Readiness,
}

impl Trigger {
    fn fire(&self) {
        self.readiness.start_draining();
        let _ = self.tx.send(true);
    }
}

/// Waits for SIGTERM or Ctrl-C.
pub async fn signal() {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{signal, SignalKind};
        match signal(SignalKind::terminate()) {
            Ok(mut term) => {
                tokio::select! {
                    _ = term.recv() => {}
                    _ = tokio::signal::ctrl_c() => {}
                }
            }
            Err(_) => {
                let _ = tokio::signal::ctrl_c().await;
            }
        }
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
    }
}

/// Runs components until `stop` resolves (normally [`signal`]) or one of them
/// fails, then shuts all of them down within `timeout`. Returns the first
/// component error, if any.
pub async fn run<S>(
    trigger: Trigger,
    stop: S,
    timeout: Duration,
    components: Vec<Component>,
) -> Result<(), LifecycleError>
where
    S: Future<Output = ()>,
{
    let mut set = JoinSet::new();
    for c in components {
        let name = c.name;
        set.spawn(async move { (name, c.fut.await) });
    }

    let mut first_error = None;
    tokio::select! {
        () = stop => tracing::info!("shutdown signal received"),
        Some(res) = set.join_next() => {
            first_error = to_error(res).err();
            if let Some(e) = &first_error {
                tracing::error!(error = %e, "component stopped, shutting down");
            }
        }
    }

    trigger.fire();
    tracing::info!(?timeout, "shutting down");

    let drain = async {
        while let Some(res) = set.join_next().await {
            if let Err(e) = to_error(res) {
                tracing::error!(error = %e, "component failed during shutdown");
                first_error.get_or_insert(e);
            }
        }
    };
    if tokio::time::timeout(timeout, drain).await.is_err() {
        set.abort_all();
        return Err(LifecycleError::Timeout(timeout));
    }
    first_error.map_or(Ok(()), Err)
}

fn to_error(
    res: Result<(&'static str, Result<(), String>), tokio::task::JoinError>,
) -> Result<(), LifecycleError> {
    match res {
        Ok((_, Ok(()))) => Ok(()),
        Ok((name, Err(message))) => Err(LifecycleError::Component { name, message }),
        Err(e) => Err(LifecycleError::Panicked(e.to_string())),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::AtomicUsize;

    fn stopping_component(
        name: &'static str,
        token: ShutdownToken,
        stopped: Arc<AtomicUsize>,
    ) -> Component {
        Component::new(name, async move {
            token.wait().await;
            stopped.fetch_add(1, Ordering::SeqCst);
            Ok::<(), String>(())
        })
    }

    #[tokio::test]
    async fn stop_signal_shuts_down_all_components() {
        let (ready, token, trigger) = new();
        let stopped = Arc::new(AtomicUsize::new(0));
        let comps = vec![
            stopping_component("a", token.clone(), stopped.clone()),
            stopping_component("b", token, stopped.clone()),
        ];
        assert!(!ready.draining());

        run(trigger, async {}, Duration::from_secs(1), comps)
            .await
            .unwrap();

        assert!(ready.draining());
        assert_eq!(stopped.load(Ordering::SeqCst), 2);
    }

    #[tokio::test]
    async fn failing_component_stops_the_others() {
        let (_ready, token, trigger) = new();
        let stopped = Arc::new(AtomicUsize::new(0));
        let comps = vec![
            Component::new("failing", async { Err::<(), _>("boom") }),
            stopping_component("healthy", token, stopped.clone()),
        ];

        let err = run(
            trigger,
            std::future::pending(),
            Duration::from_secs(1),
            comps,
        )
        .await
        .unwrap_err();

        assert!(err.to_string().contains("failing failed: boom"), "{err}");
        assert_eq!(stopped.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn shutdown_timeout_is_enforced() {
        let (_ready, _token, trigger) = new();
        let comps = vec![Component::new("stuck", async {
            std::future::pending::<()>().await;
            Ok::<(), String>(())
        })];

        let err = run(trigger, async {}, Duration::from_millis(50), comps)
            .await
            .unwrap_err();

        assert!(matches!(err, LifecycleError::Timeout(_)), "{err}");
    }
}

//! `healthcheck` subcommand: the service binary probes its own `/readyz`.
//! Distroless images have no shell or curl, so docker compose healthchecks
//! run `/service healthcheck`.

use std::net::{Ipv4Addr, SocketAddr};
use std::time::Duration;

use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

/// The subcommand name.
pub const HEALTHCHECK_ARG: &str = "healthcheck";

/// Whether the process was started as a probe.
pub fn is_healthcheck(args: &[String]) -> bool {
    args.get(1).is_some_and(|a| a == HEALTHCHECK_ARG)
}

/// GETs `/readyz` on the local port of `addr`; Ok if the status is 200.
pub async fn probe(addr: SocketAddr) -> Result<(), String> {
    let target = SocketAddr::from((Ipv4Addr::LOCALHOST, addr.port()));
    let fut = async {
        let mut stream = TcpStream::connect(target)
            .await
            .map_err(|e| format!("connect {target}: {e}"))?;
        stream
            .write_all(b"GET /readyz HTTP/1.0\r\nHost: localhost\r\n\r\n")
            .await
            .map_err(|e| e.to_string())?;
        let mut buf = Vec::with_capacity(256);
        stream
            .read_to_end(&mut buf)
            .await
            .map_err(|e| e.to_string())?;
        let head = String::from_utf8_lossy(&buf);
        let status = head.lines().next().unwrap_or_default().to_owned();
        if status.split_whitespace().nth(1) == Some("200") {
            Ok(())
        } else {
            Err(format!("readyz: {status}"))
        }
    };
    tokio::time::timeout(Duration::from_secs(3), fut)
        .await
        .map_err(|_| "readyz: timeout".to_owned())?
}

/// Runs the probe and returns the process exit code.
pub async fn run(addr: SocketAddr) -> i32 {
    match probe(addr).await {
        Ok(()) => 0,
        Err(e) => {
            eprintln!("{e}");
            1
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::admin;
    use crate::lifecycle::Readiness;

    #[test]
    fn detects_subcommand() {
        let args = |v: &[&str]| v.iter().map(|s| (*s).to_owned()).collect::<Vec<_>>();
        assert!(is_healthcheck(&args(&["/service", "healthcheck"])));
        assert!(!is_healthcheck(&args(&["/service"])));
    }

    #[tokio::test]
    async fn probes_admin_server() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let router = admin::router(crate::build_info!("t"), Readiness::default(), vec![]);
        tokio::spawn(async move { axum::serve(listener, router).await });

        // services bind 0.0.0.0; the probe still targets localhost
        let any = SocketAddr::from(([0, 0, 0, 0], addr.port()));
        probe(any).await.unwrap();
        assert_eq!(run(any).await, 0);
    }

    #[tokio::test]
    async fn fails_when_nothing_listens() {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        drop(listener);
        assert!(probe(addr).await.is_err());
    }
}

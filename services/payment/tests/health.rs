//! Starts the service on free ports and checks gRPC health and shutdown.

#![allow(clippy::unwrap_used)]

use std::time::Duration;

use polka_runtime::service::{serve_grpc, Listeners};
use tokio::net::TcpListener;
use tonic_health::pb::health_check_response::ServingStatus;
use tonic_health::pb::health_client::HealthClient;
use tonic_health::pb::HealthCheckRequest;

use payment::grpc;

#[tokio::test]
async fn serves_health_and_stops_on_signal() {
    // Bound here and handed over: no free-port race.
    let grpc = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let admin = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let grpc_addr = grpc.local_addr().unwrap();
    let admin_addr = admin.local_addr().unwrap();
    let (stop_tx, stop_rx) = tokio::sync::oneshot::channel::<()>();

    let svc = tokio::spawn(serve_grpc(
        polka_runtime::build_info!("payment"),
        Duration::from_secs(5),
        Listeners { grpc, admin },
        vec![grpc::SERVICE_NAME],
        |h| h.serving(grpc::server()),
        async {
            let _ = stop_rx.await;
        },
    ));

    let mut client = None;
    for _ in 0..50 {
        let endpoint =
            tonic::transport::Endpoint::from_shared(format!("http://{grpc_addr}")).unwrap();
        if let Ok(channel) = endpoint.connect().await {
            client = Some(HealthClient::new(channel));
            break;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    let mut client = client.unwrap();
    let status = client
        .check(HealthCheckRequest {
            service: grpc::SERVICE_NAME.to_owned(),
        })
        .await
        .unwrap()
        .into_inner()
        .status;
    assert_eq!(status, ServingStatus::Serving as i32);
    assert_eq!(grpc::SERVICE_NAME, "polka.payment.v1.PaymentService");
    polka_runtime::probe::probe(admin_addr).await.unwrap();

    stop_tx.send(()).unwrap();
    tokio::time::timeout(Duration::from_secs(5), svc)
        .await
        .unwrap()
        .unwrap()
        .unwrap();
}

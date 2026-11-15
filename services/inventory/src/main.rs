use inventory::grpc;

#[tokio::main]
async fn main() {
    let info = polka_runtime::build_info!("inventory");
    let code = polka_runtime::service::main_grpc(info, vec![grpc::SERVICE_NAME], |health| {
        health.serving(grpc::server())
    })
    .await;
    std::process::exit(code);
}

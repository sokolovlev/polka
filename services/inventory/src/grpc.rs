//! gRPC transport. Methods are added in the stage that needs them (spec
//! 6.2); until then the service answers only health checks.

use polka_proto::inventory::v1::inventory_service_server::{
    InventoryService, InventoryServiceServer,
};
use tonic::server::NamedService;

/// Implementation of `polka.inventory.v1.InventoryService`.
#[derive(Debug, Default, Clone, Copy)]
pub struct Service;

impl InventoryService for Service {}

/// The tonic service to register on the server.
pub fn server() -> InventoryServiceServer<Service> {
    InventoryServiceServer::new(Service)
}

/// Full service name, as reported by the health service.
pub const SERVICE_NAME: &str = <InventoryServiceServer<Service> as NamedService>::NAME;

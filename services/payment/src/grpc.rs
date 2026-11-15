//! gRPC transport. Methods are added in the stage that needs them (spec
//! 6.2); until then the service answers only health checks.

use polka_proto::payment::v1::payment_service_server::{PaymentService, PaymentServiceServer};
use tonic::server::NamedService;

/// Implementation of `polka.payment.v1.PaymentService`.
#[derive(Debug, Default, Clone, Copy)]
pub struct Service;

impl PaymentService for Service {}

/// The tonic service to register on the server.
pub fn server() -> PaymentServiceServer<Service> {
    PaymentServiceServer::new(Service)
}

/// Full service name, as reported by the health service.
pub const SERVICE_NAME: &str = <PaymentServiceServer<Service> as NamedService>::NAME;

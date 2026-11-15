//! Generated protobuf messages and gRPC stubs for Polka.
//!
//! The code under `src/gen` is produced by `make gen` from `proto/`; do not
//! edit it by hand. Packages are re-exported under their proto names, e.g.
//! `polka_proto::common::v1::Money`.

#[allow(clippy::all, clippy::pedantic, missing_docs)]
mod gen;

pub use gen::polka::*;

use prost::{Message, Name};
use prost_types::Any;

/// Type URL prefix used by Go (`anypb`) and TS (ts-proto).
pub const TYPE_URL_PREFIX: &str = "type.googleapis.com/";

/// Packs a message into `Any` with the same type URL as Go and TS
/// (`type.googleapis.com/<full name>`).
///
/// Use this instead of `Any::from_msg`: protoc-gen-prost has no option for the
/// type URL domain, so the generated `Name::type_url()` is `/<full name>` and
/// would not match what other services write. Decoding with `Any::to_msg` is
/// fine: it compares only the full name, ignoring the domain.
pub fn pack<M: Message + Name>(msg: &M) -> Any {
    Any {
        type_url: format!("{TYPE_URL_PREFIX}{}", M::full_name()),
        value: msg.encode_to_vec(),
    }
}

//! Checks the shared wire-format fixture in proto/testdata (also checked by
//! the Go and TS tests), so all three generated libraries agree byte for byte.

#![allow(clippy::unwrap_used)] // test helpers outside #[test] fns

use polka_proto::common::v1::{Envelope, Money};
use prost::Message;

const GOLDEN: &str = include_str!("../../../../proto/testdata/envelope_money.hex");

fn fixture() -> Envelope {
    Envelope {
        event_id: "0193a6f0-0000-7000-8000-000000000001".into(),
        event_type: "order.created".into(),
        aggregate_id: "0193a6f0-0000-7000-8000-0000000000aa".into(),
        aggregate_version: 3,
        occurred_at: Some(prost_types::Timestamp {
            seconds: 1_763_116_800,
            nanos: 500,
        }),
        correlation_id: "0193a6f0-0000-7000-8000-0000000000aa".into(),
        traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01".into(),
        payload: Some(polka_proto::pack(&Money {
            amount_minor: 129_900,
            currency: "RUB".into(),
        })),
    }
}

fn golden_bytes() -> Vec<u8> {
    hex::decode(GOLDEN.trim()).unwrap()
}

#[test]
fn encoding_matches_golden() {
    assert_eq!(hex::encode(fixture().encode_to_vec()), GOLDEN.trim());
}

#[test]
fn golden_decodes_with_payload() {
    let env = Envelope::decode(golden_bytes().as_slice()).unwrap();
    assert_eq!(env, fixture());
    let money: Money = env.payload.unwrap().to_msg().unwrap();
    assert_eq!(money.amount_minor, 129_900);
    assert_eq!(money.currency, "RUB");
}

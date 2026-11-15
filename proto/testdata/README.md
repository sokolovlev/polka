# Golden wire-format fixtures

`envelope_money.hex` is the deterministic protobuf encoding of one `Envelope`
carrying a `Money` payload in `Any`. The Go, Rust and TS tests build the same
message and compare their encoding with this file, and decode the file back.
This proves the three generated libraries agree on the wire format.

Regenerate (only if the fixture message itself changes):

    cd libs/go && go test ./internal/protogolden -update

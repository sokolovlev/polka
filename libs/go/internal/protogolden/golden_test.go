// Package protogolden checks that the generated Go code produces the shared
// wire-format fixture in proto/testdata (also checked by the Rust and TS
// tests). It lives outside libs/go/proto because `buf generate` cleans that
// directory.
package protogolden

import (
	"encoding/hex"
	"flag"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/sokolovlev/polka/libs/go/proto/polka/common/v1"
)

var update = flag.Bool("update", false, "rewrite the golden file")

const goldenPath = "../../../../proto/testdata/envelope_money.hex"

func fixture(t *testing.T) *commonv1.Envelope {
	t.Helper()
	payload, err := anypb.New(&commonv1.Money{AmountMinor: 129900, Currency: "RUB"})
	if err != nil {
		t.Fatal(err)
	}
	return &commonv1.Envelope{
		EventId:          "0193a6f0-0000-7000-8000-000000000001",
		EventType:        "order.created",
		AggregateId:      "0193a6f0-0000-7000-8000-0000000000aa",
		AggregateVersion: 3,
		OccurredAt:       &timestamppb.Timestamp{Seconds: 1763116800, Nanos: 500},
		CorrelationId:    "0193a6f0-0000-7000-8000-0000000000aa",
		Traceparent:      "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		Payload:          payload,
	}
}

func readGolden(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEnvelopeEncodingMatchesGolden(t *testing.T) {
	got, err := proto.MarshalOptions{Deterministic: true}.Marshal(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(goldenPath, []byte(hex.EncodeToString(got)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if want := readGolden(t); hex.EncodeToString(got) != hex.EncodeToString(want) {
		t.Fatalf("encoding differs from golden file\n got: %x\nwant: %x", got, want)
	}
}

func TestEnvelopeGoldenDecodes(t *testing.T) {
	var env commonv1.Envelope
	if err := proto.Unmarshal(readGolden(t), &env); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(&env, fixture(t)) {
		t.Fatalf("decoded envelope differs from fixture: %v", &env)
	}
	var money commonv1.Money
	if err := env.GetPayload().UnmarshalTo(&money); err != nil {
		t.Fatal(err)
	}
	if money.GetAmountMinor() != 129900 || money.GetCurrency() != "RUB" {
		t.Fatalf("unexpected payload: %v", &money)
	}
}

// Checks the shared wire-format fixture in proto/testdata (also checked by the
// Go and Rust tests), so all three generated libraries agree byte for byte.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { Any, Envelope, Money, TYPE_URL_PREFIX } from "../src/index.js";

const golden = readFileSync(
  fileURLToPath(new URL("../../../../proto/testdata/envelope_money.hex", import.meta.url)),
  "utf8",
).trim();

function fixture(): Envelope {
  return {
    eventId: "0193a6f0-0000-7000-8000-000000000001",
    eventType: "order.created",
    aggregateId: "0193a6f0-0000-7000-8000-0000000000aa",
    aggregateVersion: 3n,
    occurredAt: { seconds: 1763116800n, nanos: 500 },
    correlationId: "0193a6f0-0000-7000-8000-0000000000aa",
    traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
    payload: Any.fromPartial({
      typeUrl: `${TYPE_URL_PREFIX}polka.common.v1.Money`,
      value: Money.encode({ amountMinor: 129900n, currency: "RUB" }).finish(),
    }),
  };
}

describe("Envelope wire format", () => {
  it("encodes exactly the golden bytes", () => {
    expect(Buffer.from(Envelope.encode(fixture()).finish()).toString("hex")).toBe(golden);
  });

  it("decodes the golden bytes, including the Money payload", () => {
    const env = Envelope.decode(Buffer.from(golden, "hex"));
    // bytes fields decode as Uint8Array views; compare them separately
    const { payload: want, ...wantRest } = fixture();
    const { payload: got, ...gotRest } = env;
    expect(gotRest).toEqual(wantRest);
    expect(Buffer.from(got!.value).equals(Buffer.from(want!.value))).toBe(true);
    expect(env.payload?.typeUrl).toBe("type.googleapis.com/polka.common.v1.Money");
    const money = Money.decode(env.payload!.value);
    expect(money).toEqual({ amountMinor: 129900n, currency: "RUB" });
  });
});

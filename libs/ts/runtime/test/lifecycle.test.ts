import { describe, expect, it, vi } from "vitest";
import { Readiness, shutdown } from "../src/index.js";

const log = { info: vi.fn(), error: vi.fn() };

describe("shutdown", () => {
  it("drains readiness and closes components in order", async () => {
    const order: string[] = [];
    const r = new Readiness();
    const code = await shutdown(
      r,
      [
        { name: "http", close: () => Promise.resolve(void order.push("http")) },
        { name: "kafka", close: () => Promise.resolve(void order.push("kafka")) },
      ],
      1000,
      log,
    );
    expect(code).toBe(0);
    expect(r.draining).toBe(true);
    expect(order).toEqual(["http", "kafka"]);
  });

  it("returns 1 when a component fails but closes the rest", async () => {
    const closed = vi.fn(() => Promise.resolve());
    const code = await shutdown(
      new Readiness(),
      [
        { name: "bad", close: () => Promise.reject(new Error("boom")) },
        { name: "good", close: closed },
      ],
      1000,
      log,
    );
    expect(code).toBe(1);
    expect(closed).toHaveBeenCalled();
  });

  it("enforces the deadline", async () => {
    const code = await shutdown(
      new Readiness(),
      [{ name: "stuck", close: () => new Promise<void>(() => undefined) }],
      20,
      log,
    );
    expect(code).toBe(1);
  });
});

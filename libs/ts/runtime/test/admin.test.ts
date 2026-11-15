import Fastify from "fastify";
import { describe, expect, it } from "vitest";
import { buildInfo, Readiness, registerAdminRoutes } from "../src/index.js";

function app(readiness = new Readiness(), checks: Parameters<typeof registerAdminRoutes>[3] = []) {
  const a = Fastify();
  registerAdminRoutes(a, buildInfo("notification", {}), readiness, checks);
  return a;
}

const failing = {
  name: "db",
  check: () => Promise.reject(new Error("down")),
};

describe("admin routes", () => {
  it("healthz ignores dependency checks", async () => {
    const res = await app(new Readiness(), [failing]).inject("/healthz");
    expect(res.statusCode).toBe(200);
  });

  it("readyz lists failed checks", async () => {
    const ok = { name: "kafka", check: () => Promise.resolve() };
    expect((await app(new Readiness(), [ok]).inject("/readyz")).statusCode).toBe(200);

    const res = await app(new Readiness(), [ok, failing]).inject("/readyz");
    expect(res.statusCode).toBe(503);
    expect(res.json()).toEqual({ status: "unavailable", failed: { db: "down" } });
  });

  it("readyz is unavailable while draining", async () => {
    const r = new Readiness();
    r.startDraining();
    const res = await app(r).inject("/readyz");
    expect(res.statusCode).toBe(503);
    expect(res.json()).toEqual({ status: "draining" });
  });

  it("version returns build info", async () => {
    const res = await app().inject("/version");
    expect(res.json()).toMatchObject({
      service: "notification",
      version: "dev",
      commit: "unknown",
    });
  });
});

import { Readiness } from "@polka/runtime";
import { describe, expect, it } from "vitest";
import { buildApp } from "../src/app.js";
import { configSchema } from "../src/config.js";

describe("notification app", () => {
  it("serves health routes", async () => {
    const app = buildApp({ LOG_LEVEL: "error" }, new Readiness());
    for (const path of ["/healthz", "/readyz", "/version"]) {
      const res = await app.inject(path);
      expect(res.statusCode, path).toBe(200);
    }
    expect((await app.inject("/version")).json()).toMatchObject({ service: "notification" });
  });

  it("config defaults to port 8080", () => {
    expect(configSchema.parse({}).HTTP_PORT).toBe(8080);
  });
});

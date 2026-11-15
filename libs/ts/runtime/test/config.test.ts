import { describe, expect, it } from "vitest";
import { z } from "zod";
import { commonConfigSchema, ConfigError, loadConfig } from "../src/index.js";

const schema = commonConfigSchema.extend({
  DATABASE_URL: z.url(),
});

describe("loadConfig", () => {
  it("applies defaults and coerces numbers", () => {
    const cfg = loadConfig(schema, { DATABASE_URL: "postgres://x/db", SHUTDOWN_TIMEOUT_SEC: "10" });
    expect(cfg).toEqual({
      LOG_LEVEL: "info",
      SHUTDOWN_TIMEOUT_SEC: 10,
      DATABASE_URL: "postgres://x/db",
    });
  });

  it("reports every problem at once", () => {
    try {
      loadConfig(schema, { LOG_LEVEL: "verbose", SHUTDOWN_TIMEOUT_SEC: "60" });
      expect.unreachable();
    } catch (e) {
      expect(e).toBeInstanceOf(ConfigError);
      const issues = (e as ConfigError).issues.join("\n");
      expect(issues).toContain("DATABASE_URL");
      expect(issues).toContain("LOG_LEVEL");
      expect(issues).toContain("SHUTDOWN_TIMEOUT_SEC");
    }
  });
});

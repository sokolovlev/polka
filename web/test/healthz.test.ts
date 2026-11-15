import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "../app/healthz/route";

describe("GET /healthz", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("reports build info from the environment", async () => {
    vi.stubEnv("POLKA_VERSION", "1.2.3");
    vi.stubEnv("POLKA_COMMIT", "abc1234");
    const res = GET();
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({
      status: "ok",
      service: "web",
      version: "1.2.3",
      commit: "abc1234",
    });
  });

  it("falls back to dev/unknown", async () => {
    vi.stubEnv("POLKA_VERSION", undefined);
    vi.stubEnv("POLKA_COMMIT", undefined);
    expect(await GET().json()).toMatchObject({ version: "dev", commit: "unknown" });
  });
});

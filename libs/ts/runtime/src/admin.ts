import type { FastifyInstance } from "fastify";
import type { BuildInfo } from "./buildinfo.js";

/** A readiness check for one dependency; rejects or throws when unusable. */
export interface Check {
  name: string;
  check: () => Promise<void>;
}

/** Shared by /readyz; reports not-ready from the moment shutdown starts. */
export class Readiness {
  #draining = false;
  get draining(): boolean {
    return this.#draining;
  }
  startDraining(): void {
    this.#draining = true;
  }
}

const CHECK_TIMEOUT_MS = 2000;

function withTimeout(p: Promise<void>, ms: number): Promise<void> {
  let timer: NodeJS.Timeout | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new Error("timeout"));
    }, ms);
  });
  return Promise.race([p, timeout]).finally(() => {
    clearTimeout(timer);
  });
}

/**
 * Registers /healthz (liveness: never checks dependencies), /readyz and
 * /version on a Fastify instance.
 */
export function registerAdminRoutes(
  app: FastifyInstance,
  info: BuildInfo,
  readiness: Readiness,
  checks: Check[] = [],
): void {
  app.get("/healthz", () => ({ status: "ok" }));

  app.get("/readyz", async (_req, reply) => {
    if (readiness.draining) {
      return reply.code(503).send({ status: "draining" });
    }
    const failed: Record<string, string> = {};
    await Promise.all(
      checks.map(async (c) => {
        try {
          await withTimeout(c.check(), CHECK_TIMEOUT_MS);
        } catch (e) {
          failed[c.name] = e instanceof Error ? e.message : String(e);
        }
      }),
    );
    if (Object.keys(failed).length > 0) {
      return reply.code(503).send({ status: "unavailable", failed });
    }
    return { status: "ok" };
  });

  app.get("/version", () => info);
}

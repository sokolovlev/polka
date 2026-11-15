import { buildInfo, Readiness, registerAdminRoutes } from "@polka/runtime";
import Fastify, { type FastifyInstance } from "fastify";
import type { Config } from "./config.js";

/** Builds the HTTP app. Kafka consumers and WebSocket arrive in stage 4. */
export function buildApp(cfg: Pick<Config, "LOG_LEVEL">, readiness: Readiness): FastifyInstance {
  // Fastify logs JSON through pino (spec 9.4).
  const app = Fastify({
    logger: { level: cfg.LOG_LEVEL, base: { service: "notification" } },
  });
  registerAdminRoutes(app, buildInfo("notification"), readiness);
  return app;
}

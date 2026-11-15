import { z } from "zod";

/**
 * Settings every TS service has (mirrors Go config.Common and Rust Common).
 * TS services serve /healthz, /readyz and /version on their main HTTP port,
 * so there is no admin port here.
 */
export const commonConfigSchema = z.object({
  LOG_LEVEL: z.enum(["debug", "info", "warn", "error"]).default("info"),
  // Seconds to finish in-flight work after SIGTERM (spec 8.2: up to 25 s).
  SHUTDOWN_TIMEOUT_SEC: z.coerce.number().int().min(1).max(25).default(25),
});

export type CommonConfig = z.infer<typeof commonConfigSchema>;

export class ConfigError extends Error {
  constructor(public readonly issues: string[]) {
    super(`invalid config: ${issues.join("; ")}`);
    this.name = "ConfigError";
  }
}

/**
 * Validates environment variables against a zod schema at startup
 * (12-factor, spec 8.4), listing every problem at once.
 */
export function loadConfig<S extends z.ZodType>(
  schema: S,
  env: Record<string, string | undefined> = process.env,
): z.infer<S> {
  const result = schema.safeParse(env);
  if (!result.success) {
    throw new ConfigError(
      result.error.issues.map((i) => `${i.path.join(".") || "(root)"}: ${i.message}`),
    );
  }
  return result.data;
}

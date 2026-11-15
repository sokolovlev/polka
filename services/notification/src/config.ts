import { commonConfigSchema } from "@polka/runtime";
import { z } from "zod";

export const configSchema = commonConfigSchema.extend({
  // Public HTTP port: WebSocket (stage 4) and /healthz, /readyz, /version.
  HTTP_PORT: z.coerce.number().int().min(1).max(65535).default(8080),
});

export type Config = z.infer<typeof configSchema>;

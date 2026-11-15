import { loadConfig, onTerminate, Readiness, shutdown } from "@polka/runtime";
import { buildApp } from "./app.js";
import { configSchema } from "./config.js";

const cfg = loadConfig(configSchema);
const readiness = new Readiness();
const app = buildApp(cfg, readiness);

onTerminate(() => {
  void shutdown(
    readiness,
    [{ name: "http", close: () => app.close() }],
    cfg.SHUTDOWN_TIMEOUT_SEC * 1000,
    app.log,
  ).then((code) => process.exit(code));
});

await app.listen({ host: "0.0.0.0", port: cfg.HTTP_PORT });

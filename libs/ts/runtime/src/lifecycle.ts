import type { Readiness } from "./admin.js";

export interface Closable {
  name: string;
  /** Stops accepting new work and waits for in-flight work. */
  close: () => Promise<void>;
}

export interface Logger {
  info: (obj: object, msg: string) => void;
  error: (obj: object, msg: string) => void;
}

/**
 * Graceful shutdown (spec 8.2): flips readiness, then closes components in
 * the given order (e.g. HTTP server first, then Kafka consumer, then DB),
 * within one deadline. Resolves to the exit code.
 */
export async function shutdown(
  readiness: Readiness,
  components: Closable[],
  timeoutMs: number,
  log: Logger,
): Promise<number> {
  readiness.startDraining();
  log.info({ timeoutMs }, "shutting down");
  let timer: NodeJS.Timeout | undefined;
  const deadline = new Promise<"timeout">((resolve) => {
    timer = setTimeout(() => {
      resolve("timeout");
    }, timeoutMs);
  });
  const closeAll = (async () => {
    let code = 0;
    for (const c of components) {
      try {
        await c.close();
      } catch (err) {
        log.error({ err, component: c.name }, "shutdown failed");
        code = 1;
      }
    }
    return code;
  })();
  const result = await Promise.race([closeAll, deadline]);
  clearTimeout(timer);
  if (result === "timeout") {
    log.error({ timeoutMs }, "shutdown deadline exceeded");
    return 1;
  }
  return result;
}

/** Calls `onSignal` once on SIGTERM or SIGINT. */
export function onTerminate(onSignal: () => void): void {
  let fired = false;
  const handler = () => {
    if (!fired) {
      fired = true;
      onSignal();
    }
  };
  process.once("SIGTERM", handler);
  process.once("SIGINT", handler);
}

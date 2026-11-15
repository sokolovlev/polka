/** Version and commit baked into the image (spec 9.1). */
export interface BuildInfo {
  service: string;
  version: string;
  commit: string;
  node_version: string;
}

/**
 * Reads POLKA_VERSION and POLKA_COMMIT, which the Dockerfile sets from build
 * args (Node has no compile step to embed them in).
 */
export function buildInfo(
  service: string,
  env: Record<string, string | undefined> = process.env,
): BuildInfo {
  return {
    service,
    version: env.POLKA_VERSION ?? "dev",
    commit: env.POLKA_COMMIT ?? "unknown",
    node_version: process.version,
  };
}

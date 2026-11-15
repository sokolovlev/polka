// Liveness and version for the web container (spec 9.1, 9.3). The web app has
// no dependencies of its own to check, so one route serves both probes.
export const dynamic = "force-dynamic";

export function GET(): Response {
  return Response.json({
    status: "ok",
    service: "web",
    version: process.env.POLKA_VERSION ?? "dev",
    commit: process.env.POLKA_COMMIT ?? "unknown",
  });
}

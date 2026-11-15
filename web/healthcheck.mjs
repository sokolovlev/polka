// Self-probe for docker compose (distroless has no shell or curl).
const port = process.env.PORT ?? "3000";
try {
  const res = await fetch(`http://127.0.0.1:${port}/healthz`, {
    signal: AbortSignal.timeout(3000),
  });
  process.exit(res.ok ? 0 : 1);
} catch (err) {
  console.error(err);
  process.exit(1);
}

// Self-probe for docker compose: distroless images have no shell or curl.
// Exits 0 if /readyz answers 200.
const port = process.env.HTTP_PORT ?? "8080";
try {
  const res = await fetch(`http://127.0.0.1:${port}/readyz`, { signal: AbortSignal.timeout(3000) });
  process.exit(res.ok ? 0 : 1);
} catch (err) {
  console.error(err);
  process.exit(1);
}

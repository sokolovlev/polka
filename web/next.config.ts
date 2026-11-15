import path from "node:path";
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Self-contained server bundle for the Docker image (spec 9.1).
  output: "standalone",
  // The app lives in an npm workspace: trace dependencies from the repo root.
  outputFileTracingRoot: path.join(import.meta.dirname, ".."),
  poweredByHeader: false,
};

export default nextConfig;

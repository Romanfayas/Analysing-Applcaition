import type { NextConfig } from "next";

// All browser -> backend traffic goes through the authenticated server-side proxy (src/app/api/proxy).
// There is deliberately no rewrite exposing the backend directly.
const nextConfig: NextConfig = {};

export default nextConfig;

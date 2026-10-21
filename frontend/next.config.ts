import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    return [
      {
        source: '/api/v1/:path*',
        destination: `${process.env.BACKEND_API_URL || 'http://backend:8080/api/v1'}/:path*`,
      },
    ];
  },
};

export default nextConfig;

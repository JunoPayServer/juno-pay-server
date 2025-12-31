import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  async rewrites() {
    const base = (process.env.JUNO_PAY_BASE_URL ?? "http://127.0.0.1:8080").replace(/\/+$/, "");
    return [
      { source: "/admin/login", destination: `${base}/admin/login` },
      { source: "/admin/logout", destination: `${base}/admin/logout` },
      { source: "/v1/:path*", destination: `${base}/v1/:path*` },
    ];
  },
};

export default nextConfig;

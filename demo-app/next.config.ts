import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  skipTrailingSlashRedirect: true,
  transpilePackages: ["@junopayserver/widgets"],
};

export default nextConfig;


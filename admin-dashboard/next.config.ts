import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "export",
  basePath: "/admin",
  trailingSlash: true,
  transpilePackages: ["@junopayserver/widgets"],
};

export default nextConfig;

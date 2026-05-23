import type { NextConfig } from "next";

const config: NextConfig = {
  async rewrites() {
    const target = process.env.CS_API || "http://127.0.0.1:7070";
    return [{ source: "/api/:path*", destination: `${target}/api/:path*` }];
  },
};

export default config;

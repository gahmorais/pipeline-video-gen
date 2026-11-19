import type { NextConfig } from "next";

const config: NextConfig = {
  // Pacotes do monorepo são TypeScript-fonte: o Next transpila.
  transpilePackages: ["@videogen/compositions", "@videogen/video-schema"],
  reactStrictMode: true,
  poweredByHeader: false,
};

export default config;

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
	// Keep an acceptance build separate from a running development server.
	distDir: process.env.NEXT_DIST_DIR ?? ".next",
  // Isolated retained-runtime builds must not rewrite the active source config.
  typescript: { tsconfigPath: process.env.NEXT_TSCONFIG_PATH ?? "tsconfig.json" },
  poweredByHeader: false,
};

export default nextConfig;

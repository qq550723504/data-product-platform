/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
	// Keep an acceptance build separate from a running development server.
	distDir: process.env.NEXT_DIST_DIR ?? ".next",
  poweredByHeader: false,
};

export default nextConfig;

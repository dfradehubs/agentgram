import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  // Disable Next's gzip compression: it buffers streamed responses and flushes
  // them in large blocks, which breaks incremental SSE delivery (the agent's
  // streamed text would only appear once the whole message is written). Static
  // asset compression should be handled at the edge (ingress/CDN/service mesh).
  compress: false,
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "X-XSS-Protection", value: "0" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          {
            key: "Strict-Transport-Security",
            value: "max-age=31536000; includeSubDomains",
          },
          {
            key: "Permissions-Policy",
            value:
              "camera=(), microphone=(), geolocation=(), browsing-topics=()",
          },
          {
            key: "Content-Security-Policy",
            value:
              "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data: blob:; font-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'",
          },
        ],
      },
    ];
  },
};

export default nextConfig;

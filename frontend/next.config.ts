import type { NextConfig } from "next";

const isProd = process.env.NODE_ENV === "production";

// Content-Security-Policy is set per-request in src/proxy.ts (nonce +
// 'strict-dynamic'); it can't live here because each request needs a fresh
// nonce. The static headers below apply to every response, including assets.
const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
  },
];

/** HSTS только за пределами localhost (задаётся при деплое за HTTPS). */
if (isProd && process.env.NEXT_ENABLE_HSTS === "true") {
  securityHeaders.push({
    key: "Strict-Transport-Security",
    value: "max-age=63072000; includeSubDomains; preload",
  });
}

/**
 * Базовый адрес gateway для server-side прокси rewrites.
 * В контейнере фронта — внутренний хост compose (INTERNAL_API_URL),
 * в dev/браузере — NEXT_PUBLIC_API_URL (по умолчанию localhost:8080).
 */
const gatewayOrigin =
  process.env.INTERNAL_API_URL ||
  process.env.NEXT_PUBLIC_API_URL ||
  "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    // Медиа отдаётся по относительному пути /api/v1/uploads/* (см. venueMediaUrl
    // в src/lib/api.ts). Проксируем его на gateway, чтобы путь одинаково
    // работал и в браузере, и при server-side fetch оптимизатора next/image.
    return [
      {
        source: "/api/v1/uploads/:path*",
        destination: `${gatewayOrigin}/api/v1/uploads/:path*`,
      },
    ];
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: securityHeaders,
      },
      {
        source: "/",
        headers: [
          {
            key: "Cache-Control",
            value: "private, no-cache, no-store, max-age=0, must-revalidate",
          },
        ],
      },
    ];
  },
  experimental: {
    optimizePackageImports: ["lucide-react"],
  },
  images: {
    remotePatterns: [
      {
        protocol: "http",
        hostname: "localhost",
        port: "8080",
      },
      {
        protocol: "https",
        hostname: "images.unsplash.com",
      },
      // Добавь сюда свой MinIO hostname если он не localhost
    ],
  },
};

export default nextConfig;

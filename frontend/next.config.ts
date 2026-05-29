import type { NextConfig } from "next";

const isProd = process.env.NODE_ENV === "production";

// ─── Content-Security-Policy ────────────────────────────────────────────────
// Директивы составлены под реальный стек:
//   - Next.js: inline scripts с nonce (если нет — 'unsafe-inline' только dev)
//   - Яндекс.Метрика: mc.yandex.ru / mc.yandex.com
//   - Яндекс.Карты: api-maps.yandex.ru, suggest-maps.yandex.ru, geocode-maps.yandex.ru
//   - Unsplash: изображения
//   - MinIO/S3 (собственный медиа-хост): задаётся через NEXT_PUBLIC_MEDIA_HOST
// В production включи NEXT_ENABLE_CSP=true и убери 'unsafe-inline' из script-src
// когда внедришь nonce через middleware.
const mediaHost = process.env.NEXT_PUBLIC_MEDIA_HOST ?? "";

function buildCsp(nonce?: string): string {
  const nonceAttr = nonce ? `'nonce-${nonce}'` : "";
  // dev: unsafe-inline нужен для hot-reload; prod: только nonce или hash
  const scriptInline = isProd ? (nonceAttr || "") : "'unsafe-inline'";

  const directives: Record<string, string> = {
    "default-src": "'self'",
    "script-src": [
      "'self'",
      scriptInline,
      // Яндекс.Метрика
      "https://mc.yandex.ru",
      "https://mc.yandex.com",
      // Яндекс.Карты
      "https://api-maps.yandex.ru",
      "https://yandex.ru",
    ]
      .filter(Boolean)
      .join(" "),
    "style-src": "'self' 'unsafe-inline'", // Tailwind/CSS-in-JS требуют inline
    "img-src": [
      "'self'",
      "data:",
      "blob:",
      "https://images.unsplash.com",
      "https://mc.yandex.ru",
      "https://mc.yandex.com",
      // Яндекс.Карты тайлы
      "https://*.maps.yandex.net",
      "https://*.yandex.net",
      // MinIO / собственный хост медиа
      mediaHost,
      // localhost для dev
      isProd ? "" : "http://localhost:*",
    ]
      .filter(Boolean)
      .join(" "),
    "font-src": "'self' https://fonts.gstatic.com data:",
    "connect-src": [
      "'self'",
      // API gateway
      process.env.NEXT_PUBLIC_API_URL ?? "",
      // Яндекс.Карты API
      "https://api-maps.yandex.ru",
      "https://geocode-maps.yandex.ru",
      "https://suggest-maps.yandex.ru",
      "https://mc.yandex.ru",
      "https://mc.yandex.com",
      // WebSocket для чата
      process.env.NEXT_PUBLIC_WS_URL ?? "",
      isProd ? "" : "ws://localhost:* http://localhost:*",
    ]
      .filter(Boolean)
      .join(" "),
    "frame-src": "'none'",
    "object-src": "'none'",
    "base-uri": "'self'",
    "form-action": "'self'",
    "upgrade-insecure-requests": isProd ? "" : undefined,
  } as Record<string, string | undefined>;

  return Object.entries(directives)
    .filter(([, v]) => v !== undefined)
    .map(([k, v]) => (v ? `${k} ${v}` : k))
    .join("; ");
}

const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "SAMEORIGIN" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
  },
  {
    key: "Content-Security-Policy",
    value: buildCsp(),
  },
];

/** HSTS только за пределами localhost (задаётся при деплое за HTTPS). */
if (isProd && process.env.NEXT_ENABLE_HSTS === "true") {
  securityHeaders.push({
    key: "Strict-Transport-Security",
    value: "max-age=63072000; includeSubDomains; preload",
  });
}

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
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

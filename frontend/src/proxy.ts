import { NextResponse } from "next/server"
import type { NextRequest } from "next/server"

import { buildCsp } from "@/lib/csp"

// Next 16 file convention (formerly `middleware`). Generates a fresh per-request
// nonce and injects it into the CSP so Next applies it to its own scripts during
// SSR, and so we can read it via headers() to nonce the Метрика init script.
// Using a nonce forces dynamic rendering — see docs/content-security-policy.
export function proxy(request: NextRequest) {
  const nonce = Buffer.from(crypto.randomUUID()).toString("base64")
  const isDev = process.env.NODE_ENV === "development"
  // /embed/* — публичный виджет брони, встраивается в чужие сайты: CSP разрешает
  // фрейминг, а root layout по x-pathname убирает Метрику/cookie-баннер.
  const pathname = request.nextUrl.pathname
  const embeddable = pathname.startsWith("/embed")
  // upgrade-insecure-requests шлём только для HTTPS-страниц. За TLS-прокси —
  // x-forwarded-proto; напрямую (локальный докер по HTTP) — protocol запроса.
  const secure =
    (request.headers.get("x-forwarded-proto") ??
      request.nextUrl.protocol.replace(":", "")) === "https"
  const csp = buildCsp(nonce, isDev, { embeddable, secure })

  // Propagate to the app via request headers (Next extracts the nonce from the
  // CSP request header during render and applies it to framework scripts).
  const requestHeaders = new Headers(request.headers)
  requestHeaders.set("x-nonce", nonce)
  requestHeaders.set("x-pathname", pathname)
  requestHeaders.set("Content-Security-Policy", csp)

  const response = NextResponse.next({ request: { headers: requestHeaders } })
  response.headers.set("Content-Security-Policy", csp)
  return response
}

export const config = {
  matcher: [
    // Skip API routes, Next static/image assets, the favicon, and prefetches —
    // they don't need a per-request CSP nonce.
    {
      source: "/((?!api|_next/static|_next/image|favicon.ico).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
}

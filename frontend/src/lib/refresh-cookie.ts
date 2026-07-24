import type { NextRequest } from "next/server"

export const REFRESH_COOKIE = "banya_refresh"
const MAX_AGE = 60 * 60 * 24 * 30 // 30 дней

/**
 * Атрибуты httpOnly refresh-cookie — единый источник для /api/auth/set-refresh
 * и /api/auth/refresh (раньше опции дублировались в 3 местах и разъезжались).
 *
 * secure вычисляется по ФАКТИЧЕСКОМУ протоколу запроса, а не NODE_ENV: прод-бандл
 * (NODE_ENV=production) отдаётся и по http (нативное приложение грузит сайт по
 * cleartext http://<lan-ip>:3000). Браузер/WKWebView отбрасывает Secure-cookie на
 * http-соединении → refresh-cookie не сохранялась → сессия не переживала
 * перезапуск приложения. За TLS-прокси приходит x-forwarded-proto=https → Secure.
 * Та же логика, что в proxy.ts для upgrade-insecure-requests.
 */
export function refreshCookieOptions(req: NextRequest, opts?: { clear?: boolean }) {
  const proto =
    req.headers.get("x-forwarded-proto") ?? new URL(req.url).protocol.replace(":", "")
  return {
    httpOnly: true,
    secure: proto === "https",
    sameSite: "lax" as const,
    path: "/",
    maxAge: opts?.clear ? 0 : MAX_AGE,
  }
}

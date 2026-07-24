import { type NextRequest, NextResponse } from "next/server"
import { REFRESH_COOKIE, refreshCookieOptions } from "@/lib/refresh-cookie"

/**
 * POST /api/auth/set-refresh
 * Принимает { refresh_token: string } и записывает его в httpOnly cookie.
 * Клиентский JS refresh_token в localStorage больше не нужен.
 */
export async function POST(req: NextRequest) {
  const body = await req.json().catch(() => null)
  const refreshToken: string | undefined =
    body && typeof body.refresh_token === "string" ? body.refresh_token.trim() : undefined

  if (!refreshToken) {
    return NextResponse.json({ error: "missing refresh_token" }, { status: 400 })
  }

  const res = NextResponse.json({ ok: true })
  res.cookies.set(REFRESH_COOKIE, refreshToken, refreshCookieOptions(req))
  return res
}

/**
 * DELETE /api/auth/set-refresh
 * Очищает cookie при logout.
 */
export async function DELETE(req: NextRequest) {
  const res = NextResponse.json({ ok: true })
  res.cookies.set(REFRESH_COOKIE, "", refreshCookieOptions(req, { clear: true }))
  return res
}

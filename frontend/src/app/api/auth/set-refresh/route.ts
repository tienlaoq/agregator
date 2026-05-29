import { type NextRequest, NextResponse } from "next/server"

const REFRESH_COOKIE = "banya_refresh"
const COOKIE_MAX_AGE = 60 * 60 * 24 * 30 // 30 дней

/**
 * POST /api/auth/set-refresh
 * Принимает { refresh_token: string } и записывает его в httpOnly Secure cookie.
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
  res.cookies.set(REFRESH_COOKIE, refreshToken, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: COOKIE_MAX_AGE,
  })
  return res
}

/**
 * DELETE /api/auth/set-refresh
 * Очищает cookie при logout.
 */
export async function DELETE() {
  const res = NextResponse.json({ ok: true })
  res.cookies.set(REFRESH_COOKIE, "", {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 0,
  })
  return res
}

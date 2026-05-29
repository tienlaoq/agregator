import { type NextRequest, NextResponse } from "next/server"

const REFRESH_COOKIE = "banya_refresh"
const COOKIE_MAX_AGE = 60 * 60 * 24 * 30

/**
 * POST /api/auth/refresh
 * Читает refresh_token из httpOnly cookie, проксирует запрос к gateway,
 * возвращает новый access_token клиенту (в JSON, не в cookie),
 * обновляет refresh cookie если gateway выдал новый refresh_token.
 *
 * Клиентский JS хранит access_token только в памяти (Zustand), не в localStorage.
 */
export async function POST(req: NextRequest) {
  const refreshToken = req.cookies.get(REFRESH_COOKIE)?.value
  if (!refreshToken) {
    return NextResponse.json({ error: "no_refresh_token" }, { status: 401 })
  }

  const apiUrl =
    process.env.INTERNAL_API_URL ||
    process.env.NEXT_PUBLIC_API_URL ||
    "http://localhost:8080"

  let upstream: Response
  try {
    upstream = await fetch(`${apiUrl}/api/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken }),
    })
  } catch {
    return NextResponse.json({ error: "gateway_unreachable" }, { status: 502 })
  }

  if (!upstream.ok) {
    // Сбрасываем протухший cookie
    const errRes = NextResponse.json(
      { error: "refresh_failed", status: upstream.status },
      { status: upstream.status },
    )
    errRes.cookies.set(REFRESH_COOKIE, "", {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: 0,
    })
    return errRes
  }

  const data = await upstream.json()
  const res = NextResponse.json({ access_token: data.access_token })

  // Если gateway обновил refresh_token — обновляем cookie
  if (data.refresh_token) {
    res.cookies.set(REFRESH_COOKIE, data.refresh_token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: COOKIE_MAX_AGE,
    })
  }

  return res
}

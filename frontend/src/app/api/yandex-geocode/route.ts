import { NextRequest, NextResponse } from "next/server"
import { RateLimiter, getClientIp } from "@/lib/rate-limit"

/** HTTP Геокодер 1 (тот же ключ, что и для JS API, если в кабинете подключён «HTTP Геокодер»). */
const GEOCODE_BASE = "https://geocode-maps.yandex.ru/v1/"
const MAX_QUERY_LEN = 400

// Геокодирование — дорогая операция по квоте Яндекса: 10 запросов в минуту на IP.
const limiter = new RateLimiter({ maxRequests: 10, windowMs: 60_000 })

type GeoObject = {
  Point?: { pos?: string }
  name?: string
  description?: string
  metaDataProperty?: {
    GeocoderMetaData?: {
      text?: string
    }
  }
}

type GeocoderJSON = {
  response?: {
    GeoObjectCollection?: {
      metaDataProperty?: {
        GeocoderResponseMetaData?: { found?: string }
      }
      featureMember?: Array<{ GeoObject?: GeoObject }>
    }
  }
  statusCode?: number
  message?: string
  error?: string
}

function parseLonLatFromPos(pos: string): { lat: number; lon: number } | null {
  const parts = pos.trim().split(/\s+/).filter(Boolean)
  if (parts.length < 2) return null
  const a = parseFloat(parts[0])
  const b = parseFloat(parts[1])
  if (!Number.isFinite(a) || !Number.isFinite(b)) return null
  // Геокодер 1.x: в Point.pos порядок «долгота широта» (см. документацию Yandex).
  const lon = a
  const lat = b
  if (Math.abs(lat) > 90 || Math.abs(lon) > 180) return null
  return { lat, lon }
}

export async function GET(req: NextRequest) {
  const ip = getClientIp(req.headers)
  if (!limiter.check(ip)) {
    return NextResponse.json(
      { error: "rate_limited", message: "Слишком много запросов. Попробуйте через минуту." },
      { status: 429, headers: { "Retry-After": "60" } },
    )
  }

  const apiKey =
    process.env.YANDEX_MAPS_SERVER_API_KEY?.trim() ||
    process.env.NEXT_PUBLIC_YANDEX_MAPS_API_KEY?.trim()
  if (!apiKey) {
    return NextResponse.json(
      { error: "missing_api_key", message: "Не задан ключ API для геокодера" },
      { status: 503 },
    )
  }

  const q = req.nextUrl.searchParams.get("q")?.trim() ?? ""
  if (!q) {
    return NextResponse.json(
      { error: "empty_query", message: "Пустой запрос геокодирования" },
      { status: 400 },
    )
  }
  if (q.length > MAX_QUERY_LEN) {
    return NextResponse.json({ error: "query_too_long" }, { status: 400 })
  }

  const url = new URL(GEOCODE_BASE)
  url.searchParams.set("apikey", apiKey)
  url.searchParams.set("geocode", q)
  url.searchParams.set("lang", "ru_RU")
  url.searchParams.set("format", "json")
  url.searchParams.set("results", "1")

  let res: Response
  try {
    res = await fetch(url.toString(), {
      headers: { Accept: "application/json" },
      next: { revalidate: 0 },
    })
  } catch {
    return NextResponse.json(
      { error: "fetch_failed", message: "Не удалось связаться с геокодером" },
      { status: 502 },
    )
  }

  let data: GeocoderJSON
  try {
    data = (await res.json()) as GeocoderJSON
  } catch {
    return NextResponse.json(
      { error: "invalid_json", message: "Некорректный ответ геокодера" },
      { status: 502 },
    )
  }

  if (!res.ok) {
    const message =
      typeof data.message === "string" && data.message.trim()
        ? data.message
        : `Геокодер вернул ${res.status}`
    return NextResponse.json(
      { error: "geocoder_error", message, status: res.status },
      { status: res.status >= 400 && res.status < 600 ? res.status : 502 },
    )
  }

  const found =
    data.response?.GeoObjectCollection?.metaDataProperty?.GeocoderResponseMetaData?.found ?? "0"
  const member = data.response?.GeoObjectCollection?.featureMember?.[0]?.GeoObject
  const pos = member?.Point?.pos
  if (!pos || found === "0") {
    return NextResponse.json(
      { error: "not_found", message: "Адрес не найден" },
      { status: 404 },
    )
  }

  const coords = parseLonLatFromPos(pos)
  if (!coords) {
    return NextResponse.json(
      { error: "bad_coords", message: "Не удалось разобрать координаты ответа" },
      { status: 502 },
    )
  }

  const line =
    member?.metaDataProperty?.GeocoderMetaData?.text ||
    member?.name ||
    member?.description ||
    ""

  return NextResponse.json({
    lat: coords.lat,
    lon: coords.lon,
    line: line || undefined,
  })
}

import { NextRequest, NextResponse } from "next/server"
import { isStreetSuggestReady } from "@/lib/address-suggest-rules"
import { RateLimiter, getClientIp } from "@/lib/rate-limit"

const MAX_CITY_LEN = 120
const MAX_STREET_LEN = 200
const NOMINATIM = "https://nominatim.openstreetmap.org/search"

// 30 запросов в минуту на IP — достаточно для автодополнения при наборе.
const limiter = new RateLimiter({ maxRequests: 30, windowMs: 60_000 })

function formatHit(addr: Record<string, string | undefined>): string | null {
  const road =
    addr.road ||
    addr.pedestrian ||
    addr.footway ||
    addr.path ||
    addr.residential ||
    addr.neighbourhood
  if (!road) return null
  const house = addr.house_number
  if (house) {
    return `${road}, д. ${house}`
  }
  return road
}

function cityFromHit(addr: Record<string, string | undefined>): string {
  return (
    addr.city ||
    addr.town ||
    addr.village ||
    addr.municipality ||
    addr.hamlet ||
    ""
  )
}

function cityMatches(selected: string, hitCity: string): boolean {
  const a = selected.trim().toLowerCase()
  const b = hitCity.trim().toLowerCase()
  if (!a || !b) return true
  return b.includes(a) || a.includes(b)
}

export async function GET(req: NextRequest) {
  const ip = getClientIp(req.headers)
  if (!limiter.check(ip)) {
    return NextResponse.json(
      { suggestions: [] as string[], error: "rate_limited" },
      { status: 429, headers: { "Retry-After": "60" } },
    )
  }

  const city = req.nextUrl.searchParams.get("city")?.trim() ?? ""
  const street = req.nextUrl.searchParams.get("street")?.trim() ?? ""

  if (!city || city.length > MAX_CITY_LEN) {
    return NextResponse.json({ suggestions: [] as string[], error: "city_required" }, { status: 400 })
  }
  if (!street || street.length > MAX_STREET_LEN) {
    return NextResponse.json({ suggestions: [] as string[] })
  }

  if (!isStreetSuggestReady(street)) {
    return NextResponse.json({ suggestions: [] as string[] })
  }
  const firstSegment = street.split(",")[0]?.trim() ?? street

  const params = new URLSearchParams({
    format: "json",
    addressdetails: "1",
    limit: "12",
    countrycodes: "ru",
    "accept-language": "ru",
    street: firstSegment,
    city,
    country: "Russia",
  })

  try {
    const url = `${NOMINATIM}?${params.toString()}`
    const res = await fetch(url, {
      headers: {
        Accept: "application/json",
        "User-Agent": "AgregatorVenueApp/1.0 (address autocomplete)",
      },
      next: { revalidate: 0 },
    })

    if (!res.ok) {
      return NextResponse.json({ suggestions: [] as string[] })
    }

    const data = (await res.json()) as Array<{
      display_name?: string
      address?: Record<string, string>
    }>

    const seen = new Set<string>()
    const suggestions: string[] = []

    for (const item of data) {
      const addr = item.address || {}
      const formatted = formatHit(addr)
      if (!formatted) continue
      if (!cityMatches(city, cityFromHit(addr))) continue
      const key = formatted.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      suggestions.push(formatted)
      if (suggestions.length >= 8) break
    }

    return NextResponse.json({ suggestions })
  } catch {
    return NextResponse.json({ suggestions: [] as string[] })
  }
}

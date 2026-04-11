/**
 * Разделитель городов в `cities=` — ASCII `|`, чтобы значение не терялось при прокси/нормализации URL
 * (в отличие от U+001E в query).
 * Старые ссылки с `\u001e` по-прежнему разбираются в parseCitiesFromSearchParams.
 */
export const CITIES_HTTP_SEP = "|"

function dedupeCities(raw: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const s of raw) {
    const t = s.trim()
    if (!t) continue
    const k = t.toLowerCase()
    if (seen.has(k)) continue
    seen.add(k)
    out.push(t)
  }
  return out
}

/** Для 2+ городов — одна строка в параметр `cities`. Для 0–1 — вернуть null (используйте `city=`). */
export function packCitiesForQuery(cities: string[]): string | null {
  const list = dedupeCities(cities)
  if (list.length <= 1) return null
  return list.join(CITIES_HTTP_SEP)
}

/** Разбор из URL: сначала `cities`, иначе повторяющиеся `city`. */
export function parseCitiesFromSearchParams(sp: {
  get: (key: string) => string | null
  getAll: (key: string) => string[]
}): string[] {
  const packed = (sp.get("cities") ?? "").trim()
  if (packed) {
    const parts = packed.includes("\u001e") ? packed.split("\u001e") : packed.split(CITIES_HTTP_SEP)
    return dedupeCities(parts)
  }
  return dedupeCities(sp.getAll("city"))
}

export function parseCitiesFromStableKey(stableKey: string): string[] {
  const p = new URLSearchParams(stableKey)
  return parseCitiesFromSearchParams(p)
}

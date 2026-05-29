import { siteUrl } from "@/lib/seo-site"

function apiBase(): string {
  return (process.env.INTERNAL_API_URL || process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080").replace(
    /\/+$/,
    "",
  )
}

async function getJson<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(`${apiBase()}${path}`, {
      next: { revalidate: 3600 },
      headers: { Accept: "application/json" },
    })
    if (!res.ok) return null
    return (await res.json()) as T
  } catch {
    return null
  }
}

/** Все публичные slug заведений для sitemap (пагинация по API). */
export async function collectVenueSlugsForSitemap(): Promise<string[]> {
  const slugs = new Set<string>()
  const pageSize = 100
  let page = 1
  for (;;) {
    const data = await getJson<{ venues: { slug: string }[]; total: number }>(
      `/api/v1/venues?page=${page}&page_size=${pageSize}&sort_by=rating`,
    )
    if (!data?.venues?.length) break
    for (const v of data.venues) {
      if (v.slug) slugs.add(v.slug)
    }
    if (data.venues.length < pageSize) break
    page += 1
    if (page > 200) break
  }
  return [...slugs]
}

/** Публичные slug мастеров для sitemap. */
export async function collectMasterSlugsForSitemap(): Promise<string[]> {
  const slugs = new Set<string>()
  const pageSize = 100
  let page = 1
  for (;;) {
    const data = await getJson<{ masters: { slug: string }[]; total: number }>(
      `/api/v1/masters?page=${page}&page_size=${pageSize}`,
    )
    if (!data?.masters?.length) break
    for (const m of data.masters) {
      if (m.slug) slugs.add(m.slug)
    }
    if (data.masters.length < pageSize) break
    page += 1
    if (page > 200) break
  }
  return [...slugs]
}

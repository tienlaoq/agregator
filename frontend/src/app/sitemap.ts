/**
 * Sitemap index via Next.js generateSitemaps().
 *
 * Yields multiple sub-sitemaps, each ≤ 50 000 URLs:
 *   /sitemap/0.xml — static + city hub pages
 *   /sitemap/1.xml — venue detail pages (paginated, VENUES_PER_SITEMAP per file)
 *   /sitemap/2.xml … — master detail pages (MASTERS_PER_SITEMAP per file)
 *
 * The framework auto-generates /sitemap.xml as an index pointing to them all.
 */
import type { MetadataRoute } from "next"
import { SEO_CITY_HUBS, SEO_VENUE_KIND_SLUGS } from "@/lib/seo-city-hubs"
import { collectVenueSlugsForSitemap, collectMasterSlugsForSitemap } from "@/lib/seo-index-urls"
import { siteUrl } from "@/lib/seo-site"

// ── Partitioning constants ────────────────────────────────────────────────────

/** Shard 0 is reserved for static + city-hub pages. */
const STATIC_SHARD = 0

/**
 * Venue detail pages. With 45 000 per shard we stay safely below the 50 k limit.
 * Shard 0 is static; venue shards start at encoded id 1.
 */
const VENUES_PER_SITEMAP = 45_000

/** Masters detail pages — one shard per 45 000 slugs. */
const MASTERS_PER_SITEMAP = 45_000

// ── Shard‐id encoding ────────────────────────────────────────────────────────

/**
 * Encode (kind, page) into a single integer shard id so Next.js can route them.
 *
 * Layout:
 *   id = 0                        → static shard
 *   id = 1 … V                    → venue shards (V = ceil(venueCount / VENUES_PER_SITEMAP))
 *   id = V+1 … V+M                → master shards
 *
 * Because generateSitemaps() and the sitemap() default export must agree on the
 * mapping without sharing state, we embed the kind in the high bits:
 *   bit 31   = 0 for venues, 1 for masters
 *   bits 0-30 = zero-based page index within that kind
 *
 * This lets the default export decode the kind from a single number.
 */
const KIND_VENUES = 0
const KIND_MASTERS = 1

function encodeShardId(kind: typeof KIND_VENUES | typeof KIND_MASTERS, page: number): number {
  return (kind << 30) | page
}

function decodeShardId(id: number): { kind: typeof KIND_VENUES | typeof KIND_MASTERS; page: number } {
  const kind = ((id >>> 30) & 1) as typeof KIND_VENUES | typeof KIND_MASTERS
  const page = id & 0x3fffffff
  return { kind, page }
}

// ── generateSitemaps ──────────────────────────────────────────────────────────

export async function generateSitemaps() {
  const [venueSlugs, masterSlugs] = await Promise.all([
    collectVenueSlugsForSitemap(),
    collectMasterSlugsForSitemap(),
  ])

  const venueShards = Math.max(1, Math.ceil(venueSlugs.length / VENUES_PER_SITEMAP))
  const masterShards = Math.max(1, Math.ceil(masterSlugs.length / MASTERS_PER_SITEMAP))

  const ids: Array<{ id: number }> = [{ id: STATIC_SHARD }]

  for (let i = 0; i < venueShards; i++) {
    ids.push({ id: encodeShardId(KIND_VENUES, i) })
  }
  for (let i = 0; i < masterShards; i++) {
    ids.push({ id: encodeShardId(KIND_MASTERS, i) })
  }

  return ids
}

// ── Default export — builds one shard ────────────────────────────────────────

export default async function sitemap({
  id,
}: {
  id: number
}): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl()
  const now = new Date()

  // ── Shard 0: static pages + city hubs ──────────────────────────────────────
  if (id === STATIC_SHARD) {
    const entries: MetadataRoute.Sitemap = [
      { url: `${base}/`, lastModified: now, changeFrequency: "weekly", priority: 1 },
      { url: `${base}/venues`, lastModified: now, changeFrequency: "daily", priority: 0.95 },
      { url: `${base}/masters`, lastModified: now, changeFrequency: "daily", priority: 0.9 },
      { url: `${base}/partner`, changeFrequency: "monthly", priority: 0.7 },
      { url: `${base}/support`, changeFrequency: "monthly", priority: 0.65 },
      { url: `${base}/privacy`, changeFrequency: "yearly", priority: 0.3 },
      { url: `${base}/terms`, changeFrequency: "yearly", priority: 0.3 },
    ]

    for (const c of SEO_CITY_HUBS) {
      entries.push({
        url: `${base}/venues/city/${c.slug}`,
        lastModified: now,
        changeFrequency: "daily",
        priority: 0.88,
      })
      for (const k of SEO_VENUE_KIND_SLUGS) {
        entries.push({
          url: `${base}/venues/city/${c.slug}/${k}`,
          lastModified: now,
          changeFrequency: "daily",
          priority: 0.82,
        })
      }
    }

    return entries
  }

  // ── Venue / Master shards ───────────────────────────────────────────────────
  const { kind, page } = decodeShardId(id)

  if (kind === KIND_VENUES) {
    const slugs = await collectVenueSlugsForSitemap()
    const slice = slugs.slice(page * VENUES_PER_SITEMAP, (page + 1) * VENUES_PER_SITEMAP)
    return slice.map((slug) => ({
      url: `${base}/venues/${encodeURIComponent(slug)}`,
      lastModified: now,
      changeFrequency: "weekly" as const,
      priority: 0.75,
    }))
  }

  // KIND_MASTERS
  const slugs = await collectMasterSlugsForSitemap()
  const slice = slugs.slice(page * MASTERS_PER_SITEMAP, (page + 1) * MASTERS_PER_SITEMAP)
  return slice.map((slug) => ({
    url: `${base}/masters/${encodeURIComponent(slug)}`,
    lastModified: now,
    changeFrequency: "weekly" as const,
    priority: 0.72,
  }))
}


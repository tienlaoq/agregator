import type { Venue } from "@/lib/types"
import { siteUrl } from "@/lib/seo-site"

/** Первое фото обложки заведения (is_cover=true) или первое из gallery, или image_url. */
export function venueOgImageUrl(venue: Venue): string | undefined {
  const cover = venue.photos?.find((p) => p.is_cover)
  const first = venue.photos?.[0]
  return cover?.url || first?.url || venue.image_url || undefined
}

/** JSON-LD для карточки заведения. Рейтинг — только при ненулевом числе отзывов в каталоге. */
export function venueLocalBusinessJsonLd(venue: Venue, pathSlug: string): Record<string, unknown> {
  const base = siteUrl()
  const url = `${base}/venues/${encodeURIComponent(pathSlug)}`
  const out: Record<string, unknown> = {
    "@context": "https://schema.org",
    "@type": "HealthAndBeautyBusiness",
    name: venue.name,
    description: venue.description?.trim() || undefined,
    url,
    telephone: venue.phone?.trim() || undefined,
    address: {
      "@type": "PostalAddress",
      addressLocality: venue.city,
      streetAddress: venue.address || undefined,
      addressCountry: "RU",
    },
  }
  // image — абсолютный URL (schema.org требует абсолютных значений).
  const imgUrl = venueOgImageUrl(venue)
  if (imgUrl) {
    out.image = imgUrl.startsWith("http") ? imgUrl : `${base}${imgUrl}`
  }
  if (typeof venue.latitude === "number" && typeof venue.longitude === "number") {
    out.geo = {
      "@type": "GeoCoordinates",
      latitude: venue.latitude,
      longitude: venue.longitude,
    }
  }
  const rc = Number(venue.review_count) || 0
  const rating = Number(venue.rating)
  if (rc > 0 && Number.isFinite(rating) && rating > 0) {
    out.aggregateRating = {
      "@type": "AggregateRating",
      ratingValue: rating,
      reviewCount: rc,
      bestRating: 5,
      worstRating: 1,
    }
  }
  return out
}

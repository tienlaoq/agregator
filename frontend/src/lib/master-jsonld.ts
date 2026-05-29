import type { MasterProfile } from "@/lib/types"
import { siteUrl } from "@/lib/seo-site"

/** Первое фото обложки мастера (is_cover=true) или первое из галереи. */
export function masterOgImageUrl(profile: MasterProfile): string | undefined {
  const cover = profile.photos?.find((p) => p.is_cover)
  const first = profile.photos?.[0]
  return cover?.url || first?.url || undefined
}

export function masterPersonJsonLd(profile: MasterProfile, pathSlug: string): Record<string, unknown> {
  const base = siteUrl()
  const url = `${base}/masters/${encodeURIComponent(pathSlug)}`
  const out: Record<string, unknown> = {
    "@context": "https://schema.org",
    "@type": "Person",
    name: profile.display_name,
    description: profile.bio?.trim() || undefined,
    url,
    telephone: profile.phone?.trim() || undefined,
    jobTitle: "Мастер банных услуг",
  }
  // image — абсолютный URL.
  const imgUrl = masterOgImageUrl(profile)
  if (imgUrl) {
    out.image = imgUrl.startsWith("http") ? imgUrl : `${base}${imgUrl}`
  }
  const city = profile.city?.trim()
  if (city) {
    out.homeLocation = {
      "@type": "Place",
      name: city,
    }
  }
  return out
}

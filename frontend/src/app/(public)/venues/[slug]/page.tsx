import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { ApiError, getVenueBySlug } from "@/lib/api"
import { siteUrl } from "@/lib/seo-site"
import { safeJsonLdStringify } from "@/lib/seo-jsonld"
import { venueLocalBusinessJsonLd, venueOgImageUrl } from "@/lib/venue-jsonld"
import { VenuePublicPageClient } from "./venue-public-page-client"

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>
}): Promise<Metadata> {
  const { slug } = await params
  try {
    const v = await getVenueBySlug(slug)
    const title = `${v.name} — ${v.city} | БаняГид`
    const raw = (v.description || "").replace(/\s+/g, " ").trim()
    const desc =
      raw.length > 155 ? `${raw.slice(0, 152)}…` : raw || `${v.name}: баня/сауна в ${v.city}. Цены, отзывы, онлайн-бронирование.`
    const base = siteUrl()
    const canonicalUrl = `${base}/venues/${encodeURIComponent(slug)}`
    const ogImage = venueOgImageUrl(v)
    return {
      title,
      description: desc,
      alternates: { canonical: canonicalUrl },
      openGraph: {
        title: v.name,
        description: desc,
        // url должен быть абсолютным — metadataBase не всегда применяется к OG.
        url: canonicalUrl,
        locale: "ru_RU",
        type: "website",
        ...(ogImage
          ? {
              images: [
                {
                  url: ogImage.startsWith("http") ? ogImage : `${base}${ogImage}`,
                  alt: v.name,
                },
              ],
            }
          : {}),
      },
    }
  } catch {
    return { title: "Заведение | БаняГид" }
  }
}

export default async function VenueDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = await params
  let venue: Awaited<ReturnType<typeof getVenueBySlug>>
  try {
    venue = await getVenueBySlug(slug)
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      notFound()
    }
    throw error
  }
  const jsonLd = venueLocalBusinessJsonLd(venue, slug)
  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(jsonLd) }}
      />
      <VenuePublicPageClient slug={slug} initialVenue={venue} />
    </>
  )
}

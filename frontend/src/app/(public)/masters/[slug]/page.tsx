import type { Metadata } from "next"
import { notFound } from "next/navigation"
import { ApiError, getPublicMaster } from "@/lib/api"
import { safeJsonLdStringify } from "@/lib/seo-jsonld"
import { siteUrl } from "@/lib/seo-site"
import { masterPersonJsonLd, masterOgImageUrl } from "@/lib/master-jsonld"
import { MasterPublicPageClient } from "./master-public-page-client"

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>
}): Promise<Metadata> {
  const { slug } = await params
  try {
    const m = await getPublicMaster(slug)
    const title = `${m.display_name} — мастер, ${m.city} | БаняГид`
    const raw = (m.bio || "").replace(/\s+/g, " ").trim()
    const desc =
      raw.length > 160 ? `${raw.slice(0, 157)}…` : raw || `Мастер ${m.display_name} в ${m.city}. Услуги, отзывы, онлайн-запись.`
    const base = siteUrl()
    const canonicalUrl = `${base}/masters/${encodeURIComponent(slug)}`
    const ogImage = masterOgImageUrl(m)
    return {
      title,
      description: desc,
      alternates: { canonical: canonicalUrl },
      openGraph: {
        title: m.display_name,
        description: desc,
        url: canonicalUrl,
        locale: "ru_RU",
        type: "profile",
        ...(ogImage
          ? {
              images: [
                {
                  url: ogImage.startsWith("http") ? ogImage : `${base}${ogImage}`,
                  alt: m.display_name,
                },
              ],
            }
          : {}),
      },
    }
  } catch {
    return { title: "Мастер | БаняГид" }
  }
}

export default async function MasterPublicPage({
  params,
}: {
  params: Promise<{ slug: string }>
}) {
  const { slug } = await params
  let profile: Awaited<ReturnType<typeof getPublicMaster>>
  try {
    profile = await getPublicMaster(slug)
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      notFound()
    }
    throw error
  }
  const jsonLd = masterPersonJsonLd(profile, slug)
  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(jsonLd) }}
      />
      <MasterPublicPageClient slug={slug} initialMaster={profile} />
    </>
  )
}

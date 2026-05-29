import { Suspense } from "react"
import type { Metadata } from "next"
import { HeroSection } from "@/components/banya/hero-section"
import { FeaturesSection } from "@/components/banya/features-section"
import { PopularVenuesSection } from "@/components/banya/popular-venues-section"
import { PopularVenuesSkeleton } from "@/components/banya/popular-venues-skeleton"
import { getVenues } from "@/lib/api"
import { siteUrl } from "@/lib/seo-site"

export const metadata: Metadata = {
  title: "БаняГид — бани и сауны с отзывами и бронированием",
  description:
    "Каталог бань и саун России: сравнивайте цены, читайте отзывы, бронируйте онлайн. Бани рядом с вами в Москве, Санкт-Петербурге, Казани и других городах.",
  alternates: { canonical: siteUrl() },
  openGraph: {
    title: "БаняГид — бани и сауны с отзывами и бронированием",
    description: "Более 500 проверенных заведений с отзывами, фото и онлайн-бронированием.",
    url: siteUrl(),
    images: [{ url: `${siteUrl()}/og-home.jpg`, width: 1200, height: 630, alt: "БаняГид" }],
  },
}

/** Async Server Component — fetches venues independently, streamed via Suspense. */
async function PopularVenuesFetcher() {
  const result = await getVenues({ page: 1, page_size: 3, sort_by: "rating" }).catch(
    () => ({ venues: [] as import("@/lib/types").Venue[], total: 0, page: 1, page_size: 3 }),
  )
  return <PopularVenuesSection venues={result.venues} />
}

/** Server Component — HeroSection и FeaturesSection рендерятся мгновенно.
 *  PopularVenuesFetcher стримится отдельно через Suspense — не блокирует
 *  первый байт ответа и навигацию к /partner. */
export default function HomePage() {
  return (
    <>
      <HeroSection />
      <FeaturesSection />
      <Suspense fallback={<PopularVenuesSkeleton />}>
        <PopularVenuesFetcher />
      </Suspense>
    </>
  )
}

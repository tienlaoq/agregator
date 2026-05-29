import type { Metadata } from "next"
import Link from "next/link"
import { Suspense } from "react"
import { notFound } from "next/navigation"
import { CatalogSection } from "@/components/banya/catalog-section"
import {
  SEO_VENUE_KIND_INTRO,
  SEO_VENUE_KIND_LABELS,
  allCityKindStaticParams,
  getSeoCityHub,
  isSeoVenueKindSlug,
  type SeoVenueKindSlug,
} from "@/lib/seo-city-hubs"
import { siteUrl } from "@/lib/seo-site"
import { searchVenues } from "@/lib/api"

export function generateStaticParams() {
  return allCityKindStaticParams()
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ citySlug: string; kind: string }>
}): Promise<Metadata> {
  const { citySlug, kind } = await params
  if (!isSeoVenueKindSlug(kind)) return { title: "Каталог" }
  const hub = getSeoCityHub(citySlug)
  if (!hub) return { title: "Каталог" }
  const k = kind as SeoVenueKindSlug
  const label = SEO_VENUE_KIND_LABELS[k].toLowerCase()
  const path = `/venues/city/${citySlug}/${kind}`
  const title = `${SEO_VENUE_KIND_LABELS[k]} в ${hub.name} — каталог | БаняГид`
  return {
    title,
    description: `${label.charAt(0).toUpperCase() + label.slice(1)} в ${hub.name}: сравните цены и отзывы, забронируйте онлайн. ${SEO_VENUE_KIND_INTRO[k]}`,
    alternates: { canonical: `${siteUrl()}${path}` },
    openGraph: {
      title: `${SEO_VENUE_KIND_LABELS[k]} в ${hub.name}`,
      description: SEO_VENUE_KIND_INTRO[k],
      url: path,
      locale: "ru_RU",
    },
  }
}

export default async function VenuesCityKindHubPage({
  params,
}: {
  params: Promise<{ citySlug: string; kind: string }>
}) {
  const { citySlug, kind } = await params
  if (!isSeoVenueKindSlug(kind)) notFound()
  const hub = getSeoCityHub(citySlug)
  if (!hub) notFound()
  const k = kind as SeoVenueKindSlug

  const initialData = await searchVenues(
    { city: hub.name, type: k, page: 1, page_size: 12 },
  ).catch(() => ({ venues: [] as import("@/lib/types").Venue[], total: 0, page: 1, page_size: 12 }))

  return (
    <article className="bg-background">
      <div className="container mx-auto max-w-4xl px-4 pt-10 pb-2">
        <nav className="mb-4 text-sm text-muted-foreground">
          <Link href="/" className="hover:text-foreground">
            Главная
          </Link>
          {" / "}
          <Link href="/venues" className="hover:text-foreground">
            Каталог
          </Link>
          {" / "}
          <Link href={`/venues/city/${hub.slug}`} className="hover:text-foreground">
            {hub.name}
          </Link>
          {" / "}
          <span className="text-foreground">{SEO_VENUE_KIND_LABELS[k]}</span>
        </nav>
        <h1 className="text-3xl font-bold tracking-tight text-foreground md:text-4xl">
          {SEO_VENUE_KIND_LABELS[k]} в {hub.name}
        </h1>
        <p className="mt-4 leading-relaxed text-muted-foreground">{hub.intro}</p>
        <p className="mt-4 leading-relaxed text-muted-foreground">{SEO_VENUE_KIND_INTRO[k]}</p>
        <p className="mt-6 text-sm">
          <Link href={`/venues/city/${hub.slug}`} className="text-muted-foreground underline underline-offset-2 hover:text-foreground">
            Все типы в {hub.name}
          </Link>
          {" · "}
          <Link href="/venues" className="text-muted-foreground underline underline-offset-2 hover:text-foreground">
            Весь каталог
          </Link>
        </p>
      </div>
      <Suspense fallback={<div className="flex min-h-[40vh] items-center justify-center text-muted-foreground">Загрузка каталога…</div>}>
        <CatalogSection
          key={`hub-${hub.slug}-${k}`}
          hubCity={hub.name}
          hubDefaultVenueType={k}
          catalogTitle={`${SEO_VENUE_KIND_LABELS[k]} — ${hub.name}`}
          initialData={{ venues: initialData.venues, total: initialData.total }}
        />
      </Suspense>
    </article>
  )
}

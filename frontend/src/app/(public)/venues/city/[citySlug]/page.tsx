import type { Metadata } from "next"
import Link from "next/link"
import { Suspense } from "react"
import { notFound } from "next/navigation"
import { CatalogSection } from "@/components/banya/catalog-section"
import { SEO_CITY_HUBS, getSeoCityHub } from "@/lib/seo-city-hubs"
import { siteUrl } from "@/lib/seo-site"
import { searchVenues } from "@/lib/api"

export function generateStaticParams() {
  return SEO_CITY_HUBS.map((c) => ({ citySlug: c.slug }))
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ citySlug: string }>
}): Promise<Metadata> {
  const { citySlug } = await params
  const hub = getSeoCityHub(citySlug)
  if (!hub) return { title: "Каталог" }
  const path = `/venues/city/${citySlug}`
  return {
    title: `Бани и сауны в ${hub.name} — каталог | БаняГид`,
    description: `Выберите баню или сауну в ${hub.name}: цены, отзывы, онлайн-бронирование. ${hub.intro}`,
    alternates: { canonical: `${siteUrl()}${path}` },
    openGraph: {
      title: `Бани и сауны в ${hub.name}`,
      description: hub.intro,
      url: path,
      locale: "ru_RU",
    },
  }
}

export default async function VenuesCityHubPage({
  params,
}: {
  params: Promise<{ citySlug: string }>
}) {
  const { citySlug } = await params
  const hub = getSeoCityHub(citySlug)
  if (!hub) notFound()

  const others = SEO_CITY_HUBS.filter((c) => c.slug !== hub.slug)

  // SSR: город уже известен из URL — делаем fetch сразу, бот получает готовый HTML.
  const initialData = await searchVenues(
    { city: hub.name, page: 1, page_size: 12 },
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
          <span className="text-foreground">{hub.name}</span>
        </nav>
        <h1 className="text-3xl font-bold tracking-tight text-foreground md:text-4xl">
          Бани и сауны в {hub.name}
        </h1>
        <p className="mt-4 leading-relaxed text-muted-foreground">{hub.intro}</p>
        <p className="mt-6 text-sm text-muted-foreground">
          Ещё города:{" "}
          {others.slice(0, 6).map((c, i) => (
            <span key={c.slug}>
              {i > 0 ? " · " : ""}
              <Link href={`/venues/city/${c.slug}`} className="underline underline-offset-2 hover:text-foreground">
                {c.name}
              </Link>
            </span>
          ))}
          {" · "}
          <Link href="/venues" className="underline underline-offset-2 hover:text-foreground">
            весь каталог
          </Link>
        </p>
        <p className="mt-4 text-sm text-muted-foreground">
          По типу:{" "}
          <Link className="underline underline-offset-2 hover:text-foreground" href={`/venues/city/${hub.slug}/banya`}>
            бани
          </Link>
          {" · "}
          <Link className="underline underline-offset-2 hover:text-foreground" href={`/venues/city/${hub.slug}/sauna`}>
            сауны
          </Link>
          {" · "}
          <Link className="underline underline-offset-2 hover:text-foreground" href={`/venues/city/${hub.slug}/hammam`}>
            хаммамы
          </Link>
        </p>
      </div>
      <Suspense fallback={<div className="flex min-h-[40vh] items-center justify-center text-muted-foreground">Загрузка каталога…</div>}>
        <CatalogSection
          key={`hub-${hub.slug}-all`}
          hubCity={hub.name}
          hubDefaultVenueType="all"
          catalogTitle={`Подборка в ${hub.name}`}
          initialData={{ venues: initialData.venues, total: initialData.total }}
        />
      </Suspense>
    </article>
  )
}

import { Suspense } from "react"
import type { Metadata } from "next"
import { CatalogSection } from "@/components/banya/catalog-section"
import { getVenues } from "@/lib/api"

export const metadata: Metadata = {
  title: "Каталог бань и саун",
  description:
    "Ищите баню или сауну по городу, цене и рейтингу. Отзывы гостей и онлайн-бронирование на БаняГид.",
  alternates: { canonical: "/venues" },
  openGraph: {
    title: "Каталог бань и саун — БаняГид",
    description: "Сравнивайте заведения, читайте отзывы, бронируйте онлайн.",
    url: "/venues",
  },
}

/** Server Component: первая страница каталога рендерится на сервере — бот получает HTML с заведениями. */
export default async function VenuesPage() {
  const initialData = await getVenues({ page: 1, page_size: 12, sort_by: "rating" }).catch(
    () => ({ venues: [] as import("@/lib/types").Venue[], total: 0, page: 1, page_size: 12 }),
  )

  return (
    <Suspense fallback={<div className="flex min-h-[50vh] items-center justify-center text-muted-foreground">Загрузка каталога...</div>}>
      <CatalogSection initialData={{ venues: initialData.venues, total: initialData.total }} />
    </Suspense>
  )
}

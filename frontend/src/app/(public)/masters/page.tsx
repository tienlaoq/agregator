import { Suspense } from "react"
import type { Metadata } from "next"
import { MastersCatalogSection } from "@/components/banya/masters-catalog-section"

export const metadata: Metadata = {
  title: "Мастера банных услуг",
  description:
    "Каталог мастеров парения и банных услуг: город, формат работы, отзывы и онлайн-запись через БаняГид.",
  alternates: { canonical: "/masters" },
  openGraph: {
    title: "Мастера — БаняГид",
    description: "Найдите мастера в своём городе, посмотрите специализации и забронируйте визит.",
    url: "/masters",
  },
}

export default function MastersCatalogPage() {
  return (
    <Suspense
      fallback={
        <div className="flex min-h-[50vh] items-center justify-center text-muted-foreground">
          Загрузка каталога…
        </div>
      }
    >
      <MastersCatalogSection />
    </Suspense>
  )
}

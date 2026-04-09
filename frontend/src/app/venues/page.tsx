import { Suspense } from "react"
import { CatalogSection } from "@/components/banya/catalog-section"

export default function VenuesPage() {
  return (
    <Suspense fallback={<div className="flex min-h-[50vh] items-center justify-center text-muted-foreground">Загрузка каталога...</div>}>
      <CatalogSection />
    </Suspense>
  )
}

import { Suspense } from "react"
import { MastersCatalogSection } from "@/components/banya/masters-catalog-section"

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

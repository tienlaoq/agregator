import { Suspense } from "react"
import type { Metadata } from "next"
import { MastersCatalogSection } from "@/components/banya/masters-catalog-section"
import { getPublicMastersCatalog } from "@/lib/api"
import type { MasterProfile } from "@/lib/types"

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

/** Server Component: первая страница каталога мастеров рендерится на сервере —
 *  SSR обязателен для каталога (SEO «мастера бани [город]»). Без серверных
 *  данных useSearchParams в секции делал CSR-bailout, и страница залипала на
 *  фолбэке. Клиент дофетчивает при фильтрах/пагинации. */
export default async function MastersCatalogPage() {
  const initialData = await getPublicMastersCatalog({ page: 1, page_size: 12 }).catch(
    () => ({ masters: [] as MasterProfile[], total: 0 }),
  )

  return (
    <Suspense
      fallback={
        <div className="flex min-h-[50vh] items-center justify-center text-muted-foreground">
          Загрузка каталога…
        </div>
      }
    >
      <MastersCatalogSection initialData={{ masters: initialData.masters, total: initialData.total }} />
    </Suspense>
  )
}

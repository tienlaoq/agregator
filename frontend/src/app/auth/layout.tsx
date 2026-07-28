import type { Metadata } from "next"
import Link from "next/link"
import { Flame } from "lucide-react"

export const metadata: Metadata = {
  robots: { index: false, follow: false },
}

/**
 * Layout для /auth/* — минималистичный, без основной шапки, футера и чат-виджета.
 * Фокус на форме входа/регистрации без отвлекающих элементов.
 *
 * Кликабельный логотип «БаняГид» сверху — единственный навигационный элемент:
 * он же служит понятным способом вернуться на главную, не загромождая форму.
 */
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col bg-background">
      {/* pt со safe-area: в нативной оболочке viewport-fit=cover пускает контент
          под статус-бар, и логотип наезжает на часы. Публичный Header делает то
          же самое (header.tsx), здесь свой — отступ пришлось повторить. */}
      <header className="px-4 pb-5 pt-[calc(env(safe-area-inset-top)+1.25rem)] sm:px-6">
        <Link
          href="/"
          aria-label="На главную БаняГид"
          className="inline-flex items-center gap-2 rounded-lg transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary">
            <Flame className="h-5 w-5 text-primary-foreground" />
          </div>
          <span className="text-xl font-bold text-foreground">БаняГид</span>
        </Link>
      </header>
      <main className="flex flex-1 flex-col items-center justify-center px-4 pb-12">
        {children}
      </main>
    </div>
  )
}

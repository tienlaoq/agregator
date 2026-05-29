import type { Metadata } from "next"

export const metadata: Metadata = {
  robots: { index: false, follow: false },
}

/**
 * Layout для /auth/* — минималистичный, без шапки, футера и чат-виджета.
 * Фокус на форме входа/регистрации без отвлекающих элементов.
 */
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-background flex flex-col items-center justify-center px-4">
      {children}
    </div>
  )
}

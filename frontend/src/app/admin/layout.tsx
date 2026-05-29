import type { Metadata } from "next"
import { AppHeader } from "@/components/banya/app-header"

export const metadata: Metadata = {
  robots: { index: false, follow: false },
}

/**
 * Layout для /admin — шапка есть (для навигации и выхода из аккаунта),
 * без публичного футера и чат-виджета.
 */
export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col bg-background">
      <AppHeader />
      <main className="flex-1">{children}</main>
    </div>
  )
}

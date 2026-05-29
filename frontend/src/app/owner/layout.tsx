import type { Metadata } from "next"
import { AppHeader } from "@/components/banya/app-header"

export const metadata: Metadata = {
  robots: { index: false, follow: false },
}

/**
 * Layout для личного кабинета владельца /owner — шапка есть, но без футера и чат-виджета.
 */
export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col bg-background">
      <AppHeader />
      <main className="flex-1">{children}</main>
    </div>
  )
}

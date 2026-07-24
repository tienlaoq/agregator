"use client"

import { AppHeader } from "@/components/banya/app-header"
import { Footer } from "@/components/banya/footer"
import { ChatWidget } from "@/components/banya/chat-widget"
import { useIsNative } from "@/hooks/use-is-native"

export function AppLayout({ children }: { children: React.ReactNode }) {
  // Внутри приложения (Capacitor) прячем веб-футер: это большой SEO-блок
  // (города, реквизиты, соцсети), в нативной оболочке он лишний и выдаёт «сайт».
  const isNative = useIsNative()

  return (
    <div className="min-h-screen flex flex-col bg-background">
      <AppHeader />
      <main className="flex-1">{children}</main>
      {!isNative && <Footer />}
      <ChatWidget />
    </div>
  )
}

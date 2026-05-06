"use client"

import { AppHeader } from "@/components/banya/app-header"
import { Footer } from "@/components/banya/footer"
import { ChatWidget } from "@/components/banya/chat-widget"

export function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col bg-background">
      <AppHeader />
      <main className="flex-1">{children}</main>
      <Footer />
      <ChatWidget />
    </div>
  )
}

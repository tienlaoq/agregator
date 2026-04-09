"use client"

import { Header } from "@/components/banya/header"
import { useAuthStore } from "@/store/auth"

export function AppHeader() {
  const user = useAuthStore((s) => s.user)
  const hydrated = useAuthStore((s) => s.hydrated)

  if (!hydrated) return <Header isLoggedIn={false} />

  return (
    <Header
      isLoggedIn={!!user}
      userName={user?.name || ""}
      userAvatar={user?.avatar_url}
      userRole={user?.role}
    />
  )
}

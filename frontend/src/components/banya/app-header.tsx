"use client"

import { Header } from "@/components/banya/header"
import { useAuthStore, hasAuthSession } from "@/store/auth"

export function AppHeader() {
  const user = useAuthStore((s) => s.user)
  const token = useAuthStore((s) => s.token)
  const hydrated = useAuthStore((s) => s.hydrated)

  if (!hydrated) return <Header isLoggedIn={false} />

  return (
    <Header
      isLoggedIn={hasAuthSession(user, token)}
      userName={user?.name || ""}
      userAvatar={user?.avatar_url}
      userRole={user?.role}
    />
  )
}

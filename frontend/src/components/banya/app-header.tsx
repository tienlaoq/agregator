"use client"

import { useQuery } from "@tanstack/react-query"
import { Header } from "@/components/banya/header"
import { getOwnerVenues } from "@/lib/api"
import { useAuthStore, hasAuthSession } from "@/store/auth"

export function AppHeader() {
  const user = useAuthStore((s) => s.user)
  const token = useAuthStore((s) => s.token)
  const hydrated = useAuthStore((s) => s.hydrated)

  const loggedIn = hasAuthSession(user, token)
  // Invited staff keep role "user"/"master"; the owner-cabinet link is otherwise
  // gated by role and would never appear for them. Query CRM management access so
  // anyone managing ≥1 venue (owner or invited worker) gets the link. Admins
  // don't manage venues — skip the call for them. Shares the ["owner-venues"]
  // cache with the owner dashboard, so no duplicate fetch there.
  const { data: managedVenues } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: loggedIn && user?.role !== "admin",
    staleTime: 60_000,
  })

  if (!hydrated) return <Header isLoggedIn={false} />

  return (
    <Header
      isLoggedIn={loggedIn}
      userName={user?.name || ""}
      userAvatar={user?.avatar_url}
      userRole={user?.role}
      hasManagedVenues={(managedVenues?.length ?? 0) > 0}
    />
  )
}

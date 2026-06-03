"use client"

import { Suspense, useEffect } from "react"
import { useRouter } from "next/navigation"
import { getProfile } from "@/lib/api"
import { useAuthStore } from "@/store/auth"
import { Flame } from "lucide-react"

function CallbackSuspenseFallback() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 text-sm text-muted-foreground">
      Загрузка…
    </div>
  )
}

export default function OAuthCallbackPage() {
  return (
    <Suspense fallback={<CallbackSuspenseFallback />}>
      <CallbackHandler />
    </Suspense>
  )
}

/**
 * Reads the access_token from the URL fragment (#access_token=...).
 *
 * The gateway no longer puts tokens in query parameters to prevent them from
 * appearing in proxy/CDN logs, Referer headers, and browser history:
 *   - refresh_token → HttpOnly "banya_refresh" cookie (set by the gateway,
 *     consumed by /api/auth/refresh; JS never sees it).
 *   - access_token  → URL fragment (not sent to the server, not in Referer).
 *
 * Fragments are not available server-side, so this must be a Client Component
 * that reads window.location.hash after mount.
 */
function CallbackHandler() {
  const router = useRouter()
  const authLogin = useAuthStore((s) => s.login)

  useEffect(() => {
    // Parse the fragment: "#access_token=<value>" → URLSearchParams
    const fragment = window.location.hash.slice(1) // strip leading '#'
    const params = new URLSearchParams(fragment)
    const accessToken = params.get("access_token")

    if (!accessToken) {
      router.push("/auth/login?error=oauth_failed")
      return
    }

    // Clear the fragment from the address bar so the token is not visible in
    // browser history entries created after this navigation.
    window.history.replaceState(null, "", window.location.pathname)

    // Store the access token in memory FIRST, so getProfile() authenticates with
    // it directly. Otherwise getProfile() would run with no token, 401, and
    // trigger the silent-refresh flow — which consumes the single-use refresh
    // cookie and races the app-level bootstrap(), causing a spurious 401 and a
    // bounce back to the auth screen.
    useAuthStore.setState({ token: accessToken })

    // The refresh token was delivered as an HttpOnly cookie by the gateway.
    // Pass an empty string to login() — it will still call /api/auth/set-refresh
    // but with an empty body, which the route handler ignores (missing token → no-op).
    // The cookie is already set; no further action is needed.
    getProfile()
      .then((user) => {
        authLogin(accessToken, "", user)
        router.push("/")
      })
      .catch(() => {
        router.push("/auth/login?error=profile_failed")
      })
  }, [authLogin, router])

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4">
      <div className="flex h-16 w-16 animate-pulse items-center justify-center rounded-full bg-primary">
        <Flame className="h-8 w-8 text-primary-foreground" />
      </div>
      <p className="text-muted-foreground">Авторизация...</p>
    </div>
  )
}

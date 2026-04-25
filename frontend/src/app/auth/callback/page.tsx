"use client"

import { Suspense, useEffect } from "react"
import { useRouter, useSearchParams } from "next/navigation"
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

function CallbackHandler() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const authLogin = useAuthStore((s) => s.login)

  useEffect(() => {
    const accessToken = searchParams.get("access_token")
    const refreshToken = searchParams.get("refresh_token")

    if (!accessToken) {
      router.push("/auth/login?error=oauth_failed")
      return
    }

    localStorage.setItem("token", accessToken)
    if (refreshToken) {
      localStorage.setItem("refresh_token", refreshToken)
    }

    getProfile()
      .then((user) => {
        authLogin(accessToken, refreshToken || "", user)
        router.push("/")
      })
      .catch(() => {
        router.push("/auth/login?error=profile_failed")
      })
  }, [searchParams, authLogin, router])

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4">
      <div className="flex h-16 w-16 animate-pulse items-center justify-center rounded-full bg-primary">
        <Flame className="h-8 w-8 text-primary-foreground" />
      </div>
      <p className="text-muted-foreground">Авторизация...</p>
    </div>
  )
}

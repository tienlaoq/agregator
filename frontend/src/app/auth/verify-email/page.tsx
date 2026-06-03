"use client"

import { Suspense, useEffect, useRef, useState } from "react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { verifyEmail, resendVerification, ApiError, formatApiErrorMessage } from "@/lib/api"
import { Flame } from "lucide-react"

const RESEND_COOLDOWN_SECONDS = 60

type Status = "verifying" | "success" | "error"

function VerifyEmailInner() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get("token")?.trim() ?? ""

  const [status, setStatus] = useState<Status>(token ? "verifying" : "error")
  const [error, setError] = useState(
    token ? "" : "В ссылке нет токена. Откройте письмо ещё раз или запросите новое.",
  )

  // Resend sub-form state.
  const [email, setEmail] = useState("")
  const [resendDone, setResendDone] = useState(false)
  const [resendError, setResendError] = useState("")
  const [resending, setResending] = useState(false)
  const [cooldown, setCooldown] = useState(0)

  // Guard against React 18/19 double-invoke of effects in dev StrictMode so the
  // one-time token is not consumed twice (the second call would 400).
  const verifiedRef = useRef(false)

  useEffect(() => {
    if (!token || verifiedRef.current) return
    verifiedRef.current = true

    let cancelled = false
    void (async () => {
      try {
        await verifyEmail(token)
        if (cancelled) return
        setStatus("success")
        setTimeout(() => router.push("/auth/login"), 2500)
      } catch (err) {
        if (cancelled) return
        setStatus("error")
        setError(formatApiErrorMessage(err, "Не удалось подтвердить email"))
      }
    })()

    return () => {
      cancelled = true
    }
  }, [token, router])

  // Cooldown ticker for the resend button.
  useEffect(() => {
    if (cooldown <= 0) return
    const id = setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => clearTimeout(id)
  }, [cooldown])

  const handleResend = async (e: React.FormEvent) => {
    e.preventDefault()
    setResendError("")
    setResending(true)
    try {
      await resendVerification(email.trim())
      setResendDone(true)
      setCooldown(RESEND_COOLDOWN_SECONDS)
    } catch (err) {
      if (err instanceof ApiError) {
        setResendError(formatApiErrorMessage(err, "Не удалось отправить письмо"))
      } else {
        setResendError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
    } finally {
      setResending(false)
    }
  }

  return (
    <Card className="w-full max-w-md border-border">
      <CardHeader className="text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary">
          <Flame className="h-6 w-6 text-primary-foreground" />
        </div>
        <CardTitle className="text-2xl text-card-foreground">Подтверждение email</CardTitle>
        <CardDescription>
          {status === "verifying" && "Проверяем ссылку…"}
          {status === "success" && "Email подтверждён."}
          {status === "error" && "Не удалось подтвердить email по этой ссылке."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {status === "verifying" && (
          <p className="text-center text-sm text-muted-foreground">Подождите немного…</p>
        )}

        {status === "success" && (
          <p className="text-center text-sm text-muted-foreground">
            Готово! Теперь вы можете публиковать баню или анкету мастера. Сейчас откроется страница входа…
          </p>
        )}

        {status === "error" && (
          <div className="space-y-4">
            <p className="text-sm text-destructive">{error}</p>
            {resendDone ? (
              <p className="text-sm text-muted-foreground">
                Если для адреса требуется подтверждение, мы повторно отправили ссылку. Проверьте почту и
                папку «Спам».
              </p>
            ) : (
              <form onSubmit={handleResend} className="space-y-3">
                <div className="space-y-2">
                  <Label htmlFor="email">Email аккаунта</Label>
                  <Input
                    id="email"
                    type="email"
                    autoComplete="email"
                    placeholder="your@email.com"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                  />
                </div>
                {resendError && <p className="text-sm text-destructive">{resendError}</p>}
                <Button type="submit" className="w-full" disabled={resending || cooldown > 0}>
                  {resending
                    ? "Отправка…"
                    : cooldown > 0
                      ? `Отправить повторно через ${cooldown} с`
                      : "Отправить ссылку заново"}
                </Button>
              </form>
            )}
          </div>
        )}

        <p className="text-center text-sm text-muted-foreground">
          <Link href="/auth/login" className="font-medium text-primary hover:underline">
            Назад ко входу
          </Link>
        </p>
      </CardContent>
    </Card>
  )
}

function VerifyFallback() {
  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4 text-sm text-muted-foreground">
      Загрузка…
    </div>
  )
}

export default function VerifyEmailPage() {
  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4 py-12">
      <Suspense fallback={<VerifyFallback />}>
        <VerifyEmailInner />
      </Suspense>
    </div>
  )
}

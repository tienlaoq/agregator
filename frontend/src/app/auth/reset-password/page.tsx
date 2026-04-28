"use client"

import { Suspense, useState } from "react"
import Link from "next/link"
import { useRouter, useSearchParams } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { completePasswordReset, ApiError, formatApiErrorMessage } from "@/lib/api"
import { Flame } from "lucide-react"

function ResetPasswordForm() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const token = searchParams.get("token")?.trim() ?? ""

  const [password, setPassword] = useState("")
  const [password2, setPassword2] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [done, setDone] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError("")
    if (password.length < 8) {
      setError("Пароль не короче 8 символов")
      return
    }
    if (password !== password2) {
      setError("Пароли не совпадают")
      return
    }
    if (!token) {
      setError("В ссылке нет токена. Запросите новое письмо со страницы сброса пароля.")
      return
    }
    setLoading(true)
    try {
      await completePasswordReset(token, password)
      setDone(true)
      setTimeout(() => router.push("/auth/login"), 2000)
    } catch (err) {
      if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Не удалось сменить пароль"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card className="w-full max-w-md border-border">
      <CardHeader className="text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary">
          <Flame className="h-6 w-6 text-primary-foreground" />
        </div>
        <CardTitle className="text-2xl text-card-foreground">Новый пароль</CardTitle>
        <CardDescription>Введите новый пароль для входа по email.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {done ? (
          <p className="text-center text-sm text-muted-foreground">
            Пароль обновлён. Сейчас откроется страница входа…
          </p>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            {!token && (
              <p className="text-sm text-destructive">
                Ссылка неполная. Откройте письмо ещё раз или{" "}
                <Link href="/auth/forgot-password" className="underline">
                  запросите сброс заново
                </Link>
                .
              </p>
            )}
            <div className="space-y-2">
              <Label htmlFor="password">Новый пароль</Label>
              <Input
                id="password"
                type="password"
                autoComplete="new-password"
                placeholder="Не менее 8 символов"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password2">Повтор пароля</Label>
              <Input
                id="password2"
                type="password"
                autoComplete="new-password"
                value={password2}
                onChange={(e) => setPassword2(e.target.value)}
                required
                minLength={8}
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading || !token}>
              {loading ? "Сохранение…" : "Сохранить пароль"}
            </Button>
          </form>
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

function ResetFallback() {
  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4 text-sm text-muted-foreground">
      Загрузка…
    </div>
  )
}

export default function ResetPasswordPage() {
  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4 py-12">
      <Suspense fallback={<ResetFallback />}>
        <ResetPasswordForm />
      </Suspense>
    </div>
  )
}

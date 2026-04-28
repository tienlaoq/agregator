"use client"

import { useState } from "react"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { requestPasswordReset, ApiError, formatApiErrorMessage } from "@/lib/api"
import { Flame } from "lucide-react"

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("")
  const [done, setDone] = useState(false)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError("")
    setLoading(true)
    try {
      await requestPasswordReset(email.trim())
      setDone(true)
    } catch (err) {
      if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Не удалось отправить запрос"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4 py-12">
      <Card className="w-full max-w-md border-border">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary">
            <Flame className="h-6 w-6 text-primary-foreground" />
          </div>
          <CardTitle className="text-2xl text-card-foreground">Сброс пароля</CardTitle>
          <CardDescription>
            Укажите email аккаунта. Если он зарегистрирован и для входа задан пароль, придёт письмо со
            ссылкой.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {done ? (
            <p className="text-center text-sm text-muted-foreground">
              Если указанный адрес зарегистрирован и для него задан пароль, мы отправили на него ссылку для
              сброса. Проверьте почту и папку «Спам».
            </p>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
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
              {error && <p className="text-sm text-destructive">{error}</p>}
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? "Отправка…" : "Отправить ссылку"}
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
    </div>
  )
}

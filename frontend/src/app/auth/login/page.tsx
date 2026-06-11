"use client"

import { Suspense, useState, useEffect } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { login, getProfile, ApiError, formatApiErrorMessage } from "@/lib/api"

const OAUTH_QUERY_ERROR_MESSAGES: Record<string, string> = {
  oauth_failed: "Не удалось войти через соцсеть. Попробуйте ещё раз.",
  profile_failed: "Не удалось загрузить профиль. Войдите снова.",
}
import { useAuthStore } from "@/store/auth"
import { Flame } from "lucide-react"

function VKIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor">
      <path d="M12.785 16.241s.288-.032.436-.194c.136-.148.132-.427.132-.427s-.02-1.304.587-1.496c.598-.188 1.366 1.259 2.18 1.815.616.42 1.084.328 1.084.328l2.175-.03s1.14-.071.6-.97c-.044-.074-.316-.668-1.627-1.886-1.372-1.276-1.188-1.07.465-3.278.896-1.198 1.386-2.07 1.176-2.342-.2-.26-1.432-.106-1.432-.106l-2.456.015s-.182-.025-.317.056c-.131.079-.216.263-.216.263s-.388 1.032-.905 1.91c-1.091 1.852-1.527 1.95-1.706 1.836-.416-.267-.312-1.07-.312-1.641 0-1.785.27-2.528-.53-2.72-.265-.064-.46-.106-1.138-.113-.87-.009-1.606.003-2.023.207-.278.136-.492.439-.362.456.162.022.528.099.722.363.25.341.241 1.11.241 1.11s.144 2.1-.335 2.36c-.329.18-.78-.186-1.748-1.862-.496-.858-.87-1.808-.87-1.808s-.072-.177-.2-.272c-.156-.115-.373-.151-.373-.151l-2.335.015s-.35.01-.479.163c-.114.135-.01.415-.01.415s1.825 4.272 3.892 6.424c1.895 1.973 4.046 1.843 4.046 1.843h.975z" />
    </svg>
  )
}

function YandexIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24">
      <circle cx="12" cy="12" r="12" fill="#FC3F1D" />
      <path d="M13.32 18.86V5.93h-1.8c-2.18 0-3.66 1.27-3.66 3.42 0 1.43.61 2.43 1.83 3.18l-2.85 6.33h2.13l3.18-7.1-1.02-.55c-1-.55-1.44-1.06-1.44-2.05 0-.87.55-1.46 1.5-1.46h.93v10.16h2.2z" fill="#fff" />
    </svg>
  )
}

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"

function LoginSuspenseFallback() {
  return (
    <div className="flex min-h-[70vh] items-center justify-center px-4 text-sm text-muted-foreground">
      Загрузка…
    </div>
  )
}

export default function LoginPage() {
  return (
    <Suspense fallback={<LoginSuspenseFallback />}>
      <LoginForm />
    </Suspense>
  )
}

function LoginForm() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const setTokens = useAuthStore((s) => s.setTokens)
  const setUser = useAuthStore((s) => s.setUser)
  const [emailOrPhone, setEmailOrPhone] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    const oauthError = searchParams.get("error")
    if (!oauthError) return
    const key = decodeURIComponent(oauthError).trim()
    setError(OAUTH_QUERY_ERROR_MESSAGES[key] ?? "Не удалось выполнить вход. Попробуйте снова.")
  }, [searchParams])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError("")
    setLoading(true)
    try {
      const res = await login({ email: emailOrPhone, password })
      // Сначала кладём access-токен в стор (fetchAPI читает его только из памяти
      // Zustand) и ставим refresh-cookie — иначе getProfile() уйдёт без
      // Authorization, получит 401 и обработчик 401 в fetchAPI сделает
      // window.location.href = "/auth/login" (та самая «перезагрузка»).
      await setTokens(res.access_token, res.refresh_token)
      const user = await getProfile()
      setUser(user)
      router.push("/")
    } catch (err) {
      if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Неверный email или пароль"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
      // Сбрасываем только пароль — email/телефон оставляем, чтобы не вводить заново.
      setPassword("")
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
          <CardTitle className="text-2xl text-card-foreground">Войти в БаняГид</CardTitle>
          <CardDescription>Быстрый вход для бронирования</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* OAuth Buttons */}
          <div className="grid grid-cols-2 gap-3">
            <Button variant="outline" className="min-w-0 gap-2" type="button" asChild>
              <a href={`${API_URL}/api/v1/auth/vk`}>
                <VKIcon className="h-5 w-5 shrink-0" />
                ВК
              </a>
            </Button>
            <Button variant="outline" className="min-w-0 gap-2" type="button" asChild>
              <a href={`${API_URL}/api/v1/auth/yandex`}>
                <YandexIcon className="h-5 w-5 shrink-0" />
                Яндекс
              </a>
            </Button>
          </div>

          <div className="relative">
            <div className="absolute inset-0 flex items-center">
              <Separator className="w-full" />
            </div>
            <div className="relative flex justify-center text-xs uppercase">
              <span className="bg-card px-2 text-muted-foreground">или</span>
            </div>
          </div>

          {/* Email/Phone + Password */}
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="emailOrPhone">Email или телефон</Label>
              <Input
                id="emailOrPhone"
                type="text"
                placeholder="your@email.com или +7 999 123-45-67"
                value={emailOrPhone}
                onChange={(e) => setEmailOrPhone(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Пароль</Label>
              <Input
                id="password"
                type="password"
                placeholder="••••••••"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? "Вход..." : "Войти"}
            </Button>
          </form>

          <div className="space-y-2 text-center text-sm text-muted-foreground">
            <p>
              Нет аккаунта?{" "}
              <Link href="/auth/register" className="font-medium text-primary hover:underline">
                Зарегистрироваться
              </Link>
            </p>
            <p>
              <Link href="/auth/forgot-password" className="font-medium text-primary hover:underline">
                Забыли пароль?
              </Link>
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

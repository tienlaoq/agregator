"use client"

import { useEffect, useState } from "react"
import { MailWarning } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { resendVerification, ApiError, formatApiErrorMessage } from "@/lib/api"

const RESEND_COOLDOWN_SECONDS = 60

interface Props {
  /** Email аккаунта — на него повторно уйдёт письмо. */
  email: string
}

/**
 * Блок, который показывается, когда создание бани / анкеты мастера отклонено
 * шлюзом с кодом EMAIL_NOT_VERIFIED. Объясняет причину и даёт повторно
 * отправить письмо со ссылкой (с кулдауном, чтобы не спамить SMTP). Черновик
 * формы при этом сохраняется — компонент не вызывает навигацию.
 */
export function EmailVerificationNotice({ email }: Props) {
  const [done, setDone] = useState(false)
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [cooldown, setCooldown] = useState(0)

  useEffect(() => {
    if (cooldown <= 0) return
    const id = setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => clearTimeout(id)
  }, [cooldown])

  const handleResend = async () => {
    if (loading || cooldown > 0) return
    setError("")
    setLoading(true)
    try {
      await resendVerification(email.trim())
      setDone(true)
      setCooldown(RESEND_COOLDOWN_SECONDS)
    } catch (err) {
      if (err instanceof ApiError) {
        setError(formatApiErrorMessage(err, "Не удалось отправить письмо"))
      } else {
        setError(formatApiErrorMessage(err, "Не удалось подключиться к серверу"))
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <Alert variant="destructive">
      <MailWarning />
      <AlertTitle>Подтвердите email, чтобы продолжить</AlertTitle>
      <AlertDescription>
        <p>
          Чтобы опубликовать карточку, подтвердите адрес{" "}
          <span className="font-medium">{email}</span>. Мы отправили письмо со ссылкой при
          регистрации — проверьте почту и папку «Спам». Ваш черновик сохранён.
        </p>
        {done ? (
          <p className="text-muted-foreground">
            Письмо отправлено повторно. После перехода по ссылке вернитесь сюда и нажмите «Создать»
            ещё раз.
          </p>
        ) : (
          <>
            {error && <p className="text-destructive">{error}</p>}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={handleResend}
              disabled={loading || cooldown > 0}
            >
              {loading
                ? "Отправка…"
                : cooldown > 0
                  ? `Отправить повторно через ${cooldown} с`
                  : "Отправить письмо заново"}
            </Button>
          </>
        )}
      </AlertDescription>
    </Alert>
  )
}

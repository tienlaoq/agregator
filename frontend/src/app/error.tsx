"use client"

import { useEffect } from "react"
import { Button } from "@/components/ui/button"
import { AlertTriangle } from "lucide-react"

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    // Логируем в Sentry / другой трекер ошибок, когда подключим
    console.error("[GlobalError]", error)
  }, [error])

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-6 px-4 text-center">
      <AlertTriangle className="h-12 w-12 text-destructive" />
      <div>
        <h2 className="mb-2 text-2xl font-semibold text-foreground">Что-то пошло не так</h2>
        <p className="text-muted-foreground">
          Произошла ошибка при загрузке страницы. Попробуйте ещё раз.
        </p>
        {error.digest && (
          <p className="mt-1 text-xs text-muted-foreground/60">Код: {error.digest}</p>
        )}
      </div>
      <div className="flex gap-3">
        <Button onClick={reset}>Попробовать снова</Button>
        <Button variant="outline" onClick={() => (window.location.href = "/")}>
          На главную
        </Button>
      </div>
    </div>
  )
}

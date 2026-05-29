"use client"

import { Button } from "@/components/ui/button"
import { AlertTriangle } from "lucide-react"
import { useEffect } from "react"

export default function MyError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    console.error("[MyError]", error)
  }, [error])

  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center gap-6 px-4 text-center">
      <AlertTriangle className="h-10 w-10 text-destructive" />
      <div>
        <h2 className="mb-2 text-xl font-semibold">Ошибка загрузки</h2>
        <p className="text-muted-foreground">Не удалось загрузить данные вашего кабинета.</p>
      </div>
      <Button onClick={reset}>Попробовать снова</Button>
    </div>
  )
}

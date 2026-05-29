"use client"

import { Button } from "@/components/ui/button"
import { Building2 } from "lucide-react"
import { useEffect } from "react"

export default function VenuesError({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    console.error("[VenuesError]", error)
  }, [error])

  return (
    <div className="flex min-h-[50vh] flex-col items-center justify-center gap-6 px-4 text-center">
      <Building2 className="h-12 w-12 text-muted-foreground/50" />
      <div>
        <h2 className="mb-2 text-xl font-semibold">Не удалось загрузить каталог</h2>
        <p className="text-muted-foreground">Проверьте соединение и попробуйте снова.</p>
      </div>
      <Button onClick={reset}>Обновить</Button>
    </div>
  )
}

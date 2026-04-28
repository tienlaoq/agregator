import type { ReactNode } from "react"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"

/** Стили и подписи статусов как на странице «Мои бронирования» (заведения). */
export const bookingStatusBadgeConfig: Record<string, { label: string; className: string }> = {
  pending: { label: "Ожидает", className: "bg-accent text-accent-foreground" },
  payment_pending: { label: "Ожидает оплаты", className: "bg-yellow-100 text-yellow-800" },
  confirmed: { label: "Подтверждено", className: "bg-primary/10 text-primary" },
  completed: { label: "Завершено", className: "bg-muted text-muted-foreground" },
  cancelled: { label: "Отменено", className: "bg-destructive/10 text-destructive" },
}

export function bookingStatusBadge(status: string) {
  return bookingStatusBadgeConfig[status] ?? bookingStatusBadgeConfig.pending
}

type BookingCardLayoutProps = {
  title: ReactNode
  status: string
  meta: ReactNode
  /** Доп. блок под строкой с иконками (комментарий и т.п.) */
  subMeta?: ReactNode
  aside?: ReactNode
}

/**
 * Общая сетка карточки бронирования: заголовок + бейдж, строка метаданных с иконками,
 * опционально правая колонка (цена, кнопки).
 */
export function BookingCardLayout({ title, status, meta, subMeta, aside }: BookingCardLayoutProps) {
  const badge = bookingStatusBadge(status)

  return (
    <Card className="border-border">
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0 space-y-2">
            <div className="flex flex-wrap items-center gap-3">
              <h3 className="font-semibold text-card-foreground">{title}</h3>
              <Badge className={badge.className}>{badge.label}</Badge>
            </div>
            {meta}
            {subMeta ? <div className="text-sm text-muted-foreground">{subMeta}</div> : null}
          </div>
          {aside ? <div className="flex shrink-0 flex-wrap items-center gap-4">{aside}</div> : null}
        </div>
      </CardContent>
    </Card>
  )
}

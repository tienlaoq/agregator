"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import Link from "next/link"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { getMyBookings, cancelBooking } from "@/lib/api"
import { useAuthStore } from "@/store/auth"
import type { Booking } from "@/lib/types"
import { CalendarDays, Users, Clock, X } from "lucide-react"

const statusConfig: Record<string, { label: string; className: string }> = {
  pending: { label: "Ожидает", className: "bg-accent text-accent-foreground" },
  payment_pending: { label: "Ожидает оплаты", className: "bg-yellow-100 text-yellow-800" },
  confirmed: { label: "Подтверждено", className: "bg-primary/10 text-primary" },
  completed: { label: "Завершено", className: "bg-muted text-muted-foreground" },
  cancelled: { label: "Отменено", className: "bg-destructive/10 text-destructive" },
}

function formatTime(b: Booking): string {
  if (b.time_from && b.time_to) return `${b.time_from}–${b.time_to}`
  if (b.time_from) return b.time_from
  return b.time || "—"
}

function BookingCard({ booking, onCancel, cancelling }: { booking: Booking; onCancel: (id: string) => void; cancelling: boolean }) {
  const status = statusConfig[booking.status] ?? statusConfig.pending

  return (
    <Card className="border-border">
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-2">
            <div className="flex items-center gap-3">
              <h3 className="font-semibold text-card-foreground">{booking.venue_name || "Без названия"}</h3>
              <Badge className={status.className}>{status.label}</Badge>
            </div>
            <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
              <div className="flex items-center gap-1">
                <CalendarDays className="h-4 w-4" />
                {new Date(booking.date).toLocaleDateString("ru-RU")}
              </div>
              <div className="flex items-center gap-1">
                <Clock className="h-4 w-4" />
                {formatTime(booking)}
              </div>
              <div className="flex items-center gap-1">
                <Users className="h-4 w-4" />
                {booking.guests} гостей
              </div>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <span className="text-lg font-bold text-foreground">
              {booking.total_price.toLocaleString("ru-RU")} ₽
            </span>
            {(booking.status === "pending" || booking.status === "confirmed") && (
              <Button
                variant="outline"
                size="sm"
                className="gap-1 text-destructive hover:text-destructive"
                onClick={() => onCancel(booking.id)}
                disabled={cancelling}
              >
                <X className="h-4 w-4" />
                Отменить
              </Button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function EmptyState({ title, description, showCatalogLink }: { title: string; description?: string; showCatalogLink?: boolean }) {
  return (
    <div className="flex flex-col items-center py-16 text-center">
      <CalendarDays className="mb-4 h-10 w-10 text-muted-foreground/50" />
      <h3 className="mb-2 text-lg font-semibold">{title}</h3>
      {description && <p className="mb-4 text-sm text-muted-foreground">{description}</p>}
      {showCatalogLink && (
        <Button asChild>
          <Link href="/venues">Перейти в каталог</Link>
        </Button>
      )}
    </div>
  )
}

export default function MyBookingsPage() {
  const router = useRouter()
  const { token, hydrated } = useAuthStore()
  const queryClient = useQueryClient()

  useEffect(() => {
    if (hydrated && !token) router.push("/auth/login")
  }, [hydrated, token, router])

  const { data: bookings, isLoading } = useQuery({
    queryKey: ["my-bookings"],
    queryFn: getMyBookings,
    enabled: !!token,
  })

  const cancelMutation = useMutation({
    mutationFn: cancelBooking,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["my-bookings"] })
    },
  })

  if (!hydrated || !token) return null

  const upcomingBookings = (bookings ?? []).filter((b) => b.status === "pending" || b.status === "payment_pending" || b.status === "confirmed")
  const completedBookings = (bookings ?? []).filter((b) => b.status === "completed")
  const cancelledBookings = (bookings ?? []).filter((b) => b.status === "cancelled")

  return (
    <section className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <h2 className="mb-8 text-3xl font-bold text-foreground md:text-4xl">Мои бронирования</h2>

        {isLoading ? (
          <div className="space-y-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="h-32 animate-pulse rounded-xl bg-muted" />
            ))}
          </div>
        ) : (
          <Tabs defaultValue="upcoming" className="w-full">
            <TabsList className="mb-6 w-full justify-start">
              <TabsTrigger value="upcoming" className="gap-2">
                Предстоящие
                {upcomingBookings.length > 0 && (
                  <Badge variant="secondary" className="ml-1 h-5 min-w-[20px] rounded-full px-1.5">
                    {upcomingBookings.length}
                  </Badge>
                )}
              </TabsTrigger>
              <TabsTrigger value="completed">Завершённые</TabsTrigger>
              <TabsTrigger value="cancelled">Отменённые</TabsTrigger>
            </TabsList>

            <TabsContent value="upcoming">
              {upcomingBookings.length > 0 ? (
                <div className="space-y-4">
                  {upcomingBookings.map((booking) => (
                    <BookingCard
                      key={booking.id}
                      booking={booking}
                      onCancel={(id) => cancelMutation.mutate(id)}
                      cancelling={cancelMutation.isPending}
                    />
                  ))}
                </div>
              ) : (
                <EmptyState
                  title="У вас пока нет предстоящих бронирований"
                  description="Найдите идеальную баню или сауну в нашем каталоге"
                  showCatalogLink
                />
              )}
            </TabsContent>

            <TabsContent value="completed">
              {completedBookings.length > 0 ? (
                <div className="space-y-4">
                  {completedBookings.map((booking) => (
                    <BookingCard
                      key={booking.id}
                      booking={booking}
                      onCancel={(id) => cancelMutation.mutate(id)}
                      cancelling={cancelMutation.isPending}
                    />
                  ))}
                </div>
              ) : (
                <EmptyState title="У вас пока нет завершённых бронирований" />
              )}
            </TabsContent>

            <TabsContent value="cancelled">
              {cancelledBookings.length > 0 ? (
                <div className="space-y-4">
                  {cancelledBookings.map((booking) => (
                    <BookingCard
                      key={booking.id}
                      booking={booking}
                      onCancel={(id) => cancelMutation.mutate(id)}
                      cancelling={cancelMutation.isPending}
                    />
                  ))}
                </div>
              ) : (
                <EmptyState title="У вас нет отменённых бронирований" />
              )}
            </TabsContent>
          </Tabs>
        )}
      </div>
    </section>
  )
}

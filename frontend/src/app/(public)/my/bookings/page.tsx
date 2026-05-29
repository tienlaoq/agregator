"use client"

import { useEffect, useState } from "react"
import { useRouter } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { BookingCardLayout } from "@/components/banya/booking-card-layout"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { getMyBookings, cancelBooking, getMyClientMasterBookings } from "@/lib/api"
import { BookingChatPanel } from "@/components/banya/booking-chat-panel"
import { useAuthStore } from "@/store/auth"
import type { Booking, MasterBooking } from "@/lib/types"
import { CalendarDays, Users, Clock, X } from "lucide-react"

function formatTime(b: Booking): string {
  if (b.time_from && b.time_to) return `${b.time_from}–${b.time_to}`
  if (b.time_from) return b.time_from
  return b.time || "—"
}

function BookingCard({
  booking,
  onCancel,
  cancellingId,
}: {
  booking: Booking;
  onCancel: (id: string) => void;
  /** id брони, которую сейчас отменяют; блокирует только эту кнопку. */
  cancellingId: string | null;
}) {
  const [showChat, setShowChat] = useState(false)
  return (
    <div className="space-y-3">
      <BookingCardLayout
        title={booking.venue_name || "Без названия"}
        status={booking.status}
        meta={
          <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
            <div className="flex items-center gap-1">
              <CalendarDays className="h-4 w-4 shrink-0" />
              {new Date(booking.date).toLocaleDateString("ru-RU")}
            </div>
            <div className="flex items-center gap-1">
              <Clock className="h-4 w-4 shrink-0" />
              {formatTime(booking)}
            </div>
            <div className="flex items-center gap-1">
              <Users className="h-4 w-4 shrink-0" />
              {booking.guests} гостей
            </div>
          </div>
        }
        aside={
          <>
            <span className="text-lg font-bold text-foreground">
              {booking.total_price.toLocaleString("ru-RU")} ₽
            </span>
            <Button variant="ghost" size="sm" asChild>
              <Link href={`/support?booking_id=${encodeURIComponent(booking.id)}&source=my_bookings`}>
                Поддержка
              </Link>
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowChat((v) => !v)}
            >
              {showChat ? "Скрыть чат" : "Чат по брони"}
            </Button>
            {(booking.status === "pending" || booking.status === "confirmed") && (
              <Button
                variant="outline"
                size="sm"
                className="gap-1 text-destructive hover:text-destructive"
                onClick={() => onCancel(booking.id)}
                disabled={cancellingId === booking.id}
              >
                <X className="h-4 w-4" />
                {cancellingId === booking.id ? "Отменяем…" : "Отменить"}
              </Button>
            )}
          </>
        }
      />
      {showChat ? (
        <BookingChatPanel
          kind="venue_booking"
          refId={booking.id}
          title={`Чат по брони: ${booking.venue_name || "заведение"}`}
        />
      ) : null}
    </div>
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

function MasterBookingCard({ booking }: { booking: MasterBooking }) {
  const [showChat, setShowChat] = useState(false)
  return (
    <div className="space-y-3">
      <BookingCardLayout
        title="Пар-мастер"
        status={booking.status}
        meta={
          <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
            <div className="flex items-center gap-1">
              <CalendarDays className="h-4 w-4 shrink-0" />
              {new Date(booking.date).toLocaleDateString("ru-RU")}
            </div>
            <div className="flex items-center gap-1">
              <Clock className="h-4 w-4 shrink-0" />
              {booking.time_from}–{booking.time_to}
            </div>
          </div>
        }
        aside={
          <>
            <Button variant="ghost" size="sm" asChild>
              <Link
                href={`/support?booking_id=${encodeURIComponent(booking.id)}${booking.payment_id ? `&payment_id=${encodeURIComponent(booking.payment_id)}` : ""}&source=my_bookings`}
              >
                Поддержка
              </Link>
            </Button>
            <Button variant="outline" size="sm" onClick={() => setShowChat((v) => !v)}>
              {showChat ? "Скрыть чат" : "Чат по брони"}
            </Button>
          </>
        }
      />
      {showChat ? (
        <BookingChatPanel
          kind="master_booking"
          refId={booking.id}
          title="Чат по брони: пар-мастер"
        />
      ) : null}
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
  const { data: masterBookingsResp, isLoading: isLoadingMasterBookings } = useQuery({
    queryKey: ["my-master-bookings"],
    queryFn: () => getMyClientMasterBookings(),
    enabled: !!token,
  })

  const [cancellingId, setCancellingId] = useState<string | null>(null)

  const cancelMutation = useMutation({
    mutationFn: (id: string) => {
      setCancellingId(id)
      return cancelBooking(id)
    },
    onSettled: () => setCancellingId(null),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["my-bookings"] })
    },
  })

  if (!hydrated || !token) return null

  const upcomingBookings = (bookings ?? []).filter((b) => b.status === "pending" || b.status === "payment_pending" || b.status === "confirmed")
  const completedBookings = (bookings ?? []).filter((b) => b.status === "completed")
  const cancelledBookings = (bookings ?? []).filter((b) => b.status === "cancelled")
  const masterBookings = masterBookingsResp?.bookings ?? []
  const upcomingMasterBookings = masterBookings.filter((b) => {
    const s = String(b.status || "").toLowerCase()
    return s !== "completed" && s !== "cancelled"
  })

  return (
    <section className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="mb-8 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-3xl font-bold text-foreground md:text-4xl">Мои бронирования</h2>
          <Button variant="outline" asChild>
            <Link href="/support?source=my_bookings">Связаться с поддержкой</Link>
          </Button>
        </div>

        {isLoading || isLoadingMasterBookings ? (
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
                {upcomingBookings.length + upcomingMasterBookings.length > 0 && (
                  <Badge variant="secondary" className="ml-1 h-5 min-w-[20px] rounded-full px-1.5">
                    {upcomingBookings.length + upcomingMasterBookings.length}
                  </Badge>
                )}
              </TabsTrigger>
              <TabsTrigger value="completed">Завершённые</TabsTrigger>
              <TabsTrigger value="cancelled">Отменённые</TabsTrigger>
            </TabsList>

            <TabsContent value="upcoming">
              {upcomingBookings.length > 0 || upcomingMasterBookings.length > 0 ? (
                <div className="space-y-4">
                  {upcomingBookings.map((booking) => (
                    <BookingCard
                      key={booking.id}
                      booking={booking}
                      onCancel={(id) => cancelMutation.mutate(id)}
                      cancellingId={cancellingId}
                    />
                  ))}
                  {upcomingMasterBookings.map((booking) => (
                    <MasterBookingCard key={booking.id} booking={booking} />
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
                      cancellingId={cancellingId}
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
                      cancellingId={cancellingId}
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

"use client"

import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Empty, EmptyMedia, EmptyTitle, EmptyDescription, EmptyContent } from "@/components/ui/empty"
import { CalendarDays, Users, Clock, X } from "lucide-react"

interface Booking {
  id: string
  venueName: string
  date: string
  time: string
  guests: number
  status: "upcoming" | "completed" | "cancelled"
  totalPrice: number
}

const bookings: Booking[] = [
  {
    id: "1",
    venueName: "Русская Банька на Дровах",
    date: "20 апреля 2026",
    time: "14:00",
    guests: 4,
    status: "upcoming",
    totalPrice: 4000,
  },
  {
    id: "2",
    venueName: "SPA Релакс",
    date: "25 апреля 2026",
    time: "18:00",
    guests: 2,
    status: "upcoming",
    totalPrice: 3000,
  },
  {
    id: "3",
    venueName: "Восточный Хаммам",
    date: "10 марта 2026",
    time: "16:00",
    guests: 3,
    status: "completed",
    totalPrice: 5000,
  },
  {
    id: "4",
    venueName: "Царские Бани",
    date: "5 марта 2026",
    time: "12:00",
    guests: 6,
    status: "cancelled",
    totalPrice: 9000,
  },
]

const statusConfig = {
  upcoming: { label: "Предстоящее", className: "bg-accent text-accent-foreground" },
  completed: { label: "Завершено", className: "bg-muted text-muted-foreground" },
  cancelled: { label: "Отменено", className: "bg-destructive/10 text-destructive" },
}

function BookingCard({ booking }: { booking: Booking }) {
  const status = statusConfig[booking.status]

  return (
    <Card className="border-border">
      <CardContent className="p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-2">
            <div className="flex items-center gap-3">
              <h3 className="font-semibold text-card-foreground">{booking.venueName}</h3>
              <Badge className={status.className}>{status.label}</Badge>
            </div>
            <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
              <div className="flex items-center gap-1">
                <CalendarDays className="h-4 w-4" />
                {booking.date}
              </div>
              <div className="flex items-center gap-1">
                <Clock className="h-4 w-4" />
                {booking.time}
              </div>
              <div className="flex items-center gap-1">
                <Users className="h-4 w-4" />
                {booking.guests} гостей
              </div>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <span className="text-lg font-bold text-foreground">
              {booking.totalPrice.toLocaleString("ru-RU")} ₽
            </span>
            {booking.status === "upcoming" && (
              <Button variant="outline" size="sm" className="gap-1 text-destructive hover:text-destructive">
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

export function UserDashboardSection() {
  const upcomingBookings = bookings.filter((b) => b.status === "upcoming")
  const completedBookings = bookings.filter((b) => b.status === "completed")
  const cancelledBookings = bookings.filter((b) => b.status === "cancelled")

  return (
    <section id="bookings" className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <h2 className="mb-8 text-3xl font-bold text-foreground md:text-4xl">Мои бронирования</h2>

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
                  <BookingCard key={booking.id} booking={booking} />
                ))}
              </div>
            ) : (
              <Empty>
                <EmptyMedia>
                  <CalendarDays className="h-10 w-10" />
                </EmptyMedia>
                <EmptyTitle>У вас пока нет предстоящих бронирований</EmptyTitle>
                <EmptyDescription>
                  Найдите идеальную баню или сауну в нашем каталоге
                </EmptyDescription>
                <EmptyContent>
                  <Button asChild>
                    <a href="/venues">Перейти в каталог</a>
                  </Button>
                </EmptyContent>
              </Empty>
            )}
          </TabsContent>

          <TabsContent value="completed">
            {completedBookings.length > 0 ? (
              <div className="space-y-4">
                {completedBookings.map((booking) => (
                  <BookingCard key={booking.id} booking={booking} />
                ))}
              </div>
            ) : (
              <Empty>
                <EmptyMedia>
                  <CalendarDays className="h-10 w-10" />
                </EmptyMedia>
                <EmptyTitle>У вас пока нет завершённых бронирований</EmptyTitle>
              </Empty>
            )}
          </TabsContent>

          <TabsContent value="cancelled">
            {cancelledBookings.length > 0 ? (
              <div className="space-y-4">
                {cancelledBookings.map((booking) => (
                  <BookingCard key={booking.id} booking={booking} />
                ))}
              </div>
            ) : (
              <Empty>
                <EmptyMedia>
                  <CalendarDays className="h-10 w-10" />
                </EmptyMedia>
                <EmptyTitle>У вас нет отменённых бронирований</EmptyTitle>
              </Empty>
            )}
          </TabsContent>
        </Tabs>
      </div>
    </section>
  )
}

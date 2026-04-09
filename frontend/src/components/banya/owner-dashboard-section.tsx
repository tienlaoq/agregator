"use client"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { CalendarCheck, Wallet, Star, Plus, Pencil, Eye } from "lucide-react"

const stats = [
  {
    title: "Бронирований сегодня",
    value: "5",
    icon: CalendarCheck,
  },
  {
    title: "Выручка за месяц",
    value: "125 000 ₽",
    icon: Wallet,
  },
  {
    title: "Средний рейтинг",
    value: "4.8",
    icon: Star,
  },
]

const venues = [
  {
    id: "1",
    name: "Русская Банька на Дровах",
    type: "Баня",
    rating: 4.9,
    bookingsToday: 3,
  },
  {
    id: "2",
    name: "VIP Сауна Люкс",
    type: "Сауна",
    rating: 4.7,
    bookingsToday: 2,
  },
]

const recentBookings = [
  {
    id: "1",
    guestName: "Алексей Петров",
    date: "20 апреля 2026",
    time: "14:00",
    status: "confirmed" as const,
    amount: 4000,
  },
  {
    id: "2",
    guestName: "Мария Иванова",
    date: "20 апреля 2026",
    time: "18:00",
    status: "pending" as const,
    amount: 3000,
  },
  {
    id: "3",
    guestName: "Дмитрий Сидоров",
    date: "21 апреля 2026",
    time: "12:00",
    status: "confirmed" as const,
    amount: 5000,
  },
  {
    id: "4",
    guestName: "Елена Козлова",
    date: "21 апреля 2026",
    time: "16:00",
    status: "cancelled" as const,
    amount: 3500,
  },
]

const statusConfig = {
  confirmed: { label: "Подтверждено", className: "bg-accent text-accent-foreground" },
  pending: { label: "Ожидает", className: "bg-amber-100 text-amber-800" },
  cancelled: { label: "Отменено", className: "bg-destructive/10 text-destructive" },
}

export function OwnerDashboardSection() {
  return (
    <section id="owner-dashboard" className="bg-secondary/30 py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <h2 className="text-3xl font-bold text-foreground md:text-4xl">Панель владельца</h2>
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            Добавить заведение
          </Button>
        </div>

        {/* Stats Cards */}
        <div className="mb-8 grid gap-4 sm:grid-cols-3">
          {stats.map((stat) => (
            <Card key={stat.title} className="border-border">
              <CardContent className="flex items-center gap-4 p-6">
                <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
                  <stat.icon className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground">{stat.title}</p>
                  <p className="text-2xl font-bold text-card-foreground">{stat.value}</p>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>

        <div className="grid gap-8 lg:grid-cols-2">
          {/* My Venues */}
          <Card className="border-border">
            <CardHeader>
              <CardTitle className="text-card-foreground">Мои заведения</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {venues.map((venue) => (
                  <div
                    key={venue.id}
                    className="flex flex-col gap-4 rounded-lg border border-border p-4 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div>
                      <div className="flex items-center gap-2">
                        <h3 className="font-semibold text-card-foreground">{venue.name}</h3>
                        <Badge variant="secondary">{venue.type}</Badge>
                      </div>
                      <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
                        <div className="flex items-center gap-1">
                          <Star className="h-4 w-4 fill-amber-400 text-amber-400" />
                          {venue.rating}
                        </div>
                        <span>•</span>
                        <span>{venue.bookingsToday} бронирований сегодня</span>
                      </div>
                    </div>
                    <div className="flex gap-2">
                      <Button variant="outline" size="sm" className="gap-1">
                        <Eye className="h-4 w-4" />
                        Просмотр
                      </Button>
                      <Button variant="outline" size="sm" className="gap-1">
                        <Pencil className="h-4 w-4" />
                        Редактировать
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>

          {/* Recent Bookings */}
          <Card className="border-border">
            <CardHeader>
              <CardTitle className="text-card-foreground">Последние бронирования</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Гость</TableHead>
                    <TableHead>Дата</TableHead>
                    <TableHead>Статус</TableHead>
                    <TableHead className="text-right">Сумма</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recentBookings.map((booking) => (
                    <TableRow key={booking.id}>
                      <TableCell className="font-medium">{booking.guestName}</TableCell>
                      <TableCell>
                        <div className="text-sm">
                          <div>{booking.date}</div>
                          <div className="text-muted-foreground">{booking.time}</div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge className={statusConfig[booking.status].className}>
                          {statusConfig[booking.status].label}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right font-medium">
                        {booking.amount.toLocaleString("ru-RU")} ₽
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </div>
      </div>
    </section>
  )
}

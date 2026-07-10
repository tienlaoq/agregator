"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { getMyBookings, getMyClientMasterBookings, listNotifications } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Booking } from "@/lib/types";
import { CalendarDays, Clock, Users, Bell, ArrowRight } from "lucide-react";

function greeting(): string {
  const h = new Date().getHours();
  if (h < 6) return "Доброй ночи";
  if (h < 12) return "Доброе утро";
  if (h < 18) return "Добрый день";
  return "Добрый вечер";
}

function bookingTime(b: Booking): string {
  if (b.time_from && b.time_to) return `${b.time_from}–${b.time_to}`;
  if (b.time_from) return b.time_from;
  return b.time || "—";
}

const STATUS_LABEL: Record<string, string> = {
  pending: "Ожидает",
  payment_pending: "Ожидает оплаты",
  confirmed: "Подтверждена",
  completed: "Завершена",
  cancelled: "Отменена",
};

function StatCard({ label, value, href }: { label: string; value: number; href: string }) {
  return (
    <Link
      href={href}
      className="rounded-xl bg-muted/40 p-4 transition-colors hover:bg-muted"
    >
      <div className="text-sm text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-bold">{value}</div>
    </Link>
  );
}

export default function MyOverviewPage() {
  const router = useRouter();
  const { token, hydrated, user } = useAuthStore();

  useEffect(() => {
    if (hydrated && !token) router.push("/auth/login");
  }, [hydrated, token, router]);

  const { data: bookings } = useQuery({
    queryKey: ["my-bookings"],
    queryFn: getMyBookings,
    enabled: !!token,
  });
  const { data: masterResp } = useQuery({
    queryKey: ["my-master-bookings"],
    queryFn: () => getMyClientMasterBookings(),
    enabled: !!token,
  });
  const { data: notifResp } = useQuery({
    queryKey: ["notifications", "list"],
    queryFn: () => listNotifications({ limit: 30, offset: 0 }),
    enabled: !!token,
  });

  if (!hydrated || !token) return null;

  const venueBookings = bookings ?? [];
  const masterBookings = masterResp?.bookings ?? [];

  const isUpcoming = (status: string) => {
    const s = status.toLowerCase();
    return s === "pending" || s === "payment_pending" || s === "confirmed";
  };

  const upcomingVenue = venueBookings.filter((b) => isUpcoming(b.status));
  const upcomingMaster = masterBookings.filter((b) => isUpcoming(String(b.status || "")));
  const completedCount = venueBookings.filter((b) => b.status === "completed").length;
  const upcomingCount = upcomingVenue.length + upcomingMaster.length;
  const unreadCount = notifResp?.unread_count ?? 0;

  const nextBooking = [...upcomingVenue]
    .sort((a, b) => new Date(a.date).getTime() - new Date(b.date).getTime())
    .at(0);

  return (
    <div className="mx-auto max-w-3xl px-4 py-8 md:px-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold">
          {greeting()}, {user?.name || "гость"}
        </h1>
        <p className="text-sm text-muted-foreground">
          Вот что у вас происходит в БаняГид
        </p>
      </div>

      <div className="mb-6 grid grid-cols-3 gap-3">
        <StatCard label="Предстоящие" value={upcomingCount} href="/my/bookings" />
        <StatCard label="Посещено" value={completedCount} href="/my/bookings" />
        <StatCard label="Непрочитанные" value={unreadCount} href="/my/notifications" />
      </div>

      <Card>
        <CardContent className="p-5">
          <div className="mb-3 flex items-center justify-between">
            <span className="text-sm text-muted-foreground">Ближайшая бронь</span>
            {nextBooking && (
              <Badge variant="secondary">
                {STATUS_LABEL[nextBooking.status] ?? nextBooking.status}
              </Badge>
            )}
          </div>

          {nextBooking ? (
            <>
              <div className="mb-2 text-lg font-semibold">
                {nextBooking.venue_name || "Заведение"}
              </div>
              <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <CalendarDays className="h-4 w-4 shrink-0" />
                  {new Date(nextBooking.date).toLocaleDateString("ru-RU")}
                </span>
                <span className="flex items-center gap-1.5">
                  <Clock className="h-4 w-4 shrink-0" />
                  {bookingTime(nextBooking)}
                </span>
                <span className="flex items-center gap-1.5">
                  <Users className="h-4 w-4 shrink-0" />
                  {nextBooking.guests} гостей
                </span>
                <span className="font-medium text-foreground">
                  {nextBooking.total_price.toLocaleString("ru-RU")} ₽
                </span>
              </div>
              <Button variant="ghost" size="sm" className="mt-4 gap-1 px-0" asChild>
                <Link href="/my/bookings">
                  Все бронирования
                  <ArrowRight className="h-4 w-4" />
                </Link>
              </Button>
            </>
          ) : (
            <div className="flex flex-col items-center py-8 text-center">
              <CalendarDays className="mb-3 h-9 w-9 text-muted-foreground/50" />
              <p className="mb-4 text-sm text-muted-foreground">
                У вас пока нет предстоящих бронирований
              </p>
              <Button asChild>
                <Link href="/venues">Перейти в каталог</Link>
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {unreadCount > 0 && (
        <Link
          href="/my/notifications"
          className="mt-4 flex items-center justify-between gap-3 rounded-xl bg-primary/10 px-4 py-3 text-sm text-primary transition-colors hover:bg-primary/15"
        >
          <span className="flex items-center gap-2">
            <Bell className="h-4 w-4 shrink-0" />
            {unreadCount} непрочитанных уведомлений
          </span>
          <ArrowRight className="h-4 w-4 shrink-0" />
        </Link>
      )}
    </div>
  );
}

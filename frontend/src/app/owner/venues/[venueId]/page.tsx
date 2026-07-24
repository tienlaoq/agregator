"use client";

import { useEffect, useMemo } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { AlertTriangle, ArrowRight, Loader2 } from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import { OnboardingChecklist } from "@/components/banya/onboarding-checklist";
import { getOwnerVenueBookings, getOwnerVenues } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Booking } from "@/lib/types";
import { BOOKING_STATUS_LABELS } from "@/lib/types";

// Local YYYY-MM-DD — avoids the UTC shift of toISOString() in GMT+ timezones.
function todayISO(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function formatMoney(v: number): string {
  return `${new Intl.NumberFormat("ru-RU").format(Math.round(v))} ₽`;
}

const STATUS_BADGE: Record<string, string> = {
  confirmed: "bg-primary/10 text-primary",
  completed: "bg-muted text-muted-foreground",
  pending:
    "bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-100",
  payment_pending:
    "bg-amber-100 text-amber-900 dark:bg-amber-950/40 dark:text-amber-100",
  cancelled: "bg-destructive/10 text-destructive",
};

export default function OwnerVenueTodayPage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";

  const date = todayISO();

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) redirectToLogin();
  }, [hydrated, token, canOwnerCabinet]);

  const { data: venues } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet,
  });
  const venue = venues?.find((v) => v.id === venueId) ?? null;

  const { data: bookingsData, isLoading } = useQuery({
    queryKey: ["today-bookings", venueId, date],
    queryFn: () => getOwnerVenueBookings(venueId, { date, page_size: 200 }),
    enabled: !!token && !!venue,
  });

  const bookings = useMemo(
    () =>
      [...(bookingsData?.bookings ?? [])].sort((a, b) =>
        (a.time_from || a.time || "").localeCompare(b.time_from || b.time || ""),
      ),
    [bookingsData],
  );

  const pending = bookings.filter(
    (b) => b.status === "pending" || b.status === "payment_pending",
  );
  const revenue = bookings
    .filter((b) => b.status === "confirmed" || b.status === "completed")
    .reduce((sum, b) => sum + (b.total_price || 0), 0);

  if (!hydrated || !token) return null;

  return (
    <div className="p-4 md:p-6">
      <div className="mb-5 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h1 className="text-2xl font-bold text-foreground">Сегодня</h1>
        <span className="text-sm text-muted-foreground">
          {new Date(`${date}T00:00:00`).toLocaleDateString("ru-RU", {
            weekday: "long",
            day: "2-digit",
            month: "long",
          })}
          {venue ? ` · ${venue.name}` : ""}
        </span>
      </div>

      {venue ? <OnboardingChecklist venue={venue} /> : null}

      <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MetricCard label="Брони сегодня" value={String(bookings.length)} />
        <MetricCard
          label="Ожидают оплаты"
          value={String(pending.length)}
          tone={pending.length ? "warn" : undefined}
        />
        <MetricCard label="Выручка сегодня" value={formatMoney(revenue)} />
        <MetricCard
          label="Рейтинг"
          value={(venue?.rating ?? 0).toFixed(1)}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.6fr_1fr]">
        <Card className="border-border">
          <CardHeader className="flex-row items-center justify-between space-y-0">
            <div>
              <CardTitle>Лента дня</CardTitle>
              <CardDescription>Брони на сегодня</CardDescription>
            </div>
            <Button variant="ghost" size="sm" asChild>
              <Link href={`/owner/venues/${venueId}/crm/schedule`}>
                Расписание
                <ArrowRight className="ml-1 h-4 w-4" />
              </Link>
            </Button>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="flex items-center justify-center gap-2 p-6 text-muted-foreground">
                <Loader2 className="h-5 w-5 animate-spin" />
                Загрузка…
              </div>
            ) : bookings.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                На сегодня броней нет.
              </p>
            ) : (
              <ul className="divide-y divide-border">
                {bookings.map((b: Booking) => (
                  <li key={b.id} className="flex items-center gap-3 py-2.5 text-sm">
                    <span className="w-12 shrink-0 tabular-nums text-muted-foreground">
                      {b.time_from || b.time || "—"}
                    </span>
                    <span className="flex-1 truncate font-medium text-foreground">
                      {b.user_name || "Гость"}
                    </span>
                    <span className="text-muted-foreground">{b.guests} гост.</span>
                    <Badge
                      className={`font-normal ${STATUS_BADGE[b.status] ?? ""}`}
                    >
                      {BOOKING_STATUS_LABELS[b.status] ?? b.status}
                    </Badge>
                    <span className="w-20 shrink-0 text-right tabular-nums">
                      {formatMoney(b.total_price || 0)}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card className="border-border">
          <CardHeader>
            <CardTitle>Требует внимания</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2.5 text-sm">
            {pending.length > 0 ? (
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />
                <span>
                  {pending.length} брони ждут оплаты —{" "}
                  <Link
                    href={`/owner/venues/${venueId}/crm/schedule`}
                    className="text-primary hover:underline"
                  >
                    открыть расписание
                  </Link>
                </span>
              </div>
            ) : null}
            {pending.length === 0 && !isLoading ? (
              <p className="text-muted-foreground">Всё под контролем.</p>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function MetricCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: "warn";
}) {
  return (
    <div className="rounded-lg bg-muted/40 p-3.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div
        className={`mt-1 text-2xl font-medium ${
          tone === "warn" ? "text-amber-600" : "text-foreground"
        }`}
      >
        {value}
      </div>
    </div>
  );
}

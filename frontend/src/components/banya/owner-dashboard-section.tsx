"use client";

import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { venueMediaUrl } from "@/lib/api";
import { VENUE_TYPE_LABELS, type Booking, type Venue } from "@/lib/types";
import {
  Building2,
  CalendarCheck,
  Eye,
  LayoutDashboard,
  Pencil,
  Plus,
  Star,
  Wallet,
} from "lucide-react";

export type OwnerDashboardStats = {
  bookingsToday: number;
  revenueMonthRub: number;
  avgRatingLabel: string;
};

const statusConfig: Record<
  string,
  { label: string; className: string }
> = {
  pending: { label: "Ожидает", className: "bg-accent text-accent-foreground" },
  payment_pending: {
    label: "Ожидает оплаты",
    className: "bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-100",
  },
  confirmed: {
    label: "Подтверждено",
    className: "bg-primary/10 text-primary",
  },
  completed: {
    label: "Завершено",
    className: "bg-muted text-muted-foreground",
  },
  cancelled: {
    label: "Отменено",
    className: "bg-destructive/10 text-destructive",
  },
};

/** Статус карточки заведения (модерация / каталог), не путать с status брони выше. */
const venueListingStatusConfig: Record<
  string,
  { label: string; className: string }
> = {
  draft: {
    label: "Черновик",
    className:
      "border-slate-200 bg-slate-50 text-slate-800 dark:border-slate-600 dark:bg-slate-900/50 dark:text-slate-100",
  },
  pending_review: {
    label: "На модерации",
    className:
      "border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100",
  },
  active: {
    label: "Активно",
    className:
      "border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/40 dark:text-green-100",
  },
  rejected: {
    label: "Отклонено",
    className:
      "border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/40 dark:text-red-100",
  },
  suspended: {
    label: "Приостановлено",
    className:
      "border-stone-200 bg-stone-100 text-stone-800 dark:border-stone-700 dark:bg-stone-900/50 dark:text-stone-100",
  },
};

function venueModerationBadge(venue: Venue) {
  const s = venue.status?.trim();
  if (!s) return null;
  const cfg = venueListingStatusConfig[s] ?? {
    label: s,
    className: "border-border bg-muted text-muted-foreground",
  };
  return (
    <Badge className={`border font-medium ${cfg.className}`}>{cfg.label}</Badge>
  );
}

function isoTodayLocal(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function isoYearMonthLocal(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

/** Для выручки и «бронирований сегодня» считаем только оплаченные визиты. */
function isBookingPaidForMetrics(b: Booking): boolean {
  return b.status === "confirmed" || b.status === "completed";
}

export function mergeOwnerVenueBookings(
  results: { data?: { bookings: Booking[] } }[],
): Booking[] {
  const map = new Map<string, Booking>();
  for (const r of results) {
    for (const b of r.data?.bookings ?? []) {
      map.set(b.id, b);
    }
  }
  return [...map.values()].sort(
    (a, b) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
  );
}

export function computeOwnerDashboardStats(
  venues: Venue[] | undefined,
  allBookings: Booking[],
): OwnerDashboardStats {
  const today = isoTodayLocal();
  const ym = isoYearMonthLocal(new Date());

  const bookingsToday = allBookings.filter(
    (b) => b.date === today && isBookingPaidForMetrics(b),
  ).length;

  const revenueMonthRub = allBookings
    .filter((b) => b.date.startsWith(ym) && isBookingPaidForMetrics(b))
    .reduce((s, b) => s + b.total_price, 0);

  const list = venues ?? [];
  const avgRatingLabel =
    list.length > 0
      ? (
          list.reduce((acc, v) => acc + (v.rating ?? 0), 0) / list.length
        ).toFixed(1)
      : "—";

  return { bookingsToday, revenueMonthRub, avgRatingLabel };
}

export function bookingsTodayByVenue(
  venues: Venue[] | undefined,
  allBookings: Booking[],
): Map<string, number> {
  const today = isoTodayLocal();
  const counts = new Map<string, number>();
  for (const v of venues ?? []) counts.set(v.id, 0);
  for (const b of allBookings) {
    if (b.date !== today || !isBookingPaidForMetrics(b)) continue;
    counts.set(b.venue_id, (counts.get(b.venue_id) ?? 0) + 1);
  }
  return counts;
}

function formatBookingDate(dateStr: string): string {
  const parts = dateStr.split("-").map(Number);
  const y = parts[0];
  const m = parts[1];
  const d = parts[2];
  if (!y || !m || !d) return dateStr;
  return new Date(y, m - 1, d).toLocaleDateString("ru-RU", {
    day: "numeric",
    month: "long",
    year: "numeric",
  });
}

function guestLabel(_b: Booking): string {
  return "Клиент";
}

function venueCoverSrc(v: Venue): string | undefined {
  if (v.image_url) return venueMediaUrl(v.image_url);
  const cover = v.photos?.find((p) => p.is_cover) ?? v.photos?.[0];
  return cover ? venueMediaUrl(cover.url) : undefined;
}

type OwnerDashboardSectionProps = {
  venues: Venue[] | undefined;
  isLoadingVenues: boolean;
  isLoadingBookings: boolean;
  stats: OwnerDashboardStats;
  todayBookingsByVenueId: Map<string, number>;
  recentBookings: Booking[];
  onAddVenue: () => void;
  /** Кнопка «Добавить заведение» (только владелец/мастер, не персонал с ролью user). */
  canCreateVenue?: boolean;
  /** Карточка заведения в каталоге (PATCH) — только владелец/мастер. */
  canEditVenueCard?: boolean;
};

export function OwnerDashboardSection({
  venues,
  isLoadingVenues,
  isLoadingBookings,
  stats,
  todayBookingsByVenueId,
  recentBookings,
  onAddVenue,
  canCreateVenue = true,
  canEditVenueCard = true,
}: OwnerDashboardSectionProps) {
  const statItems = [
    {
      title: "Бронирований сегодня",
      value: isLoadingVenues || isLoadingBookings ? "…" : String(stats.bookingsToday),
      icon: CalendarCheck,
    },
    {
      title: "Выручка за месяц",
      value:
        isLoadingVenues || isLoadingBookings
          ? "…"
          : `${stats.revenueMonthRub.toLocaleString("ru-RU")} ₽`,
      icon: Wallet,
    },
    {
      title: "Средний рейтинг",
      value: isLoadingVenues ? "…" : stats.avgRatingLabel,
      icon: Star,
    },
  ];

  const hasDraftVenues = (venues ?? []).some((v) => v.status === "draft");

  return (
    <section id="owner-dashboard" className="bg-secondary/30 py-10 md:py-14">
      <div className="container mx-auto px-4">
        <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <h1 className="text-3xl font-bold text-foreground md:text-4xl">
            Панель владельца
          </h1>
          {canCreateVenue ? (
            <Button type="button" className="gap-2" onClick={onAddVenue}>
              <Plus className="h-4 w-4" />
              Добавить заведение
            </Button>
          ) : null}
        </div>

        {hasDraftVenues ? (
          <div className="mb-6 rounded-lg border border-slate-200 bg-slate-50 p-4 text-sm text-slate-800 dark:border-slate-600 dark:bg-slate-900/40 dark:text-slate-100">
            У вас есть заведения в статусе «Черновик». Заполните карточку и
            нажмите «Отправить на проверку» в редактировании — до этого
            модераторы заявку не увидят.
          </div>
        ) : null}

        <div className="mb-8 grid gap-4 sm:grid-cols-3">
          {statItems.map((stat) => (
            <Card key={stat.title} className="border-border">
              <CardContent className="flex items-center gap-4 p-6">
                <div className="flex h-12 w-12 items-center justify-center rounded-lg bg-primary/10">
                  <stat.icon className="h-6 w-6 text-primary" />
                </div>
                <div>
                  <p className="text-sm text-muted-foreground">{stat.title}</p>
                  <p className="text-2xl font-bold text-card-foreground">
                    {stat.value}
                  </p>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>

        <div className="grid gap-8 lg:grid-cols-2">
          <Card className="border-border">
            <CardHeader>
              <CardTitle className="text-card-foreground">Мои заведения</CardTitle>
            </CardHeader>
            <CardContent>
              {isLoadingVenues ? (
                <div className="space-y-4">
                  {Array.from({ length: 2 }).map((_, i) => (
                    <div
                      key={i}
                      className="h-24 animate-pulse rounded-lg bg-muted"
                    />
                  ))}
                </div>
              ) : venues && venues.length > 0 ? (
                <div className="space-y-4">
                  {venues.map((venue) => {
                    const cover = venueCoverSrc(venue);
                    const todayN = todayBookingsByVenueId.get(venue.id) ?? 0;
                    return (
                      <div
                        key={venue.id}
                        className="flex flex-col gap-4 rounded-lg border border-border p-4 sm:flex-row sm:items-center sm:justify-between"
                      >
                        <div className="flex min-w-0 flex-1 gap-3">
                          <div className="hidden h-14 w-14 shrink-0 overflow-hidden rounded-lg bg-muted sm:block">
                            {cover ? (
                              // eslint-disable-next-line @next/next/no-img-element
                              <img
                                src={cover}
                                alt=""
                                className="h-full w-full object-cover"
                              />
                            ) : (
                              <div className="flex h-full w-full items-center justify-center">
                                <Building2 className="h-6 w-6 text-muted-foreground/50" />
                              </div>
                            )}
                          </div>
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-2">
                              <h3 className="font-semibold text-card-foreground">
                                {venue.name}
                              </h3>
                              {venueModerationBadge(venue)}
                              <Badge variant="secondary">
                                {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
                              </Badge>
                            </div>
                            <div className="mt-1 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
                              <div className="flex items-center gap-1">
                                <Star className="h-4 w-4 fill-amber-400 text-amber-400" />
                                {venue.rating?.toFixed(1) ?? "—"}
                              </div>
                              <span className="hidden sm:inline">•</span>
                              <span>
                                {todayN}{" "}
                                {todayN === 1
                                  ? "бронирование сегодня"
                                  : "бронирований сегодня"}
                              </span>
                            </div>
                          </div>
                        </div>
                        <div className="flex shrink-0 flex-wrap gap-2">
                          {venue.status !== "draft" ? (
                            <Button variant="outline" size="sm" className="gap-1" asChild>
                              <Link href={`/venues/${venue.slug}`}>
                                <Eye className="h-4 w-4" />
                                Просмотр
                              </Link>
                            </Button>
                          ) : null}
                          <Button variant="outline" size="sm" className="gap-1" asChild>
                            <Link href={`/owner/venues/${venue.id}/crm`}>
                              <LayoutDashboard className="h-4 w-4" />
                              CRM
                            </Link>
                          </Button>
                          {canEditVenueCard ? (
                            <Button variant="outline" size="sm" className="gap-1" asChild>
                              <Link href={`/owner/venues/${venue.id}/edit`}>
                                <Pencil className="h-4 w-4" />
                                Редактировать
                              </Link>
                            </Button>
                          ) : null}
                        </div>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <div className="flex flex-col items-center py-10 text-center">
                  <Building2 className="mb-3 h-10 w-10 text-muted-foreground/50" />
                  <p className="text-sm text-muted-foreground">
                    Заведений пока нет — добавьте первое, чтобы появились в каталоге.
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="border-border">
            <CardHeader>
              <CardTitle className="text-card-foreground">
                Последние бронирования
              </CardTitle>
            </CardHeader>
            <CardContent className="overflow-x-auto">
              {isLoadingBookings && (venues?.length ?? 0) > 0 ? (
                <div className="h-48 animate-pulse rounded-lg bg-muted" />
              ) : recentBookings.length === 0 ? (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  Пока нет бронирований по вашим заведениям.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Гость</TableHead>
                      <TableHead>Заведение</TableHead>
                      <TableHead>Дата</TableHead>
                      <TableHead>Статус</TableHead>
                      <TableHead className="text-right">Сумма</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {recentBookings.map((booking) => {
                      const st =
                        statusConfig[booking.status] ?? statusConfig.pending;
                      return (
                        <TableRow key={booking.id}>
                          <TableCell className="font-medium">
                            {guestLabel(booking)}
                          </TableCell>
                          <TableCell className="max-w-[140px] truncate text-sm">
                            {booking.venue_name || "—"}
                          </TableCell>
                          <TableCell>
                            <div className="text-sm">
                              <div>{formatBookingDate(booking.date)}</div>
                              <div className="text-muted-foreground">
                                {booking.time_from || booking.time || "—"}
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge className={st.className}>{st.label}</Badge>
                          </TableCell>
                          <TableCell className="text-right font-medium">
                            {booking.total_price.toLocaleString("ru-RU")} ₽
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </section>
  );
}

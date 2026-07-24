"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Loader2, Lock } from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import {
  getOwnerVenueBookings,
  getOwnerVenueReviewSummary,
  getOwnerVenues,
  listVenueGuests,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Booking } from "@/lib/types";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const PERIODS = [
  { days: 7, label: "7 дней" },
  { days: 30, label: "30 дней" },
  { days: 90, label: "90 дней" },
] as const;

// Local YYYY-MM-DD — avoids the UTC shift of toISOString() in GMT+ timezones.
function toLocalISO(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function formatMoney(v: number): string {
  return `${new Intl.NumberFormat("ru-RU").format(Math.round(v))} ₽`;
}

// Короткая подпись оси: 0 → «0», 6200 → «6,2 тыс.», 16800 → «17 тыс.».
const axisRub = new Intl.NumberFormat("ru-RU", {
  notation: "compact",
  maximumFractionDigits: 1,
});

/** Брони, за которые заведение получает деньги (та же логика, что на «Сегодня»). */
function isPaid(b: Booking): boolean {
  return b.status === "confirmed" || b.status === "completed";
}

// Пн-первый порядок; JS getDay(): 0=Вс..6=Сб.
const WEEKDAY_LABELS = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];
function mondayFirstIndex(iso: string): number {
  const d = new Date(`${iso}T00:00:00`);
  return (d.getDay() + 6) % 7;
}

export default function OwnerVenueAnalyticsPage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();

  const [days, setDays] = useState<number>(30);

  const validId = typeof venueId === "string" && uuidRe.test(venueId);
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) redirectToLogin();
  }, [hydrated, token, canOwnerCabinet]);

  const { dateFrom, dateTo } = useMemo(() => {
    const to = new Date();
    const from = new Date();
    from.setDate(from.getDate() - (days - 1));
    return { dateFrom: toLocalISO(from), dateTo: toLocalISO(to) };
  }, [days]);

  const { data: venues, isLoading: venuesLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet && validId,
  });
  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  const { data: bookingsData, isLoading: bookingsLoading } = useQuery({
    queryKey: ["analytics-bookings", venueId, dateFrom, dateTo],
    queryFn: () =>
      getOwnerVenueBookings(venueId as string, {
        date_from: dateFrom,
        date_to: dateTo,
        page_size: 500,
      }),
    enabled: !!token && validId && !!venue,
  });

  const { data: reviewSummary } = useQuery({
    queryKey: ["analytics-reviews", venueId],
    queryFn: () => getOwnerVenueReviewSummary(venueId as string),
    enabled: !!token && validId && !!venue,
  });

  const { data: guestsData } = useQuery({
    queryKey: ["analytics-guests", venueId],
    queryFn: () => listVenueGuests(venueId as string, { limit: 500 }),
    enabled: !!token && validId && !!venue,
  });

  const bookings = useMemo(() => bookingsData?.bookings ?? [], [bookingsData]);

  const kpi = useMemo(() => {
    const paid = bookings.filter(isPaid);
    const completed = bookings.filter((b) => b.status === "completed");
    const revenue = paid.reduce((s, b) => s + (b.total_price || 0), 0);
    return {
      revenue,
      bookings: bookings.length,
      completed: completed.length,
      avgCheck: paid.length ? revenue / paid.length : 0,
    };
  }, [bookings]);

  // Выручка по дням: непрерывная ось за весь период (нулевые дни видны как провал).
  const revenueByDay = useMemo(() => {
    const sums = new Map<string, number>();
    for (const b of bookings) {
      if (isPaid(b) && b.date) {
        sums.set(b.date, (sums.get(b.date) ?? 0) + (b.total_price || 0));
      }
    }
    const out: { date: string; label: string; revenue: number }[] = [];
    const cursor = new Date(`${dateFrom}T00:00:00`);
    const end = new Date(`${dateTo}T00:00:00`);
    while (cursor <= end) {
      const iso = toLocalISO(cursor);
      out.push({
        date: iso,
        label: `${cursor.getDate()}.${cursor.getMonth() + 1}`,
        revenue: sums.get(iso) ?? 0,
      });
      cursor.setDate(cursor.getDate() + 1);
    }
    return out;
  }, [bookings, dateFrom, dateTo]);

  const bookingsByWeekday = useMemo(() => {
    const counts = new Array(7).fill(0);
    for (const b of bookings) {
      if (b.date) counts[mondayFirstIndex(b.date)] += 1;
    }
    return WEEKDAY_LABELS.map((label, i) => ({
      label,
      count: counts[i],
      weekend: i >= 5,
    }));
  }, [bookings]);

  const funnel = useMemo(() => {
    const created = bookings.length;
    const paid = bookings.filter(isPaid).length;
    const completed = bookings.filter((b) => b.status === "completed").length;
    const cancelled = bookings.filter((b) => b.status === "cancelled").length;
    const max = Math.max(created, 1);
    return { created, paid, completed, cancelled, max };
  }, [bookings]);

  // Новые/повторные и неявки — по всей базе гостей (снимок, не за период).
  const guestSplit = useMemo(() => {
    const guests = guestsData?.guests ?? [];
    let returning = 0;
    let noShow = 0;
    for (const g of guests) {
      if (g.visits_count > 1) returning += 1;
      noShow += g.no_show_count || 0;
    }
    const total = guests.length;
    return { total, returning, fresh: total - returning, noShow };
  }, [guestsData]);

  if (!hydrated || !token) return null;

  if (!validId) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-muted-foreground">Некорректная ссылка.</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Панель</Link>
        </Button>
      </div>
    );
  }

  if (venuesLoading || !venues) {
    return (
      <div className="p-4 md:p-6">
        <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
      </div>
    );
  }

  if (!venue) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-destructive">
          Заведение не найдено в вашем доступе.
        </p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href="/owner/venues">← Панель</Link>
        </Button>
      </div>
    );
  }

  const loading = bookingsLoading;

  return (
    <div className="p-4 md:p-6">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Аналитика</h1>
          <p className="text-sm text-muted-foreground">
            {venue.name} · за {days} дней
          </p>
        </div>
        <div className="inline-flex overflow-hidden rounded-lg border border-border text-sm">
          {PERIODS.map((p) => (
            <button
              key={p.days}
              type="button"
              onClick={() => setDays(p.days)}
              className={`px-3 py-1.5 ${
                days === p.days
                  ? "bg-secondary font-medium text-secondary-foreground"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <MetricCard label="Выручка" value={formatMoney(kpi.revenue)} />
        <MetricCard
          label="Брони"
          value={String(kpi.bookings)}
          hint={`завершено ${kpi.completed}`}
        />
        <MetricCard
          label="Средний чек"
          value={formatMoney(kpi.avgCheck)}
          hint="за оплаченную бронь"
        />
        <MetricCard
          label="Рейтинг"
          value={(reviewSummary?.avg_rating ?? 0).toFixed(1)}
          hint={`по ${reviewSummary?.review_count ?? 0} отзывам`}
        />
      </div>

      <Card className="mb-4 border-border">
        <CardHeader>
          <CardTitle>Выручка по дням</CardTitle>
          <CardDescription>Пики — выходные; провалы — будни</CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <ChartLoader />
          ) : kpi.revenue === 0 ? (
            <p className="flex h-[220px] items-center justify-center text-center text-sm text-muted-foreground">
              За этот период нет оплаченных броней — выручки пока нет.
            </p>
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <AreaChart data={revenueByDay} margin={{ left: 4, right: 8 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                <XAxis
                  dataKey="label"
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  interval="preserveStartEnd"
                  minTickGap={24}
                />
                <YAxis
                  tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                  tickFormatter={(v) => axisRub.format(Number(v))}
                  width={52}
                />
                <Tooltip
                  formatter={(v) => [formatMoney(Number(v)), "Выручка"]}
                  contentStyle={{
                    background: "var(--card)",
                    border: "1px solid var(--border)",
                    borderRadius: 8,
                    fontSize: 12,
                  }}
                />
                <Area
                  type="monotone"
                  dataKey="revenue"
                  stroke="var(--chart-1)"
                  fill="var(--chart-1)"
                  fillOpacity={0.12}
                  strokeWidth={2}
                />
              </AreaChart>
            </ResponsiveContainer>
          )}
        </CardContent>
      </Card>

      <div className="mb-4 grid gap-4 lg:grid-cols-2">
        <Card className="border-border">
          <CardHeader>
            <CardTitle>Брони по дням недели</CardTitle>
            <CardDescription>Где стоите пустыми</CardDescription>
          </CardHeader>
          <CardContent>
            {loading ? (
              <ChartLoader />
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <BarChart data={bookingsByWeekday}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
                  <XAxis
                    dataKey="label"
                    tick={{ fontSize: 12, fill: "var(--muted-foreground)" }}
                  />
                  <YAxis
                    allowDecimals={false}
                    tick={{ fontSize: 11, fill: "var(--muted-foreground)" }}
                    width={28}
                  />
                  <Tooltip
                    formatter={(v) => [String(v), "Броней"]}
                    contentStyle={{
                      background: "var(--card)",
                      border: "1px solid var(--border)",
                      borderRadius: 8,
                      fontSize: 12,
                    }}
                    cursor={{ fill: "var(--muted)", opacity: 0.4 }}
                  />
                  <Bar dataKey="count" radius={[4, 4, 0, 0]} maxBarSize={40}>
                    {bookingsByWeekday.map((d) => (
                      <Cell
                        key={d.label}
                        fill={d.weekend ? "var(--chart-1)" : "var(--chart-4)"}
                      />
                    ))}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card className="border-border">
          <CardHeader>
            <CardTitle>Новые и повторные</CardTitle>
            <CardDescription>По всей базе гостей · повторные — 0% комиссии</CardDescription>
          </CardHeader>
          <CardContent>
            {guestSplit.total === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                Гостей пока нет.
              </p>
            ) : (
              <div className="flex items-center gap-6">
                <ResponsiveContainer width={140} height={140}>
                  <PieChart>
                    <Pie
                      data={[
                        { name: "Новые", value: guestSplit.fresh },
                        { name: "Повторные", value: guestSplit.returning },
                      ]}
                      dataKey="value"
                      innerRadius={40}
                      outerRadius={64}
                      strokeWidth={0}
                    >
                      <Cell fill="var(--chart-4)" />
                      <Cell fill="var(--chart-3)" />
                    </Pie>
                    <Tooltip
                      contentStyle={{
                        background: "var(--card)",
                        border: "1px solid var(--border)",
                        borderRadius: 8,
                        fontSize: 12,
                      }}
                    />
                  </PieChart>
                </ResponsiveContainer>
                <div className="space-y-2 text-sm">
                  <LegendRow color="var(--chart-4)" label="Новые" value={guestSplit.fresh} />
                  <LegendRow color="var(--chart-3)" label="Повторные" value={guestSplit.returning} />
                  {guestSplit.noShow > 0 ? (
                    <p className="pt-1 text-xs text-muted-foreground">
                      Неявки по базе: {guestSplit.noShow}
                    </p>
                  ) : null}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="mb-4 border-border">
        <CardHeader>
          <CardTitle>Воронка броней</CardTitle>
          <CardDescription>Сколько дошло до визита — и что потеряли</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {loading ? (
            <ChartLoader />
          ) : (
            <>
              <FunnelRow label="Создано" value={funnel.created} max={funnel.max} color="var(--chart-1)" />
              <FunnelRow label="Оплачено (предоплата)" value={funnel.paid} max={funnel.max} color="var(--chart-2)" />
              <FunnelRow label="Завершено" value={funnel.completed} max={funnel.max} color="var(--chart-3)" />
              <div className="pt-1">
                <div className="inline-flex items-center gap-2 rounded-lg bg-secondary px-3 py-2 text-sm text-secondary-foreground">
                  <span className="font-medium">{funnel.cancelled}</span>
                  отмены до визита
                </div>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <div className="flex items-center gap-3 rounded-xl bg-secondary px-4 py-3.5">
        <Lock className="h-5 w-5 shrink-0 text-primary" />
        <div>
          <div className="text-sm font-medium text-secondary-foreground">
            Аналитика PRO <span className="font-normal text-muted-foreground">· скоро</span>
          </div>
          <p className="text-xs text-muted-foreground">
            Сравнение периодов, источник броней (витрина / свои / виджет), когорты
            удержания, прогноз загрузки, экспорт.
          </p>
        </div>
      </div>
    </div>
  );
}

function MetricCard({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <div className="rounded-lg bg-muted/40 p-3.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-medium text-foreground">{value}</div>
      {hint ? <div className="mt-0.5 text-xs text-muted-foreground">{hint}</div> : null}
    </div>
  );
}

function LegendRow({
  color,
  label,
  value,
}: {
  color: string;
  label: string;
  value: number;
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="h-2.5 w-2.5 rounded-sm" style={{ background: color }} />
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium text-foreground">{value}</span>
    </div>
  );
}

function FunnelRow({
  label,
  value,
  max,
  color,
}: {
  label: string;
  value: number;
  max: number;
  color: string;
}) {
  const pct = Math.round((value / max) * 100);
  return (
    <div>
      <div className="mb-1 flex justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium text-foreground">{value}</span>
      </div>
      <div className="h-2.5 rounded-full bg-muted">
        <div
          className="h-2.5 rounded-full"
          style={{ width: `${pct}%`, background: color }}
        />
      </div>
    </div>
  );
}

function ChartLoader() {
  return (
    <div className="flex h-[220px] items-center justify-center text-muted-foreground">
      <Loader2 className="h-5 w-5 animate-spin" />
    </div>
  );
}

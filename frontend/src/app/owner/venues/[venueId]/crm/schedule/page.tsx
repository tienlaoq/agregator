"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import {
  ChevronLeft,
  ChevronRight,
  Loader2,
  Ban,
  Plus,
  Trash2,
} from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import {
  createOwnerSlotBlock,
  deleteOwnerSlotBlock,
  getOwnerVenueBookings,
  getOwnerVenues,
  getVenueBookingMode,
  getVenueSchedule,
  setVenueBookingMode,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Booking } from "@/lib/types";
import { BOOKING_STATUS_LABELS } from "@/lib/types";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

// Local YYYY-MM-DD — avoids the UTC shift of toISOString() (which moves the day
// back in GMT+ timezones, breaking the date arrows).
function toLocalISO(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function todayISO(): string {
  return toLocalISO(new Date());
}

function shiftISO(iso: string, days: number): string {
  const d = new Date(`${iso}T00:00:00`);
  d.setDate(d.getDate() + days);
  return toLocalISO(d);
}

function hmToMin(hm: string): number {
  const [h, m] = hm.split(":").map(Number);
  return (h || 0) * 60 + (m || 0);
}

function formatDayLabel(iso: string): string {
  const d = new Date(`${iso}T00:00:00`);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", {
    weekday: "long",
    day: "2-digit",
    month: "long",
  });
}

function formatMoney(v: number): string {
  return `${new Intl.NumberFormat("ru-RU").format(Math.round(v))} ₽`;
}

/** Цвет блока по статусу брони; ручная блокировка — нейтральный серый. */
const STATUS_BLOCK: Record<string, string> = {
  confirmed: "bg-primary/15 text-primary border-primary/30",
  completed: "bg-muted text-muted-foreground border-border",
  pending:
    "bg-amber-100 text-amber-900 border-amber-300 dark:bg-amber-950/40 dark:text-amber-100",
  payment_pending:
    "bg-amber-100 text-amber-900 border-amber-300 dark:bg-amber-950/40 dark:text-amber-100",
  cancelled: "bg-destructive/10 text-destructive border-destructive/30",
};
const MANUAL_CLASS =
  "bg-slate-200 text-slate-700 border-slate-300 dark:bg-slate-800 dark:text-slate-200";

export default function OwnerVenueSchedulePage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();

  const [date, setDate] = useState(todayISO);

  const validId = typeof venueId === "string" && uuidRe.test(venueId);
  const canOwnerCabinet =
    user?.role === "venue_owner" ||
    user?.role === "master" ||
    user?.role === "user";

  useEffect(() => {
    if (hydrated && (!token || !canOwnerCabinet)) {
      redirectToLogin();
    }
  }, [hydrated, token, canOwnerCabinet]);

  const { data: venues, isLoading: venuesLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet && validId,
  });

  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  const { data: scheduleData, isLoading: scheduleLoading } = useQuery({
    queryKey: ["venue-schedule", venueId, date],
    queryFn: () => getVenueSchedule(venueId as string, date, date),
    enabled: !!token && validId && !!venue,
  });

  const { data: bookingsData } = useQuery({
    queryKey: ["venue-schedule-bookings", venueId, date],
    queryFn: () =>
      getOwnerVenueBookings(venueId as string, { date, page_size: 200 }),
    enabled: !!token && validId && !!venue,
  });

  const queryClient = useQueryClient();
  const { data: modeData } = useQuery({
    queryKey: ["venue-booking-mode", venueId],
    queryFn: () => getVenueBookingMode(venueId as string),
    enabled: !!token && validId && !!venue,
  });
  const bookingMode = modeData?.booking_mode ?? "whole";

  const setModeMutation = useMutation({
    mutationFn: (mode: string) => setVenueBookingMode(venueId as string, mode),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["venue-booking-mode", venueId] });
      queryClient.invalidateQueries({ queryKey: ["venue-schedule", venueId] });
    },
  });

  const [formOpen, setFormOpen] = useState(false);
  const [blockFrom, setBlockFrom] = useState("10:00");
  const [blockTo, setBlockTo] = useState("12:00");
  const [blockNote, setBlockNote] = useState("");
  // "" = whole venue (all halls) in per_hall mode; a hall id targets one hall.
  const [blockHall, setBlockHall] = useState("");

  const invalidateSchedule = () =>
    queryClient.invalidateQueries({ queryKey: ["venue-schedule", venueId, date] });

  const createBlockMutation = useMutation({
    mutationFn: () =>
      createOwnerSlotBlock(venueId as string, {
        date,
        time_from: blockFrom,
        time_to: blockTo,
        note: blockNote.trim() || undefined,
        hall_ids:
          bookingMode === "per_hall" && blockHall ? [blockHall] : undefined,
      }),
    onSuccess: () => {
      setFormOpen(false);
      setBlockNote("");
      invalidateSchedule();
    },
  });

  const deleteBlockMutation = useMutation({
    mutationFn: (blockId: string) =>
      deleteOwnerSlotBlock(venueId as string, blockId),
    onSuccess: invalidateSchedule,
  });

  const entries = useMemo(
    () =>
      [...(scheduleData?.entries ?? [])].sort(
        (a, b) => hmToMin(a.time_from) - hmToMin(b.time_from),
      ),
    [scheduleData],
  );

  const bookingById = useMemo(() => {
    const map = new Map<string, Booking>();
    for (const b of bookingsData?.bookings ?? []) map.set(b.id, b);
    return map;
  }, [bookingsData]);

  const { startHour, endHour } = useMemo(() => {
    let lo = 8;
    let hi = 23;
    for (const e of entries) {
      lo = Math.min(lo, Math.floor(hmToMin(e.time_from) / 60));
      hi = Math.max(hi, Math.ceil(hmToMin(e.time_to) / 60));
    }
    return { startHour: lo, endHour: Math.max(hi, lo + 1) };
  }, [entries]);

  const rangeMin = (endHour - startHour) * 60;
  const bookingCount = entries.filter((e) => e.kind === "booking").length;
  const manualCount = entries.length - bookingCount;

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
      <div className="min-h-screen bg-muted/30 py-10">
        <div className="container mx-auto px-4">
          <div className="mx-auto max-w-5xl">
            <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
          </div>
        </div>
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

  const hourTicks = Array.from(
    { length: endHour - startHour + 1 },
    (_, i) => startHour + i,
  );

  // Group entries into grid rows. Entries carry hall_id only for per-hall
  // venues; when any are present we show a row per hall (plus a whole-venue row
  // for null-hall entries), otherwise a single venue-wide lane.
  const halls = venue.halls ?? [];
  const perHallEntries = entries.filter((e) => e.hall_id);
  const wholeEntries = entries.filter((e) => !e.hall_id);
  const hasPerHall = perHallEntries.length > 0;

  type ScheduleRow = { key: string; label: string | null; items: typeof entries };
  const rows: ScheduleRow[] = [];
  if (hasPerHall) {
    const known = new Set(halls.map((h) => h.id));
    for (const h of halls) {
      rows.push({
        key: h.id,
        label: h.name,
        items: perHallEntries.filter((e) => e.hall_id === h.id),
      });
    }
    const orphan = perHallEntries.filter((e) => !known.has(e.hall_id as string));
    if (orphan.length) {
      rows.push({ key: "__orphan", label: "Другой зал", items: orphan });
    }
    if (wholeEntries.length) {
      rows.push({ key: "__whole", label: "Всё заведение", items: wholeEntries });
    }
  } else {
    rows.push({ key: "__all", label: null, items: entries });
  }

  return (
    <section className="py-4 md:py-6">
      <div className="container mx-auto max-w-5xl px-4">
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-foreground md:text-3xl">
            Расписание: {venue.name}
          </h1>
        </div>

        <Card className="border-border">
          <CardHeader>
            <CardTitle>Загрузка дня</CardTitle>
            <CardDescription>
              Занятые интервалы за день: брони агрегатора и ручные блокировки. В
              режиме позальной сдачи — строки по залам, иначе одна лента всего
              заведения.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="Предыдущий день"
                  onClick={() => setDate((d) => shiftISO(d, -1))}
                >
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Input
                  type="date"
                  value={date}
                  onChange={(e) => setDate(e.target.value || todayISO())}
                  className="w-[160px]"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  aria-label="Следующий день"
                  onClick={() => setDate((d) => shiftISO(d, 1))}
                >
                  <ChevronRight className="h-4 w-4" />
                </Button>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setDate(todayISO())}
              >
                Сегодня
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="gap-1"
                onClick={() => setFormOpen((v) => !v)}
              >
                <Plus className="h-4 w-4" />
                Блокировка
              </Button>
              <span className="text-sm capitalize text-muted-foreground">
                {formatDayLabel(date)}
              </span>
              <div className="ml-auto flex flex-wrap items-center gap-2 text-xs">
                <Select
                  value={bookingMode}
                  onValueChange={(v) => setModeMutation.mutate(v)}
                >
                  <SelectTrigger
                    className="h-8 w-[176px] text-xs"
                    aria-label="Режим бронирования"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="whole">Режим: всё заведение</SelectItem>
                    <SelectItem value="per_hall">Режим: по залам</SelectItem>
                  </SelectContent>
                </Select>
                <Badge variant="secondary" className="font-normal">
                  Броней: {bookingCount}
                </Badge>
                <Badge variant="secondary" className="font-normal">
                  Блокировок: {manualCount}
                </Badge>
              </div>
            </div>

            {formOpen ? (
              <div className="flex flex-wrap items-end gap-3 rounded-lg border border-border p-3">
                <div className="space-y-1">
                  <Label className="text-xs">С</Label>
                  <Input
                    type="time"
                    value={blockFrom}
                    onChange={(e) => setBlockFrom(e.target.value)}
                    className="w-[112px]"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">До</Label>
                  <Input
                    type="time"
                    value={blockTo}
                    onChange={(e) => setBlockTo(e.target.value)}
                    className="w-[112px]"
                  />
                </div>
                {bookingMode === "per_hall" ? (
                  <div className="space-y-1">
                    <Label className="text-xs">Зал</Label>
                    <Select
                      value={blockHall || "all"}
                      onValueChange={(v) => setBlockHall(v === "all" ? "" : v)}
                    >
                      <SelectTrigger className="w-[180px]">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">Все залы</SelectItem>
                        {(venue.halls ?? []).map((h) => (
                          <SelectItem key={h.id} value={h.id}>
                            {h.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ) : null}
                <div className="min-w-[160px] flex-1 space-y-1">
                  <Label className="text-xs">Комментарий</Label>
                  <Input
                    value={blockNote}
                    onChange={(e) => setBlockNote(e.target.value)}
                    placeholder="напр. бронь по телефону"
                  />
                </div>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => createBlockMutation.mutate()}
                  disabled={createBlockMutation.isPending}
                >
                  {createBlockMutation.isPending ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    "Добавить"
                  )}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  onClick={() => setFormOpen(false)}
                >
                  Отмена
                </Button>
                {createBlockMutation.isError ? (
                  <p className="w-full text-xs text-destructive">
                    Не удалось создать блокировку — возможно, время пересекается
                    с занятым слотом.
                  </p>
                ) : null}
              </div>
            ) : null}

            {scheduleLoading ? (
              <div className="flex items-center justify-center gap-2 p-8 text-muted-foreground">
                <Loader2 className="h-5 w-5 animate-spin" />
                Загрузка…
              </div>
            ) : entries.length === 0 ? (
              <p className="rounded-lg border border-border p-6 text-sm text-muted-foreground">
                На этот день занятых интервалов нет.
              </p>
            ) : (
              <>
                <div className="rounded-lg border border-border p-4">
                  <div className="flex">
                    {hasPerHall ? <div className="w-28 shrink-0" /> : null}
                    <div className="relative mb-1 h-4 flex-1">
                      {hourTicks.map((h) => (
                        <span
                          key={h}
                          className="absolute -translate-x-1/2 text-[11px] tabular-nums text-muted-foreground"
                          style={{
                            left: `${((h - startHour) / (endHour - startHour)) * 100}%`,
                          }}
                        >
                          {String(h).padStart(2, "0")}
                        </span>
                      ))}
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    {rows.map((row) => (
                      <div key={row.key} className="flex items-center gap-2">
                        {hasPerHall ? (
                          <div
                            className="w-28 shrink-0 truncate text-xs font-medium text-muted-foreground"
                            title={row.label ?? undefined}
                          >
                            {row.label}
                          </div>
                        ) : null}
                        <div className="relative h-12 flex-1 rounded-md bg-muted/40">
                          {hourTicks.map((h) => (
                            <span
                              key={h}
                              className="absolute top-0 bottom-0 w-px bg-border"
                              style={{
                                left: `${((h - startHour) / (endHour - startHour)) * 100}%`,
                              }}
                            />
                          ))}
                          {row.items.map((e) => {
                            const from = hmToMin(e.time_from);
                            const to = hmToMin(e.time_to);
                            const left =
                              ((from - startHour * 60) / rangeMin) * 100;
                            const width = Math.max(
                              ((to - from) / rangeMin) * 100,
                              3,
                            );
                            const booking = e.booking_id
                              ? bookingById.get(e.booking_id)
                              : undefined;
                            const cls =
                              e.kind === "manual_block"
                                ? MANUAL_CLASS
                                : STATUS_BLOCK[booking?.status ?? "confirmed"] ??
                                  STATUS_BLOCK.confirmed;
                            const label =
                              e.kind === "manual_block"
                                ? e.note || "Блокировка"
                                : booking?.user_name || "Бронь";
                            return (
                              <div
                                key={e.id}
                                className={`absolute top-1 bottom-1 overflow-hidden rounded-md border px-2 py-1 text-xs ${cls}`}
                                style={{ left: `${left}%`, width: `${width}%` }}
                                title={`${e.time_from}–${e.time_to} · ${label}`}
                              >
                                <div className="truncate font-medium">
                                  {label}
                                </div>
                                <div className="truncate opacity-80">
                                  {e.time_from}–{e.time_to}
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <ul className="divide-y divide-border rounded-lg border border-border">
                  {entries.map((e) => {
                    const booking = e.booking_id
                      ? bookingById.get(e.booking_id)
                      : undefined;
                    return (
                      <li
                        key={e.id}
                        className="flex items-center gap-3 px-4 py-3 text-sm"
                      >
                        <span className="w-24 shrink-0 tabular-nums text-muted-foreground">
                          {e.time_from}–{e.time_to}
                        </span>
                        {hasPerHall && e.hall_id ? (
                          <Badge variant="outline" className="shrink-0 font-normal">
                            {halls.find((h) => h.id === e.hall_id)?.name ??
                              "Зал"}
                          </Badge>
                        ) : null}
                        {e.kind === "manual_block" ? (
                          <div className="flex flex-1 items-center gap-2 text-muted-foreground">
                            <Ban className="h-4 w-4" />
                            {e.note || "Ручная блокировка"}
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="ml-auto h-7 w-7"
                              aria-label="Удалить блокировку"
                              disabled={deleteBlockMutation.isPending}
                              onClick={() => deleteBlockMutation.mutate(e.id)}
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          </div>
                        ) : (
                          <div className="flex flex-1 flex-wrap items-center gap-2">
                            <span className="font-medium text-foreground">
                              {booking?.user_name || "Бронь"}
                            </span>
                            {booking ? (
                              <>
                                <Badge variant="secondary" className="font-normal">
                                  {BOOKING_STATUS_LABELS[booking.status] ??
                                    booking.status}
                                </Badge>
                                <span className="text-muted-foreground">
                                  {booking.guests} гост.
                                </span>
                                <span className="ml-auto tabular-nums text-foreground">
                                  {formatMoney(booking.total_price)}
                                </span>
                              </>
                            ) : (
                              <span className="text-xs text-muted-foreground">
                                детали брони недоступны
                              </span>
                            )}
                          </div>
                        )}
                      </li>
                    );
                  })}
                </ul>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

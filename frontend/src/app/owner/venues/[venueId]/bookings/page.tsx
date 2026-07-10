"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
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
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Loader2, MessageSquare, Search, X } from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import {
  addBookingStaffNote,
  getOwnerVenueBookings,
  getOwnerVenues,
  listBookingStaffNotes,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { Booking } from "@/lib/types";
import { BOOKING_STATUS_LABELS } from "@/lib/types";
import { formatMoney } from "@/lib/crm-guests";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const PAGE_SIZE = 30;

type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

const STATUS_VARIANT: Record<string, BadgeVariant> = {
  confirmed: "default",
  completed: "secondary",
  pending: "outline",
  payment_pending: "outline",
  cancelled: "destructive",
};

const STATUS_OPTIONS = [
  "pending",
  "payment_pending",
  "confirmed",
  "completed",
  "cancelled",
] as const;

function formatBookingDate(iso: string): string {
  const d = new Date(`${iso}T00:00:00`);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", {
    day: "2-digit",
    month: "short",
    year: "numeric",
  });
}

function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("ru-RU", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function OwnerVenueBookingsPage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();

  const [status, setStatus] = useState("");
  const [dateFrom, setDateFrom] = useState("");
  const [dateTo, setDateTo] = useState("");
  const [guestQuery, setGuestQuery] = useState("");
  // Accumulated keyset cursors; index 0 = first page (no cursor).
  const [cursors, setCursors] = useState<string[]>([""]);
  const [selected, setSelected] = useState<Booking | null>(null);

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

  // Server-side filters reset pagination to the first page.
  const resetPage = () => setCursors([""]);
  const changeStatus = (v: string) => {
    setStatus(v === "all" ? "" : v);
    resetPage();
  };
  const changeFrom = (v: string) => {
    setDateFrom(v);
    resetPage();
  };
  const changeTo = (v: string) => {
    setDateTo(v);
    resetPage();
  };
  const clearRange = () => {
    setDateFrom("");
    setDateTo("");
    resetPage();
  };

  const { data: venues, isLoading: venuesLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && canOwnerCabinet && validId,
  });

  const venue = useMemo(
    () => venues?.find((v) => v.id === venueId) ?? null,
    [venues, venueId],
  );

  const cursor = cursors[cursors.length - 1];
  const { data, isLoading, isPlaceholderData } = useQuery({
    queryKey: ["venue-bookings", venueId, status, dateFrom, dateTo, cursor],
    queryFn: () =>
      getOwnerVenueBookings(venueId as string, {
        status: status || undefined,
        date_from: dateFrom || undefined,
        date_to: dateTo || undefined,
        cursor: cursor || undefined,
        page_size: PAGE_SIZE,
      }),
    enabled: !!token && validId && !!venue,
    placeholderData: (prev) => prev,
  });

  const total = data?.total ?? 0;
  const nextCursor = data?.next_cursor ?? "";
  const page = cursors.length;

  // Guest search filters the currently loaded page only (names are resolved
  // per-page at the gateway; there is no server-side name index yet).
  const visible = useMemo(() => {
    const rows = data?.bookings ?? [];
    const q = guestQuery.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((b) => (b.user_name ?? "").toLowerCase().includes(q));
  }, [data, guestQuery]);

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

  return (
    <section className="py-4 md:py-6">
      <div className="container mx-auto max-w-5xl px-4">
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-foreground md:text-3xl">
            Брони: {venue.name}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Журнал всех броней заведения за любой период. Откройте бронь, чтобы
            посмотреть детали и вести заметки команды.
          </p>
        </div>

        <Card className="border-border">
          <CardHeader>
            <CardTitle>Реестр броней</CardTitle>
            <CardDescription>
              Фильтруйте по статусу и диапазону дат. Ежедневную ленту смотрите в
              «Сегодня», занятость залов — в «Расписании».
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-end gap-3">
              <div className="space-y-2">
                <Label>Статус</Label>
                <Select value={status || "all"} onValueChange={changeStatus}>
                  <SelectTrigger className="w-[190px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">Все статусы</SelectItem>
                    {STATUS_OPTIONS.map((s) => (
                      <SelectItem key={s} value={s}>
                        {BOOKING_STATUS_LABELS[s]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Период</Label>
                <div className="flex items-center gap-1">
                  <Input
                    type="date"
                    aria-label="Дата с"
                    value={dateFrom}
                    max={dateTo || undefined}
                    onChange={(e) => changeFrom(e.target.value)}
                    className="w-[150px]"
                  />
                  <span className="text-muted-foreground">—</span>
                  <Input
                    type="date"
                    aria-label="Дата по"
                    value={dateTo}
                    min={dateFrom || undefined}
                    onChange={(e) => changeTo(e.target.value)}
                    className="w-[150px]"
                  />
                  {dateFrom || dateTo ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label="Сбросить период"
                      onClick={clearRange}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  ) : null}
                </div>
              </div>
              <div className="min-w-[200px] flex-1 space-y-2">
                <Label>Поиск по гостю</Label>
                <div className="relative">
                  <Search className="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={guestQuery}
                    onChange={(e) => setGuestQuery(e.target.value)}
                    placeholder="Имя гостя на этой странице"
                    className="pl-8"
                  />
                </div>
              </div>
            </div>

            <div className="rounded-lg border border-border">
              {isLoading ? (
                <div className="flex items-center justify-center gap-2 p-8 text-muted-foreground">
                  <Loader2 className="h-5 w-5 animate-spin" />
                  Загрузка…
                </div>
              ) : visible.length === 0 ? (
                <p className="p-6 text-sm text-muted-foreground">
                  {guestQuery
                    ? "На этой странице нет гостя с таким именем. Попробуйте другую страницу или уточните период."
                    : status || dateFrom || dateTo
                      ? "Под фильтр ничего не подходит."
                      : "Броней пока нет. Они появятся здесь после первых бронирований гостей."}
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Дата · время</TableHead>
                      <TableHead>Гость</TableHead>
                      <TableHead className="text-right">Гости</TableHead>
                      <TableHead>Статус</TableHead>
                      <TableHead className="text-right">Заметки</TableHead>
                      <TableHead className="text-right">Сумма</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {visible.map((b: Booking) => (
                      <TableRow
                        key={b.id}
                        onClick={() => setSelected(b)}
                        className="cursor-pointer hover:bg-muted/50"
                      >
                        <TableCell>
                          <span className="font-medium text-foreground">
                            {formatBookingDate(b.date)}
                          </span>
                          <div className="mt-0.5 text-xs tabular-nums text-muted-foreground">
                            {b.time_from}
                            {b.time_to ? `–${b.time_to}` : ""}
                          </div>
                        </TableCell>
                        <TableCell className="text-sm text-foreground">
                          {b.user_name || "Гость"}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {b.guests}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant={STATUS_VARIANT[b.status] ?? "secondary"}
                            className="font-normal"
                          >
                            {BOOKING_STATUS_LABELS[b.status] ?? b.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {b.staff_notes_count ? (
                            <span className="inline-flex items-center gap-1 text-foreground">
                              <MessageSquare className="h-3.5 w-3.5 text-muted-foreground" />
                              {b.staff_notes_count}
                            </span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatMoney(b.total_price)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>

            <div className="flex items-center justify-between text-sm text-muted-foreground">
              <span>
                Всего: {total}
                {page > 1 ? ` · страница ${page}` : ""}
                {guestQuery ? ` · найдено на странице: ${visible.length}` : ""}
              </span>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page <= 1 || isPlaceholderData}
                  onClick={() => setCursors((c) => c.slice(0, -1))}
                >
                  Назад
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!nextCursor || isPlaceholderData}
                  onClick={() => setCursors((c) => [...c, nextCursor])}
                >
                  Вперёд
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Sheet
        open={!!selected}
        onOpenChange={(open) => !open && setSelected(null)}
      >
        <SheetContent className="w-full overflow-y-auto sm:max-w-md">
          {selected ? (
            <BookingDetail venueId={venueId as string} booking={selected} />
          ) : null}
        </SheetContent>
      </Sheet>
    </section>
  );
}

function DetailRow({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-right text-sm font-medium text-foreground">
        {value}
      </span>
    </div>
  );
}

function BookingDetail({
  venueId,
  booking,
}: {
  venueId: string;
  booking: Booking;
}) {
  const queryClient = useQueryClient();
  const [note, setNote] = useState("");

  const { data: notes = [], isLoading } = useQuery({
    queryKey: ["booking-staff-notes", venueId, booking.id],
    queryFn: () => listBookingStaffNotes(venueId, booking.id),
  });

  const addMutation = useMutation({
    mutationFn: () => addBookingStaffNote(venueId, booking.id, note.trim()),
    onSuccess: () => {
      setNote("");
      queryClient.invalidateQueries({
        queryKey: ["booking-staff-notes", venueId, booking.id],
      });
      // Refresh the registry so the row's notes counter reflects the new note.
      queryClient.invalidateQueries({ queryKey: ["venue-bookings", venueId] });
    },
  });

  return (
    <>
      <SheetHeader>
        <SheetTitle>{booking.user_name || "Гость"}</SheetTitle>
        <SheetDescription>
          {formatBookingDate(booking.date)} · {booking.time_from}
          {booking.time_to ? `–${booking.time_to}` : ""}
        </SheetDescription>
      </SheetHeader>

      <div className="space-y-1 px-4">
        <DetailRow
          label="Статус"
          value={
            <Badge
              variant={STATUS_VARIANT[booking.status] ?? "secondary"}
              className="font-normal"
            >
              {BOOKING_STATUS_LABELS[booking.status] ?? booking.status}
            </Badge>
          }
        />
        <DetailRow label="Гостей" value={booking.guests} />
        <DetailRow label="Сумма" value={formatMoney(booking.total_price)} />
        {booking.comment ? (
          <DetailRow label="Комментарий гостя" value={booking.comment} />
        ) : null}
        <DetailRow label="Создана" value={formatDateTime(booking.created_at)} />
      </div>

      <div className="mt-2 border-t border-border px-4 pt-4">
        <h3 className="mb-2 text-sm font-semibold text-foreground">
          Заметки команды
        </h3>

        {isLoading ? (
          <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Загрузка…
          </div>
        ) : notes.length === 0 ? (
          <p className="py-2 text-sm text-muted-foreground">
            Заметок пока нет. Оставьте первую — её увидит команда заведения.
          </p>
        ) : (
          <ul className="space-y-2">
            {notes.map((n) => (
              <li
                key={n.id}
                className="rounded-lg border border-border bg-muted/30 p-3"
              >
                <p className="whitespace-pre-wrap text-sm text-foreground">
                  {n.body}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {formatDateTime(n.created_at)}
                </p>
              </li>
            ))}
          </ul>
        )}

        <div className="mt-3 space-y-2">
          <Textarea
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="Например: гость просил веники дубовые, оплата на месте"
            rows={3}
          />
          <div className="flex items-center justify-end gap-2">
            {addMutation.isError ? (
              <span className="mr-auto text-xs text-destructive">
                Не удалось сохранить заметку.
              </span>
            ) : null}
            <Button
              type="button"
              size="sm"
              disabled={!note.trim() || addMutation.isPending}
              onClick={() => addMutation.mutate()}
            >
              {addMutation.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                "Добавить заметку"
              )}
            </Button>
          </div>
        </div>
      </div>
    </>
  );
}

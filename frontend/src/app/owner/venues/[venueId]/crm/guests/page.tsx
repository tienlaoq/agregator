"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
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
import { ArrowLeft, ChevronRight, Loader2 } from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import { getOwnerVenues, listVenueGuests } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { VenueGuest } from "@/lib/types";
import {
  formatGuestDate,
  formatMoney,
  guestDisplayName,
  guestSegmentLabel,
  guestSegmentVariant,
} from "@/lib/crm-guests";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const PAGE_SIZE = 50;

export default function OwnerVenueGuestsPage() {
  const params = useParams<{ venueId: string }>();
  const venueId = params.venueId;
  const { token, user, hydrated } = useAuthStore();

  const [segment, setSegment] = useState("");
  const [sort, setSort] = useState("last_visit");
  const [limit, setLimit] = useState(PAGE_SIZE);

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

  const { data, isLoading: guestsLoading } = useQuery({
    queryKey: ["venue-guests", venueId, segment, sort, limit],
    queryFn: () =>
      listVenueGuests(venueId as string, {
        segment: segment || undefined,
        sort,
        limit,
      }),
    enabled: !!token && validId && !!venue,
  });

  const guests = data?.guests ?? [];
  const total = data?.total ?? 0;

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
    <section className="min-h-screen bg-muted/30 py-8 md:py-10">
      <div className="container mx-auto max-w-5xl px-4">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <Button variant="ghost" size="sm" className="gap-1 px-0" asChild>
            <Link href={`/owner/venues/${venue.id}/crm`}>
              <ArrowLeft className="h-4 w-4" />
              CRM
            </Link>
          </Button>
          <h1 className="text-2xl font-bold text-foreground md:text-3xl">
            Гости: {venue.name}
          </h1>
        </div>

        <Card className="border-border">
          <CardHeader>
            <CardTitle>Customer 360</CardTitle>
            <CardDescription>
              Профили гостей по бронированиям: визиты, сумма, сегменты. Кликните
              на гостя, чтобы открыть карточку.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-end gap-3">
              <div className="space-y-2">
                <Label>Сегмент</Label>
                <Select
                  value={segment || "all"}
                  onValueChange={(v) => {
                    setSegment(v === "all" ? "" : v);
                    setLimit(PAGE_SIZE);
                  }}
                >
                  <SelectTrigger className="w-[180px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">Все</SelectItem>
                    <SelectItem value="new">Новые</SelectItem>
                    <SelectItem value="regular">Постоянные</SelectItem>
                    <SelectItem value="vip">VIP</SelectItem>
                    <SelectItem value="at_risk">В зоне риска</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Сортировка</Label>
                <Select value={sort} onValueChange={setSort}>
                  <SelectTrigger className="w-[200px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="last_visit">По последнему визиту</SelectItem>
                    <SelectItem value="ltv">По сумме (LTV)</SelectItem>
                    <SelectItem value="visits">По числу визитов</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="rounded-lg border border-border">
              {guestsLoading ? (
                <div className="flex items-center justify-center gap-2 p-8 text-muted-foreground">
                  <Loader2 className="h-5 w-5 animate-spin" />
                  Загрузка…
                </div>
              ) : guests.length === 0 ? (
                <p className="p-6 text-sm text-muted-foreground">
                  Гостей пока нет. Профили появляются после первых бронирований.
                </p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Гость</TableHead>
                      <TableHead className="text-right">Визиты</TableHead>
                      <TableHead className="text-right">Потрачено</TableHead>
                      <TableHead>Последний визит</TableHead>
                      <TableHead>Сегменты</TableHead>
                      <TableHead />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {guests.map((g: VenueGuest) => (
                      <TableRow key={g.user_id}>
                        <TableCell>
                          <Link
                            href={`/owner/venues/${venue.id}/crm/guests/${g.user_id}`}
                            className="font-medium text-foreground hover:underline"
                          >
                            {guestDisplayName(g)}
                          </Link>
                          {g.user_name && g.user_email ? (
                            <div className="mt-0.5 text-xs text-muted-foreground">
                              {g.user_email}
                            </div>
                          ) : null}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {g.visits_count}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatMoney(g.total_spent)}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {formatGuestDate(g.last_visit_at)}
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap gap-1">
                            {g.segments.map((s) => (
                              <Badge
                                key={s}
                                variant={guestSegmentVariant(s)}
                                className="font-normal"
                              >
                                {guestSegmentLabel(s)}
                              </Badge>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell className="text-right">
                          <Button variant="ghost" size="sm" asChild>
                            <Link
                              href={`/owner/venues/${venue.id}/crm/guests/${g.user_id}`}
                              aria-label="Открыть карточку гостя"
                            >
                              <ChevronRight className="h-4 w-4" />
                            </Link>
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>

            <div className="flex items-center justify-between text-sm text-muted-foreground">
              <span>
                Показано {guests.length} из {total}
              </span>
              {guests.length < total ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setLimit((n) => n + PAGE_SIZE)}
                >
                  Показать ещё
                </Button>
              ) : null}
            </div>
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

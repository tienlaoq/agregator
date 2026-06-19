"use client";

import { useEffect, useMemo } from "react";
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
import { Badge } from "@/components/ui/badge";
import { ArrowLeft } from "lucide-react";
import { redirectToLogin } from "@/lib/auth-redirect";
import { getOwnerVenues, getVenueGuest } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import type { GuestBookingSummary } from "@/lib/types";
import { BOOKING_STATUS_LABELS } from "@/lib/types";
import {
  formatGuestDate,
  formatMoney,
  guestDisplayName,
  guestSegmentLabel,
  guestSegmentVariant,
} from "@/lib/crm-guests";

const uuidRe =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-background p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-medium tabular-nums text-foreground">
        {value}
      </div>
    </div>
  );
}

export default function OwnerVenueGuestPage() {
  const params = useParams<{ venueId: string; userId: string }>();
  const venueId = params.venueId;
  const userId = params.userId;
  const { token, user, hydrated } = useAuthStore();

  const validId =
    typeof venueId === "string" &&
    uuidRe.test(venueId) &&
    typeof userId === "string" &&
    uuidRe.test(userId);
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

  const {
    data,
    isLoading: guestLoading,
    isError,
  } = useQuery({
    queryKey: ["venue-guest", venueId, userId],
    queryFn: () => getVenueGuest(venueId as string, userId as string),
    enabled: !!token && validId && !!venue,
  });

  if (!hydrated || !token) return null;

  const guestsHref = `/owner/venues/${venueId}/crm/guests`;

  if (!validId) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-muted-foreground">Некорректная ссылка.</p>
      </div>
    );
  }

  if (venuesLoading || !venues || (guestLoading && !data)) {
    return (
      <div className="min-h-screen bg-muted/30 py-10">
        <div className="container mx-auto px-4">
          <div className="mx-auto max-w-4xl">
            <div className="h-40 animate-pulse rounded-lg border border-border bg-muted" />
          </div>
        </div>
      </div>
    );
  }

  if (!venue || isError || !data?.guest) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-10">
        <p className="text-sm text-destructive">Гость не найден.</p>
        <Button asChild variant="link" className="mt-2 h-auto px-0 text-sm">
          <Link href={guestsHref}>← К списку гостей</Link>
        </Button>
      </div>
    );
  }

  const guest = data.guest;
  const bookings = data.recent_bookings;

  return (
    <section className="min-h-screen bg-muted/30 py-8 md:py-10">
      <div className="container mx-auto max-w-4xl px-4">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <Button variant="ghost" size="sm" className="gap-1 px-0" asChild>
            <Link href={guestsHref}>
              <ArrowLeft className="h-4 w-4" />
              Гости
            </Link>
          </Button>
          <h1 className="text-2xl font-bold text-foreground md:text-3xl">
            {guestDisplayName(guest)}
          </h1>
          <div className="flex flex-wrap gap-1">
            {guest.segments.map((s) => (
              <Badge key={s} variant={guestSegmentVariant(s)} className="font-normal">
                {guestSegmentLabel(s)}
              </Badge>
            ))}
          </div>
        </div>

        {guest.user_email ? (
          <p className="mb-6 text-sm text-muted-foreground">{guest.user_email}</p>
        ) : null}

        <Card className="mb-6 border-border">
          <CardHeader>
            <CardTitle>Показатели</CardTitle>
            <CardDescription>
              Агрегаты по бронированиям в этом заведении.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
              <Stat label="Визиты" value={String(guest.visits_count)} />
              <Stat label="Бронирования" value={String(guest.bookings_count)} />
              <Stat label="Потрачено (LTV)" value={formatMoney(guest.total_spent)} />
              <Stat label="Отмены" value={String(guest.cancellations_count)} />
              <Stat label="Первый визит" value={formatGuestDate(guest.first_visit_at)} />
              <Stat label="Последний визит" value={formatGuestDate(guest.last_visit_at)} />
            </div>
          </CardContent>
        </Card>

        <Card className="border-border">
          <CardHeader>
            <CardTitle>История бронирований</CardTitle>
            <CardDescription>Последние визиты гостя.</CardDescription>
          </CardHeader>
          <CardContent>
            {bookings.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Бронирований пока нет.
              </p>
            ) : (
              <ul className="space-y-2">
                {bookings.map((b: GuestBookingSummary) => (
                  <li
                    key={b.booking_id}
                    className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-4 py-3"
                  >
                    <div className="min-w-0">
                      <div className="font-medium text-foreground">
                        {formatGuestDate(b.visit_date)}
                      </div>
                      <div className="text-sm text-muted-foreground">
                        {BOOKING_STATUS_LABELS[b.status] ?? b.status}
                        {b.guests > 0 ? ` · гостей: ${b.guests}` : ""}
                      </div>
                    </div>
                    <div className="font-medium tabular-nums">
                      {formatMoney(b.total_price)}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>
    </section>
  );
}

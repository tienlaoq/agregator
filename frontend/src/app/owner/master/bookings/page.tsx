"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { useAuthStore } from "@/store/auth";
import { listMyMasterBookings, ApiError } from "@/lib/api";
import { BookingCardLayout } from "@/components/banya/booking-card-layout";
import { BookingChatPanel } from "@/components/banya/booking-chat-panel";
import type { MasterBooking } from "@/lib/types";
import { CalendarDays, Clock, Tag, Users } from "lucide-react";

function formatMasterTime(b: MasterBooking): string {
  if (b.time_from && b.time_to) return `${b.time_from}–${b.time_to}`;
  if (b.time_from) return b.time_from;
  return "—";
}

function MasterBookingCard({ booking: b }: { booking: MasterBooking }) {
  const [showChat, setShowChat] = useState(false);
  const clientRef =
    b.client_user_id.length > 10
      ? `Клиент · ${b.client_user_id.slice(0, 8)}…`
      : `Клиент · ${b.client_user_id}`;

  return (
    <div className="space-y-3">
      <BookingCardLayout
        title="Заявка на выезд"
        status={b.status}
        meta={
          <div className="flex flex-wrap gap-4 text-sm text-muted-foreground">
            <div className="flex items-center gap-1">
              <CalendarDays className="h-4 w-4 shrink-0" />
              {new Date(b.date).toLocaleDateString("ru-RU")}
            </div>
            <div className="flex items-center gap-1">
              <Clock className="h-4 w-4 shrink-0" />
              {formatMasterTime(b)}
            </div>
            <div className="flex min-w-0 items-center gap-1">
              <Users className="h-4 w-4 shrink-0" />
              <span className="truncate">{clientRef}</span>
            </div>
            {b.master_service_id ? (
              <div className="flex min-w-0 items-center gap-1">
                <Tag className="h-4 w-4 shrink-0" />
                <span className="truncate">
                  Услуга · {b.master_service_id.slice(0, 8)}
                  {b.master_service_id.length > 8 ? "…" : ""}
                </span>
              </div>
            ) : null}
          </div>
        }
        subMeta={
          <>
            {b.comment ? (
              <p className="line-clamp-2 text-foreground/90">
                <span className="text-muted-foreground">Комментарий: </span>
                {b.comment}
              </p>
            ) : null}
            <div className="pt-2">
              <Button size="sm" variant="outline" onClick={() => setShowChat((v) => !v)}>
                {showChat ? "Скрыть чат" : "Открыть чат"}
              </Button>
            </div>
          </>
        }
      />
      {showChat ? (
        <BookingChatPanel kind="master_booking" refId={b.id} title="Чат с клиентом" />
      ) : null}
    </div>
  );
}

export default function MasterBookingsPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const { data, isLoading, error } = useQuery({
    queryKey: ["my-master-bookings"],
    queryFn: () => listMyMasterBookings(),
    enabled: !!token && user?.role === "master",
    retry: false,
  });

  const noProfile = error instanceof ApiError && error.status === 404;

  if (!hydrated || !token || user?.role !== "master") return null;

  const bookings = data?.bookings ?? [];

  return (
    <div className="container mx-auto max-w-3xl px-4 py-10">
      <h1 className="mb-2 text-2xl font-bold">Входящие заявки</h1>
      <p className="mb-6 text-muted-foreground">
        Заявки от клиентов. Подтверждение записи вы согласуете с клиентом напрямую (позвонить,
        написать).
      </p>

      {isLoading && (
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-32 animate-pulse rounded-xl bg-muted" />
          ))}
        </div>
      )}
      {noProfile && (
        <p className="text-muted-foreground mb-4">
          Сначала{" "}
          <Link href="/owner/master/profile" className="text-primary underline">
            создайте профиль мастера
          </Link>
          .
        </p>
      )}
      {error && !noProfile && (
        <p className="text-destructive">Не удалось загрузить список</p>
      )}

      {!isLoading && bookings.length === 0 && (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            Пока нет заявок. После публикации профиля клиенты смогут оставлять заявки здесь.
          </CardContent>
        </Card>
      )}

      <div className="space-y-4">
        {bookings.map((b) => (
          <MasterBookingCard key={b.id} booking={b} />
        ))}
      </div>
    </div>
  );
}

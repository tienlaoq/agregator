"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useAuthStore } from "@/store/auth";
import { listMyMasterBookings, ApiError } from "@/lib/api";
import { ArrowLeft } from "lucide-react";

const STATUS_RU: Record<string, string> = {
  pending: "Новая",
  confirmed: "Подтверждена",
  cancelled: "Отменена",
};

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
      <Button variant="ghost" asChild className="mb-6 gap-2">
        <Link href="/owner/master">
          <ArrowLeft className="h-4 w-4" />
          Назад
        </Link>
      </Button>

      <h1 className="mb-2 text-2xl font-bold">Входящие заявки</h1>
      <p className="mb-6 text-muted-foreground">
        Заявки от клиентов. Подтверждение записи вы согласуете с клиентом напрямую (позвонить,
        написать).
      </p>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}
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
          <Card key={b.id}>
            <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 pb-2">
              <CardTitle className="text-base">
                {b.date} {b.time_from}–{b.time_to}
              </CardTitle>
              <Badge variant="secondary">{STATUS_RU[b.status] ?? b.status}</Badge>
            </CardHeader>
            <CardContent className="text-sm text-muted-foreground space-y-1">
              {b.master_service_id && <p>Услуга ID: {b.master_service_id}</p>}
              {b.comment && <p className="text-foreground">Комментарий: {b.comment}</p>}
              <p className="text-xs">Клиент (user id): {b.client_user_id}</p>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

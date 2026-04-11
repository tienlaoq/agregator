"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useAuthStore } from "@/store/auth";
import { getMyMasterProfile } from "@/lib/api";
import { MASTER_PROFILE_STATUS_LABELS } from "@/lib/types";
import { Users, FileEdit, CalendarDays, ArrowRight } from "lucide-react";

export default function MasterDashboardPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const { data: profile, isLoading, isSuccess } = useQuery({
    queryKey: ["my-master-profile"],
    queryFn: getMyMasterProfile,
    enabled: !!token && user?.role === "master",
    retry: false,
  });

  const notFound = isSuccess && profile === null;

  if (!hydrated || !token || user?.role !== "master") return null;

  const meta = profile ? MASTER_PROFILE_STATUS_LABELS[profile.status] : null;

  return (
    <div className="container mx-auto max-w-3xl px-4 py-10">
      <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">Кабинет пар-мастера</h1>
          <p className="text-muted-foreground">Профиль, модерация и заявки от клиентов</p>
        </div>
        <Button asChild variant="outline">
          <Link href="/masters">Каталог мастеров</Link>
        </Button>
      </div>

      {isLoading && <p className="text-muted-foreground">Загрузка...</p>}

      {notFound && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-5 w-5" />
              Создайте профиль
            </CardTitle>
            <CardDescription>
              Укажите, как вас показывать в системе, затем заполните данные для модерации.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button asChild>
              <Link href="/owner/master/profile">
                Перейти к профилю
                <ArrowRight className="ml-2 h-4 w-4" />
              </Link>
            </Button>
          </CardContent>
        </Card>
      )}

      {!isLoading && profile != null && (
        <>
          <Card className="mb-6">
            <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2">
              <div>
                <CardTitle>{profile.display_name || "Профиль мастера"}</CardTitle>
                <CardDescription className="mt-1">
                  {meta?.description ?? "Статус профиля"}
                </CardDescription>
              </div>
              <Badge variant="secondary" className="shrink-0">
                {meta?.label ?? profile.status}
              </Badge>
            </CardHeader>
            <CardContent className="flex flex-col gap-3 sm:flex-row">
              <Button asChild className="flex-1">
                <Link href="/owner/master/profile">
                  <FileEdit className="mr-2 h-4 w-4" />
                  Редактировать профиль
                </Link>
              </Button>
              <Button asChild variant="outline" className="flex-1">
                <Link href="/owner/master/bookings">
                  <CalendarDays className="mr-2 h-4 w-4" />
                  Входящие заявки
                </Link>
              </Button>
            </CardContent>
          </Card>

          {profile.status === "active" && (
            <p className="text-sm text-muted-foreground">
              Публичная страница:{" "}
              <Link href={`/masters/${profile.slug}`} className="text-primary underline">
                /masters/{profile.slug}
              </Link>
            </p>
          )}
        </>
      )}
    </div>
  );
}

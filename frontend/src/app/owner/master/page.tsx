"use client";

import { useEffect, useMemo } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAuthStore } from "@/store/auth";
import {
  getMyMasterProfile,
  listMyMasterBookings,
  getMasterRating,
  venueMediaUrl,
} from "@/lib/api";
import { MASTER_PROFILE_STATUS_LABELS } from "@/lib/types";
import {
  statusConfig,
  formatBookingDate,
  isBookingPaidForMetrics,
  isoTodayLocal,
} from "@/components/banya/owner-dashboard-section";
import { MetricCard } from "@/components/banya/dashboard-metric-card";
import {
  Users,
  FileEdit,
  CalendarDays,
  CalendarCheck,
  Wallet,
  Star,
  Eye,
  ArrowRight,
  ImageIcon,
  MapPin,
  Clock,
} from "lucide-react";

const WORK_FORMAT_LABELS: Record<string, string> = {
  venue: "В бане / у заведения",
  mobile: "Выезд к клиенту",
  both: "И то и другое",
};

/** total_price для заявок мастера хранится в копейках (s.Price / HourlyRate). */
function kopToRub(kopecks: number): number {
  return Math.round(kopecks / 100);
}

export default function MasterDashboardPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();
  const enabled = !!token && user?.role === "master";

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "master")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const {
    data: profile,
    isLoading: isLoadingProfile,
    isSuccess: profileLoaded,
  } = useQuery({
    queryKey: ["my-master-profile"],
    queryFn: getMyMasterProfile,
    enabled,
    retry: false,
  });

  const {
    data: bookingsData,
    isLoading: isLoadingBookings,
    isError: isBookingsError,
    refetch: refetchBookings,
  } = useQuery({
    queryKey: ["my-master-bookings"],
    queryFn: () => listMyMasterBookings(),
    enabled: enabled && profile != null,
  });

  const {
    data: rating,
    isLoading: isLoadingRating,
    isError: isRatingError,
  } = useQuery({
    queryKey: ["master-rating", profile?.id],
    queryFn: () => getMasterRating(profile!.id),
    enabled: enabled && !!profile?.id,
  });

  const bookings = useMemo(() => bookingsData?.bookings ?? [], [bookingsData]);

  const stats = useMemo(() => {
    const today = isoTodayLocal();
    const ym = today.slice(0, 7);
    const bookingsToday = bookings.filter(
      (b) => b.date === today && isBookingPaidForMetrics(b),
    ).length;
    const incomeMonthRub = kopToRub(
      bookings
        .filter((b) => b.date.startsWith(ym) && isBookingPaidForMetrics(b))
        .reduce((s, b) => s + (b.total_price ?? 0), 0),
    );
    return { bookingsToday, incomeMonthRub };
  }, [bookings]);

  const reviewCount = rating?.review_count ?? 0;
  const ratingLabel =
    rating && rating.review_count > 0 ? rating.avg_rating.toFixed(1) : "—";

  const serviceNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const s of profile?.services ?? []) map.set(s.id, s.name);
    return map;
  }, [profile]);

  const recentBookings = useMemo(
    () =>
      [...bookings]
        .sort(
          (a, b) =>
            new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
        )
        .slice(0, 8),
    [bookings],
  );

  if (!hydrated || !token || user?.role !== "master") return null;

  const notFound = profileLoaded && profile == null;
  const meta = profile ? MASTER_PROFILE_STATUS_LABELS[profile.status] : null;
  const isActive = profile?.status === "active";
  // Обложка выбирается так же, как на публичной карточке (sortMasterPhotosPublic):
  // по sort_order, тай-брейк по id — а не произвольный photos[0] из API.
  const coverPhoto = [...(profile?.photos ?? [])].sort(
    (a, b) => a.sort_order - b.sort_order || a.id.localeCompare(b.id),
  )[0];
  const cover = coverPhoto ? venueMediaUrl(coverPhoto.url) : undefined;

  const statItems = [
    {
      title: "Заявок сегодня",
      value: isLoadingBookings
        ? "…"
        : isBookingsError
          ? "—"
          : String(stats.bookingsToday),
      icon: CalendarCheck,
    },
    {
      title: "Доход за месяц",
      value: isLoadingBookings
        ? "…"
        : isBookingsError
          ? "—"
          : `${stats.incomeMonthRub.toLocaleString("ru-RU")} ₽`,
      icon: Wallet,
    },
    {
      title: "Рейтинг",
      value: isLoadingRating ? "…" : isRatingError ? "—" : ratingLabel,
      icon: Star,
    },
  ];

  return (
    <div className="container mx-auto max-w-5xl px-4 py-10">
      <div className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold">Кабинет пар-мастера</h1>
          <p className="text-muted-foreground">
            Профиль, модерация, заявки и доход
          </p>
        </div>
        <Button asChild variant="outline">
          <Link href="/masters">Каталог мастеров</Link>
        </Button>
      </div>

      {isLoadingProfile && <p className="text-muted-foreground">Загрузка...</p>}

      {notFound && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Users className="h-5 w-5" />
              Создайте профиль
            </CardTitle>
            <CardDescription>
              Укажите, как вас показывать в системе, затем заполните данные для
              модерации.
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

      {!isLoadingProfile && profile != null && (
        <>
          {!isActive && meta && (
            <div className="mb-6 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100">
              <p className="font-medium">{meta.label}</p>
              <p className="mt-1">{meta.description}</p>
              {(profile.status === "needs_revision" ||
                profile.status === "rejected") &&
                profile.moderation_comment?.trim() && (
                  <p className="mt-2 border-t border-amber-200/60 pt-2 dark:border-amber-900/60">
                    Комментарий модератора: {profile.moderation_comment}
                  </p>
                )}
            </div>
          )}

          <div className="mb-8 grid gap-4 sm:grid-cols-3">
            {statItems.map((stat) => (
              <MetricCard
                key={stat.title}
                title={stat.title}
                value={stat.value}
                icon={stat.icon}
              />
            ))}
          </div>

          <div className="grid gap-8 lg:grid-cols-2">
            <Card className="border-border">
              <CardHeader>
                <CardTitle className="text-card-foreground">Мой профиль</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex gap-3">
                  <div className="hidden h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-muted sm:flex">
                    {cover ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={cover}
                        alt=""
                        className="h-full w-full object-cover"
                      />
                    ) : (
                      <ImageIcon className="h-6 w-6 text-muted-foreground/50" />
                    )}
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="font-semibold text-card-foreground">
                        {profile.display_name || "Профиль мастера"}
                      </h3>
                      <Badge variant="secondary">
                        {meta?.label ?? profile.status}
                      </Badge>
                      {profile.work_format && (
                        <Badge variant="outline">
                          {WORK_FORMAT_LABELS[profile.work_format] ??
                            profile.work_format}
                        </Badge>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
                      {profile.city && (
                        <span className="flex items-center gap-1">
                          <MapPin className="h-4 w-4" />
                          {profile.city}
                        </span>
                      )}
                      <span className="flex items-center gap-1">
                        <Star className="h-4 w-4 fill-amber-400 text-amber-400" />
                        {ratingLabel}
                        {reviewCount > 0 && ` · ${reviewCount} отз.`}
                      </span>
                      {profile.experience_years > 0 && (
                        <span className="flex items-center gap-1">
                          <Clock className="h-4 w-4" />
                          Опыт {profile.experience_years} лет
                        </span>
                      )}
                    </div>
                  </div>
                </div>

                <div className="mt-4 flex flex-wrap gap-2">
                  <Button asChild>
                    <Link href="/owner/master/profile">
                      <FileEdit className="mr-2 h-4 w-4" />
                      Редактировать профиль
                    </Link>
                  </Button>
                  {isActive && profile.slug ? (
                    <Button asChild variant="outline">
                      <Link
                        href={`/masters/${profile.slug}`}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <Eye className="mr-2 h-4 w-4" />
                        Просмотр карточки
                      </Link>
                    </Button>
                  ) : (
                    <Button
                      variant="outline"
                      disabled
                      title="Доступно после публикации"
                    >
                      <Eye className="mr-2 h-4 w-4" />
                      Просмотр карточки
                    </Button>
                  )}
                  <Button asChild variant="outline">
                    <Link href="/owner/master/bookings">
                      <CalendarDays className="mr-2 h-4 w-4" />
                      Входящие заявки
                    </Link>
                  </Button>
                  <Button asChild variant="outline">
                    <Link href="/owner/master/finance">
                      <Wallet className="mr-2 h-4 w-4" />
                      Выплаты
                    </Link>
                  </Button>
                </div>

              </CardContent>
            </Card>

            <Card className="border-border">
              <CardHeader>
                <CardTitle className="text-card-foreground">
                  Последние заявки
                </CardTitle>
              </CardHeader>
              <CardContent className="overflow-x-auto">
                {isLoadingBookings ? (
                  <div className="h-48 animate-pulse rounded-lg bg-muted" />
                ) : isBookingsError ? (
                  <div className="py-8 text-center text-sm">
                    <p className="text-destructive">
                      Не удалось загрузить заявки.
                    </p>
                    <Button
                      variant="outline"
                      size="sm"
                      className="mt-3"
                      onClick={() => refetchBookings()}
                    >
                      Повторить
                    </Button>
                  </div>
                ) : recentBookings.length === 0 ? (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    Заявок пока нет. Они появятся, когда клиенты забронируют ваши
                    услуги.
                  </p>
                ) : (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Клиент</TableHead>
                        <TableHead>Услуга</TableHead>
                        <TableHead>Дата</TableHead>
                        <TableHead>Статус</TableHead>
                        <TableHead className="text-right">Сумма</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {recentBookings.map((b) => {
                        const st = statusConfig[b.status] ?? statusConfig.pending;
                        const svc = b.master_service_id
                          ? serviceNameById.get(b.master_service_id)
                          : undefined;
                        return (
                          <TableRow key={b.id}>
                            <TableCell className="font-medium">
                              {b.client_name?.trim() || "Клиент"}
                            </TableCell>
                            <TableCell className="max-w-[140px] truncate text-sm">
                              {svc ?? "Почасово"}
                            </TableCell>
                            <TableCell>
                              <div className="text-sm">
                                <div>{formatBookingDate(b.date)}</div>
                                <div className="text-muted-foreground">
                                  {b.time_from || "—"}
                                </div>
                              </div>
                            </TableCell>
                            <TableCell>
                              <Badge className={st.className}>{st.label}</Badge>
                            </TableCell>
                            <TableCell className="text-right font-medium">
                              {kopToRub(b.total_price ?? 0).toLocaleString(
                                "ru-RU",
                              )}{" "}
                              ₽
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
        </>
      )}
    </div>
  );
}

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
import { useAuthStore } from "@/store/auth";
import {
  getMyMasterProfile,
  listMyMasterBookings,
  getMasterRating,
} from "@/lib/api";
import { MASTER_PROFILE_STATUS_LABELS } from "@/lib/types";
import type { MasterBooking } from "@/lib/types";
import {
  statusConfig,
  formatBookingDate,
  isBookingPaidForMetrics,
  isoTodayLocal,
} from "@/components/banya/owner-dashboard-section";
import { cn } from "@/lib/utils";
import {
  Users,
  ArrowRight,
  CalendarCheck,
  Wallet,
  Star,
  ExternalLink,
  TrendingUp,
  MessageSquareWarning,
  HeartHandshake,
  type LucideIcon,
} from "lucide-react";

/** total_price мастер-заявок хранится в копейках (Price / HourlyRate). */
function kopToRub(kopecks: number): number {
  return Math.round(kopecks / 100);
}

function rub(value: number): string {
  return `${value.toLocaleString("ru-RU")} ₽`;
}

/** «суббота, 5 июля» — локальная дата для подзаголовка. */
function formatTodayHuman(): string {
  return new Date().toLocaleDateString("ru-RU", {
    weekday: "long",
    day: "numeric",
    month: "long",
  });
}

/** Инициалы из имени мастера для аватара («Иван Парилов» → «ИП»). */
function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "М";
  return parts
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join("");
}

/** ISO-дата N дней назад (локально). */
function isoDaysAgo(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() - days);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function prevYearMonth(ym: string): string {
  const [y, m] = ym.split("-").map(Number);
  const d = new Date(y, m - 2, 1); // m — 1-based, минус ещё месяц
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

type StatTile = {
  icon: LucideIcon;
  iconClass: string;
  value: string;
  label: string;
  sub?: React.ReactNode;
};

type Hint = {
  icon: LucideIcon;
  title: string;
  description: string;
  href?: string;
  pro?: boolean;
  tone: "info" | "warn";
};

export default function MasterTodayPage() {
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
  } = useQuery({
    queryKey: ["my-master-bookings"],
    queryFn: () => listMyMasterBookings(),
    enabled: enabled && profile != null,
  });

  const { data: rating } = useQuery({
    queryKey: ["master-rating", profile?.id],
    queryFn: () => getMasterRating(profile!.id),
    enabled: enabled && !!profile?.id,
  });

  const bookings = useMemo(() => bookingsData?.bookings ?? [], [bookingsData]);

  const serviceNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const s of profile?.services ?? []) map.set(s.id, s.name);
    return map;
  }, [profile]);

  const serviceDurationById = useMemo(() => {
    const map = new Map<string, number>();
    for (const s of profile?.services ?? []) map.set(s.id, s.duration_min);
    return map;
  }, [profile]);

  const metrics = useMemo(() => {
    const today = isoTodayLocal();
    const ym = today.slice(0, 7);
    const prevYm = prevYearMonth(ym);

    const todays = bookings.filter((b) => b.date === today && b.status !== "cancelled");
    const bookingsToday = todays.length;
    const confirmedToday = todays.filter((b) => isBookingPaidForMetrics(b)).length;

    const incomeMonth = kopToRub(
      bookings
        .filter((b) => b.date.startsWith(ym) && isBookingPaidForMetrics(b))
        .reduce((s, b) => s + (b.total_price ?? 0), 0),
    );
    const incomePrevMonth = kopToRub(
      bookings
        .filter((b) => b.date.startsWith(prevYm) && isBookingPaidForMetrics(b))
        .reduce((s, b) => s + (b.total_price ?? 0), 0),
    );
    const deltaPct =
      incomePrevMonth > 0
        ? Math.round(((incomeMonth - incomePrevMonth) / incomePrevMonth) * 100)
        : null;

    return { bookingsToday, confirmedToday, incomeMonth, deltaPct };
  }, [bookings]);

  const upcoming = useMemo(() => {
    const today = isoTodayLocal();
    return bookings
      .filter(
        (b) =>
          b.date >= today && b.status !== "cancelled" && b.status !== "completed",
      )
      .sort((a, b) =>
        a.date === b.date
          ? a.time_from.localeCompare(b.time_from)
          : a.date.localeCompare(b.date),
      )
      .slice(0, 5);
  }, [bookings]);

  const hints = useMemo<Hint[]>(() => {
    const list: Hint[] = [];
    const today = isoTodayLocal();

    // Клиенты, которых давно не было и у которых нет предстоящих визитов (Про).
    const cutoff = isoDaysAgo(60);
    const lastByClient = new Map<string, string>();
    const hasUpcoming = new Set<string>();
    for (const b of bookings) {
      if (!b.client_user_id) continue;
      const prev = lastByClient.get(b.client_user_id);
      if (!prev || b.date > prev) lastByClient.set(b.client_user_id, b.date);
      if (b.date >= today && b.status !== "cancelled") {
        hasUpcoming.add(b.client_user_id);
      }
    }
    let lapsed = 0;
    for (const [clientId, last] of lastByClient) {
      if (!hasUpcoming.has(clientId) && last < cutoff) lapsed += 1;
    }
    if (lapsed > 0) {
      list.push({
        icon: HeartHandshake,
        title: `Вернуть ${lapsed} ${plural(lapsed, "клиента", "клиентов", "клиентов")}`,
        description: "Не заходили больше 60 дней.",
        pro: true,
        tone: "info",
      });
    }

    // Заявки, ожидающие подтверждения мастером.
    const pending = bookings.filter((b) => b.status === "pending").length;
    if (pending > 0) {
      list.push({
        icon: MessageSquareWarning,
        title: `${pending} ${plural(pending, "заявка", "заявки", "заявок")} ${
          pending === 1 ? "ждёт" : "ждут"
        } ответа`,
        description: "Ответьте, пока клиент не ушёл.",
        href: "/owner/master/bookings",
        tone: "warn",
      });
    }

    return list;
  }, [bookings]);

  if (!hydrated || !token || user?.role !== "master") return null;

  const notFound = profileLoaded && profile == null;

  if (isLoadingProfile) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
        <div className="h-24 animate-pulse rounded-xl bg-muted" />
      </div>
    );
  }

  if (notFound || profile == null) {
    return (
      <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
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
      </div>
    );
  }

  const meta = MASTER_PROFILE_STATUS_LABELS[profile.status];
  const isActive = profile.status === "active";
  const subtitle = ["Пар-мастер", profile.city, formatTodayHuman()]
    .filter(Boolean)
    .join(" · ");

  const ratingLabel =
    rating && rating.review_count > 0 ? rating.avg_rating.toFixed(1) : "—";
  const reviewCount = rating?.review_count ?? 0;

  const bookingsValue = isLoadingBookings
    ? "…"
    : isBookingsError
      ? "—"
      : String(metrics.bookingsToday);
  const incomeValue = isLoadingBookings
    ? "…"
    : isBookingsError
      ? "—"
      : rub(metrics.incomeMonth);

  const tiles: StatTile[] = [
    {
      icon: CalendarCheck,
      iconClass: "bg-sky-100 text-sky-600 dark:bg-sky-950/50 dark:text-sky-300",
      value: bookingsValue,
      label: "заявки сегодня",
      sub:
        !isLoadingBookings && !isBookingsError && metrics.bookingsToday > 0 ? (
          <span className="text-muted-foreground">
            {metrics.confirmedToday} подтверждена
          </span>
        ) : null,
    },
    {
      icon: Wallet,
      iconClass:
        "bg-emerald-100 text-emerald-600 dark:bg-emerald-950/50 dark:text-emerald-300",
      value: incomeValue,
      label: "доход за месяц",
      sub:
        !isLoadingBookings && !isBookingsError && metrics.deltaPct != null ? (
          <span
            className={cn(
              "flex items-center gap-1 font-medium",
              metrics.deltaPct >= 0 ? "text-emerald-600" : "text-destructive",
            )}
          >
            <TrendingUp className="h-3.5 w-3.5" />
            {metrics.deltaPct >= 0 ? "+" : ""}
            {metrics.deltaPct}% к прошлому месяцу
          </span>
        ) : null,
    },
    {
      icon: Star,
      iconClass:
        "bg-amber-100 text-amber-600 dark:bg-amber-950/50 dark:text-amber-300",
      value: ratingLabel,
      label: "рейтинг",
      sub:
        reviewCount > 0 ? (
          <span className="text-muted-foreground">
            {reviewCount} {plural(reviewCount, "отзыв", "отзыва", "отзывов")}
          </span>
        ) : null,
    },
  ];

  return (
    <div className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      {/* Шапка: аватар, имя, статус, ссылка на карточку */}
      <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-4">
          <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-primary/10 text-lg font-semibold text-primary">
            {initials(profile.display_name || "Мастер")}
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="text-xl font-bold sm:text-2xl">
                {profile.display_name || "Профиль мастера"}
              </h1>
              <span
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium",
                  isActive
                    ? "bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300"
                    : "bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-300",
                )}
              >
                <span
                  className={cn(
                    "h-1.5 w-1.5 rounded-full",
                    isActive ? "bg-emerald-500" : "bg-amber-500",
                  )}
                />
                {meta?.label ?? profile.status}
              </span>
            </div>
            <p className="mt-0.5 truncate text-sm capitalize text-muted-foreground">
              {subtitle}
            </p>
          </div>
        </div>

        {isActive && profile.slug ? (
          <Button asChild variant="outline" size="sm">
            <Link
              href={`/masters/${profile.slug}`}
              target="_blank"
              rel="noopener noreferrer"
            >
              <ExternalLink className="mr-1.5 h-4 w-4" />
              Карточка
            </Link>
          </Button>
        ) : (
          <Button variant="outline" size="sm" disabled title="Доступно после публикации">
            <ExternalLink className="mr-1.5 h-4 w-4" />
            Карточка
          </Button>
        )}
      </header>

      {/* Плашка модерации, если профиль не в каталоге */}
      {!isActive && meta && (
        <div className="mt-6 rounded-lg border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-100">
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

      {/* Метрики */}
      <div className="mt-6 grid gap-4 sm:grid-cols-3">
        {tiles.map((t) => (
          <Card key={t.label} className="border-border">
            <CardContent className="p-5">
              <div
                className={cn(
                  "flex h-11 w-11 items-center justify-center rounded-xl",
                  t.iconClass,
                )}
              >
                <t.icon className="h-5 w-5" />
              </div>
              <p className="mt-3 text-3xl font-bold tracking-tight text-card-foreground">
                {t.value}
              </p>
              <p className="text-sm text-muted-foreground">{t.label}</p>
              {t.sub && <div className="mt-1 text-xs">{t.sub}</div>}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Ближайшие сеансы + Подсказки */}
      <div className="mt-6 grid gap-6 lg:grid-cols-[1.6fr_1fr]">
        <Card className="border-border">
          <CardContent className="p-5">
            <div className="mb-4 flex items-center justify-between">
              <h2 className="text-lg font-semibold">Ближайшие сеансы</h2>
              <Link
                href="/owner/master/bookings"
                className="text-sm font-medium text-primary hover:underline"
              >
                Все заявки
              </Link>
            </div>

            {isLoadingBookings ? (
              <div className="h-40 animate-pulse rounded-lg bg-muted" />
            ) : isBookingsError ? (
              <p className="py-8 text-center text-sm text-destructive">
                Не удалось загрузить заявки.
              </p>
            ) : upcoming.length === 0 ? (
              <p className="py-8 text-center text-sm text-muted-foreground">
                Ближайших сеансов нет. Новые заявки появятся здесь.
              </p>
            ) : (
              <ol className="relative space-y-5 pl-6">
                <span
                  aria-hidden
                  className="absolute bottom-2 left-[5px] top-2 w-px bg-border"
                />
                {upcoming.map((b) => (
                  <SessionRow
                    key={b.id}
                    booking={b}
                    today={isoTodayLocal()}
                    serviceName={
                      b.master_service_id
                        ? serviceNameById.get(b.master_service_id)
                        : undefined
                    }
                    durationMin={
                      b.master_service_id
                        ? serviceDurationById.get(b.master_service_id)
                        : undefined
                    }
                  />
                ))}
              </ol>
            )}
          </CardContent>
        </Card>

        <section>
          <h2 className="mb-4 text-lg font-semibold">Подсказки</h2>
          {hints.length === 0 ? (
            <Card className="border-dashed border-border">
              <CardContent className="p-5 text-sm text-muted-foreground">
                Всё под контролем — новых подсказок нет.
              </CardContent>
            </Card>
          ) : (
            <div className="space-y-3">
              {hints.map((h) => (
                <HintCard key={h.title} hint={h} />
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}

function SessionRow({
  booking,
  today,
  serviceName,
  durationMin,
}: {
  booking: MasterBooking;
  today: string;
  serviceName?: string;
  durationMin?: number;
}) {
  const confirmed = isBookingPaidForMetrics(booking);
  const st = statusConfig[booking.status] ?? statusConfig.pending;
  const price = kopToRub(booking.total_price ?? 0);
  const meta = [
    booking.client_name?.trim() || "Клиент",
    price > 0 ? rub(price) : null,
  ]
    .filter(Boolean)
    .join(" · ");
  const service = serviceName ?? "Почасово";

  return (
    <li className="relative">
      <span
        className={cn(
          "absolute -left-6 top-1 h-2.5 w-2.5 rounded-full border-2",
          confirmed
            ? "border-emerald-500 bg-emerald-500"
            : "border-amber-400 bg-background",
        )}
      />
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold tabular-nums">
          {booking.time_from || "—"}
        </span>
        {booking.date !== today && (
          <span className="text-xs text-muted-foreground">
            {formatBookingDate(booking.date)}
          </span>
        )}
        <span className={cn("rounded-full px-2 py-0.5 text-[11px] font-medium", st.className)}>
          {st.label}
        </span>
      </div>
      <p className="mt-0.5 text-sm">
        {service}
        {durationMin ? (
          <span className="text-muted-foreground"> · {durationMin} мин</span>
        ) : null}
      </p>
      <p className="text-sm text-muted-foreground">{meta}</p>
    </li>
  );
}

function HintCard({ hint }: { hint: Hint }) {
  const inner = (
    <Card
      className={cn(
        "border transition-colors",
        hint.tone === "warn"
          ? "border-amber-200 bg-amber-50 dark:border-amber-900/60 dark:bg-amber-950/30"
          : "border-sky-200 bg-sky-50 dark:border-sky-900/60 dark:bg-sky-950/30",
        hint.href && "hover:border-primary/40",
      )}
    >
      <CardContent className="flex items-start gap-3 p-4">
        <div
          className={cn(
            "flex h-9 w-9 shrink-0 items-center justify-center rounded-lg",
            hint.tone === "warn"
              ? "bg-amber-100 text-amber-600 dark:bg-amber-950/60 dark:text-amber-300"
              : "bg-sky-100 text-sky-600 dark:bg-sky-950/60 dark:text-sky-300",
          )}
        >
          <hint.icon className="h-5 w-5" />
        </div>
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="text-sm font-semibold">{hint.title}</p>
            {hint.pro && (
              <span className="rounded-full bg-violet-500/10 px-1.5 text-[11px] font-medium text-violet-600 dark:text-violet-300">
                Про
              </span>
            )}
          </div>
          <p className="text-sm text-muted-foreground">{hint.description}</p>
        </div>
      </CardContent>
    </Card>
  );

  return hint.href ? (
    <Link href={hint.href} className="block">
      {inner}
    </Link>
  ) : (
    inner
  );
}

/** Русское склонение по числу: 1 клиент, 2 клиента, 5 клиентов. */
function plural(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return few;
  return many;
}

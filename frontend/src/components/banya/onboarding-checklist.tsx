"use client";

import { useSyncExternalStore } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Card, CardContent } from "@/components/ui/card";
import { ArrowRight, CheckCircle2 } from "lucide-react";
import { getOwnerVenueBookings } from "@/lib/api";
import type { Venue } from "@/lib/types";

/** localStorage-ключ: владелец хоть раз открыл раздел «Виджет брони». */
export function widgetSeenKey(venueId: string): string {
  return `bg_widget_seen_${venueId}`;
}

// Пустая подписка: значение читается один раз за рендер; после навигации
// компонент перемонтируется и прочитает свежее.
const noopSubscribe = () => () => {};

/**
 * Онбординг-чеклист на «Сегодня»: путь новой бани до первой брони. Статус
 * каждого шага вычисляется из реальных данных (venue + брони), кроме шага
 * «виджет» — он отмечается по факту открытия раздела (localStorage). Карточка
 * исчезает, когда все шаги сделаны.
 */
export function OnboardingChecklist({ venue }: { venue: Venue }) {
  const base = `/owner/venues/${venue.id}`;

  // Читаем localStorage без эффекта (SSR-снимок = false, клиент перечитает).
  const widgetSeen = useSyncExternalStore(
    noopSubscribe,
    () => localStorage.getItem(widgetSeenKey(venue.id)) === "1",
    () => false,
  );

  // Есть ли хоть одна бронь за всё время (лента «Сегодня» грузит только текущий
  // день — здесь нужен отдельный лёгкий запрос на общее число).
  const { data: bookingsProbe } = useQuery({
    queryKey: ["venue-has-booking", venue.id],
    queryFn: () => getOwnerVenueBookings(venue.id, { page_size: 1 }),
  });
  const hasBooking = (bookingsProbe?.total ?? 0) > 0;

  const steps = [
    {
      label: "Карточка опубликована",
      done: venue.status === "active",
      href: `${base}/edit`,
      cta: "Заполнить",
      newTab: false,
    },
    {
      label: "Часы работы",
      done: Boolean(venue.working_hours?.trim()),
      href: `${base}/edit`,
      cta: "Указать",
      newTab: false,
    },
    {
      label: "Виджет брони для соцсетей",
      done: widgetSeen,
      href: `${base}/share`,
      cta: "Открыть",
      newTab: false,
    },
    {
      // Тестовая бронь через реальную форму на публичной странице — владелец
      // бронь в кабинете не создаёт, брони делают гости.
      label: "Первая (тестовая) бронь",
      done: hasBooking,
      href: `/venues/${venue.slug}`,
      cta: "Проверить",
      newTab: true,
    },
  ];

  const doneCount = steps.filter((s) => s.done).length;
  if (doneCount === steps.length) return null;

  const pct = Math.round((doneCount / steps.length) * 100);

  return (
    <Card className="mb-4 border-border">
      <CardContent className="p-5">
        <div className="mb-1.5 flex items-center justify-between gap-3">
          <h2 className="text-base font-medium text-foreground">
            Настройте заведение
          </h2>
          <span className="shrink-0 text-sm text-muted-foreground">
            {doneCount} из {steps.length} готово
          </span>
        </div>
        <div className="mb-1.5 h-1.5 overflow-hidden rounded-full bg-secondary">
          <div
            className="h-1.5 rounded-full bg-primary transition-all"
            style={{ width: `${pct}%` }}
          />
        </div>
        <p className="mb-1 text-sm text-muted-foreground">
          Доведите до первой брони — займёт пару минут.
        </p>

        <ul>
          {steps.map((step, i) => (
            <li
              key={step.label}
              className="flex items-center gap-3 border-t border-border py-2.5"
            >
              {step.done ? (
                <CheckCircle2 className="h-6 w-6 shrink-0 text-green-700 dark:text-green-500" />
              ) : (
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-border text-xs font-medium text-muted-foreground">
                  {i + 1}
                </span>
              )}
              <span
                className={`flex-1 text-sm ${
                  step.done
                    ? "text-muted-foreground line-through"
                    : "font-medium text-foreground"
                }`}
              >
                {step.label}
              </span>
              {step.done ? (
                <span className="text-xs text-green-700 dark:text-green-500">
                  Готово
                </span>
              ) : (
                <Link
                  href={step.href}
                  target={step.newTab ? "_blank" : undefined}
                  rel={step.newTab ? "noopener noreferrer" : undefined}
                  className="inline-flex items-center gap-1 whitespace-nowrap text-sm font-medium text-primary hover:underline"
                >
                  {step.cta}
                  <ArrowRight className="h-4 w-4" />
                </Link>
              )}
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}

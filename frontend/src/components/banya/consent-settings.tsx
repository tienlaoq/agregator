"use client"

import Link from "next/link"

import { Button } from "@/components/ui/button"
import { useCookieConsent } from "@/hooks/use-cookie-consent"
import { setConsent } from "@/lib/cookie-consent"

/**
 * Управление согласиями в личном кабинете. Реализует право субъекта на отзыв
 * согласия (152-ФЗ, ст. 9 ч. 2): согласие на аналитику (cookie/Яндекс.Метрика)
 * отзывается здесь мгновенно — hook реактивно выгружает счётчик. Отзыв согласия
 * на обработку ПДн в объёме аккаунта реализуется удалением аккаунта ниже либо
 * обращением в поддержку.
 */
export function ConsentSettings() {
  const consent = useCookieConsent()

  // undefined = ещё не смонтировано (SSR/гидрация) — не мигаем состоянием.
  const analyticsAccepted = consent === "accepted"

  return (
    <div className="space-y-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">Согласия и данные</h2>
        <p className="text-sm text-muted-foreground">
          Управляйте согласиями на обработку персональных данных.
        </p>
      </div>

      <div className="flex items-center justify-between gap-3 rounded-lg border border-border p-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">Аналитика и cookie</p>
          <p className="text-sm text-muted-foreground">
            {consent === undefined
              ? "Загрузка…"
              : analyticsAccepted
                ? "Согласие дано — Яндекс.Метрика включена."
                : "Согласие не дано — статистика не собирается."}
          </p>
        </div>
        {consent !== undefined &&
          (analyticsAccepted ? (
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => setConsent("rejected")}
            >
              Отозвать
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              onClick={() => setConsent("accepted")}
            >
              Разрешить
            </Button>
          ))}
      </div>

      <p className="text-sm text-muted-foreground">
        Отозвать согласие на обработку персональных данных, указанных в аккаунте,
        можно, удалив аккаунт ниже или написав в{" "}
        <Link href="/support" className="text-primary underline">
          поддержку
        </Link>
        . Подробнее — в{" "}
        <Link href="/consent" className="text-primary underline">
          согласии на обработку ПД
        </Link>
        .
      </p>
    </div>
  )
}

"use client"

import Link from "next/link"

import { Button } from "@/components/ui/button"
import { useCookieConsent } from "@/hooks/use-cookie-consent"
import { setConsent } from "@/lib/cookie-consent"

/**
 * Cookie/analytics consent banner. Shows once until the visitor decides; the
 * choice gates Яндекс.Метрика (see components/seo/yandex-metrika.tsx). Required
 * for 152-ФЗ / informed use of cookies and analytics.
 */
export function CookieConsent() {
  const consent = useCookieConsent()

  // undefined = not mounted yet (SSR/hydration); a stored decision = hide.
  if (consent === undefined || consent !== null) return null

  return (
    <div
      role="dialog"
      aria-label="Согласие на использование cookie"
      className="fixed inset-x-0 bottom-0 z-50 border-t border-border bg-background/95 p-4 shadow-lg backdrop-blur supports-[backdrop-filter]:bg-background/80"
    >
      <div className="container mx-auto flex max-w-5xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-sm text-muted-foreground">
          Мы используем cookie и сервисы аналитики (Яндекс.Метрика), чтобы сайт работал
          корректно и становился удобнее. Подробнее — в{" "}
          <Link href="/privacy" className="text-primary underline">
            политике конфиденциальности
          </Link>{" "}
          и{" "}
          <Link href="/consent" className="text-primary underline">
            согласии на обработку данных
          </Link>
          .
        </p>
        <div className="flex shrink-0 gap-2">
          <Button variant="outline" size="sm" onClick={() => setConsent("rejected")}>
            Только необходимые
          </Button>
          <Button size="sm" onClick={() => setConsent("accepted")}>
            Принять
          </Button>
        </div>
      </div>
    </div>
  )
}

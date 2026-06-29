"use client"

import Script from "next/script"

import { useCookieConsent } from "@/hooks/use-cookie-consent"

/**
 * Яндекс.Метрика: задайте NEXT_PUBLIC_YANDEX_METRIKA_ID (числовой id счётчика).
 * Загружается только после согласия пользователя на cookie/аналитику
 * (см. components/banya/cookie-consent.tsx) — требование 152-ФЗ.
 */
export function YandexMetrika({ nonce }: { nonce?: string }) {
  const consent = useCookieConsent()
  const id = process.env.NEXT_PUBLIC_YANDEX_METRIKA_ID?.trim()
  if (!id || !/^\d+$/.test(id)) return null
  if (consent !== "accepted") return null

  return (
    <Script
      id="yandex-metrika"
      nonce={nonce}
      strategy="afterInteractive"
      dangerouslySetInnerHTML={{
        __html: `
(function(m,e,t,r,i,k,a){m[i]=m[i]||function(){(m[i].a=m[i].a||[]).push(arguments)};
m[i].l=1*new Date();for (var j = 0; j < document.scripts.length; j++) {if (document.scripts[j].src === r) { return; }}
k=e.createElement(t),a=e.getElementsByTagName(t)[0],k.async=1,k.src=r,a.parentNode.insertBefore(k,a)})
(window, document, "script", "https://mc.yandex.ru/metrika/tag.js", "ym");
ym(${id}, "init", { clickmap:true, trackLinks:true, accurateTrackBounce:true, webvisor:false });
        `.trim(),
      }}
    />
  )
}

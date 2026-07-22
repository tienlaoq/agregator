import type { Metadata } from "next";
import { headers } from "next/headers";
import { Geist } from "next/font/google";
import { Providers } from "./providers";
import { YandexMetrika } from "@/components/seo/yandex-metrika";
import { CookieConsent } from "@/components/banya/cookie-consent";
import { siteUrl } from "@/lib/seo-site";
import "./globals.css";

const geist = Geist({
  // "cyrillic" нужен для корректного отображения русского текста без фолбэка и CLS.
  subsets: ["latin", "cyrillic"],
  display: "swap",
  adjustFontFallback: true,
});

export const metadata: Metadata = {
  metadataBase: new URL(siteUrl()),
  title: {
    default: "БаняГид — бани и сауны с отзывами и бронированием",
    template: "%s | БаняГид",
  },
  description:
    "Каталог бань и саун: сравнивайте цены, читайте отзывы, бронируйте онлайн. Подборки по городам и типу парения.",
  openGraph: {
    siteName: "БаняГид",
    locale: "ru_RU",
    type: "website",
  },
  icons: {
    icon: [
      { url: "/icon-light-32x32.png", media: "(prefers-color-scheme: light)" },
      { url: "/icon-dark-32x32.png", media: "(prefers-color-scheme: dark)" },
      { url: "/icon.svg", type: "image/svg+xml" },
    ],
    apple: "/apple-icon.png",
  },
  // Подтверждение прав в Яндекс.Вебмастере. Задайте NEXT_PUBLIC_YANDEX_VERIFICATION
  // (значение из meta yandex-verification в кабинете). Пусто → тег не рендерится.
  verification: process.env.NEXT_PUBLIC_YANDEX_VERIFICATION?.trim()
    ? { yandex: process.env.NEXT_PUBLIC_YANDEX_VERIFICATION.trim() }
    : undefined,
};

export default async function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  // Nonce is injected per-request by src/proxy.ts; pass it to the inline Метрика
  // script so it satisfies the strict-dynamic CSP.
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return (
    // suppressHydrationWarning нужен при использовании next-themes (класс темы пишется на <html>).
    <html lang="ru" suppressHydrationWarning>
      <body className={`${geist.className} font-sans antialiased`}>
        <YandexMetrika nonce={nonce} />
        <Providers>
          {/*
           * AppLayout (header + footer + chat-widget) вынесен отсюда в отдельный
           * src/app/(public)/layout.tsx — чтобы /admin, /owner, /auth работали
           * без публичной навигации. Каждый маршрут сам выбирает свой layout.
           */}
          {children}
        </Providers>
        <CookieConsent />
      </body>
    </html>
  );
}

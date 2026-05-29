import type { Metadata } from "next";
import { Geist } from "next/font/google";
import { Providers } from "./providers";
import { YandexMetrika } from "@/components/seo/yandex-metrika";
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
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    // suppressHydrationWarning нужен при использовании next-themes (класс темы пишется на <html>).
    <html lang="ru" suppressHydrationWarning>
      <body className={`${geist.className} font-sans antialiased`}>
        <YandexMetrika />
        <Providers>
          {/*
           * AppLayout (header + footer + chat-widget) вынесен отсюда в отдельный
           * src/app/(public)/layout.tsx — чтобы /admin, /owner, /auth работали
           * без публичной навигации. Каждый маршрут сам выбирает свой layout.
           */}
          {children}
        </Providers>
      </body>
    </html>
  );
}

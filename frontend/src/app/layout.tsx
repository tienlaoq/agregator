import type { Metadata } from "next";
import { Geist } from "next/font/google";
import { Providers } from "./providers";
import { AppLayout } from "./app-layout";
import "./globals.css";

const geist = Geist({
  subsets: ["latin"],
  display: "swap",
  adjustFontFallback: true,
});

export const metadata: Metadata = {
  title: "БаняГид — Найди идеальную баню или сауну",
  description: "Агрегатор бань и саун России. Более 500 заведений с отзывами, ценами и онлайн-бронированием.",
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
    <html lang="ru">
      <body className={`${geist.className} font-sans antialiased`}>
        <Providers>
          <AppLayout>{children}</AppLayout>
        </Providers>
      </body>
    </html>
  );
}

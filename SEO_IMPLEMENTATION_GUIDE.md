# SEO Реализация — Практический гайд с примерами кода

## 🚀 Быстрый старт

### 1. Добавить Breadcrumb Schema (10 минут)

**Файл: `src/lib/breadcrumb-jsonld.ts`** (создать новый)

```typescript
/**
 * Breadcrumb Schema для навигационных цепочек.
 * Помогает Google показывать хлебные крошки в результатах поиска.
 */

export type BreadcrumbItem = {
  label: string
  url: string
}

export function breadcrumbJsonLd(items: BreadcrumbItem[]): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: items.map((item, idx) => ({
      "@type": "ListItem",
      position: idx + 1,
      name: item.label,
      item: item.url,
    })),
  }
}
```

**Файл: `src/app/(public)/venues/city/[citySlug]/page.tsx`** (добавить)

```typescript
import { breadcrumbJsonLd } from "@/lib/breadcrumb-jsonld"
import { safeJsonLdStringify } from "@/lib/seo-jsonld"
import { siteUrl } from "@/lib/seo-site"

// ... существующий код

export default async function CityPage({
  params,
}: {
  params: Promise<{ citySlug: string }>
}) {
  const { citySlug } = await params
  const base = siteUrl()
  
  // Найти город в SEO_CITY_HUBS
  const city = SEO_CITY_HUBS.find(c => c.slug === citySlug)
  if (!city) notFound()
  
  const breadcrumbs = breadcrumbJsonLd([
    { label: "Главная", url: base },
    { label: "Заведения", url: `${base}/venues` },
    { label: city.name, url: `${base}/venues/city/${citySlug}` },
  ])

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(breadcrumbs) }}
      />
      {/* остальной контент */}
    </>
  )
}
```

**Файл: `src/app/(public)/venues/[slug]/page.tsx`** (добавить перед VenuePublicPageClient)

```typescript
// В venueDetailPage:
const breadcrumbs = breadcrumbJsonLd([
  { label: "Главная", url: siteUrl() },
  { label: "Заведения", url: `${siteUrl()}/venues` },
  { label: venue.city, url: `${siteUrl()}/venues/city/${slugify(venue.city)}` },
  { label: venue.name, url: canonicalUrl },
])

return (
  <>
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(jsonLd) }}
    />
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(breadcrumbs) }}
    />
    <VenuePublicPageClient slug={slug} initialVenue={venue} />
  </>
)
```

---

### 2. Оптимизировать изображения (20 минут)

**Найти все `<img>` и заменить на `<Image>`:**

```bash
# Поиск проблемных изображений
grep -r "<img" frontend/src --include="*.tsx" | grep -v "Image from"
```

**Пример: HeroSection**

```tsx
// БЫЛО:
<img src="/hero.jpg" alt="Hero" />

// СТАЛО:
import Image from "next/image"

<Image
  src="/hero.jpg"
  alt="БаняГид — каталог бань и саун"
  width={1200}
  height={400}
  priority={true}
  sizes="(max-width: 768px) 100vw, (max-width: 1200px) 80vw, 1200px"
  quality={80}
/>
```

**Для галереи заведений:**

```tsx
// src/components/venue-gallery.tsx
import Image from "next/image"

export function VenueGallery({ photos }: { photos: Photo[] }) {
  return (
    <div className="grid grid-cols-3 gap-4">
      {photos.map((photo, idx) => (
        <Image
          key={photo.id}
          src={photo.url}
          alt={`${photo.title || `Фото ${idx + 1}`}`}
          width={400}
          height={300}
          loading={idx === 0 ? "eager" : "lazy"} // Первое фото - eager, остальные - lazy
          sizes="(max-width: 768px) 100vw, (max-width: 1200px) 50vw, 33vw"
          className="object-cover w-full h-full rounded"
        />
      ))}
    </div>
  )
}
```

**Проверить результаты:**

```bash
# Запустить PageSpeed Insights
npm run build
npm run start
# Открыть https://pagespeed.web.dev
# Ввести URL localhost:3000
```

---

### 3. Добавить Twitter Card (5 минут)

**Файл: `src/app/layout.tsx`** (обновить metadata)

```typescript
import type { Metadata } from "next";

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
  // ✨ ДОБАВИТЬ:
  twitter: {
    card: "summary_large_image",
    title: "БаняГид",
    description: "Каталог бань и саун с отзывами и онлайн-бронированием",
    creator: "@banya_guid", // если есть аккаунт в Твиттере
    images: [
      {
        url: "/og-home.jpg",
        width: 1200,
        height: 630,
        alt: "БаняГид — бани и сауны",
      },
    ],
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
```

**Проверить:**

```bash
# Открыть https://cards-dev.twitter.com/validator
# Вставить URL сайта
```

---

### 4. Добавить FAQ Schema (15 минут)

**Файл: `src/lib/faq-jsonld.ts`** (создать новый)

```typescript
export type FAQ = {
  question: string
  answer: string
}

export function faqJsonLd(faqs: FAQ[]): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "FAQPage",
    mainEntity: faqs.map(({ question, answer }) => ({
      "@type": "Question",
      name: question,
      acceptedAnswer: {
        "@type": "Answer",
        text: answer,
      },
    })),
  }
}
```

**Файл: `src/app/(public)/faq/page.tsx`** (создать новый)

```typescript
import type { Metadata } from "next"
import { faqJsonLd } from "@/lib/faq-jsonld"
import { safeJsonLdStringify } from "@/lib/seo-jsonld"
import { siteUrl } from "@/lib/seo-site"

const FAQS = [
  {
    question: "Как зарезервировать баню онлайн?",
    answer:
      "Выберите заведение в каталоге, нажмите «Забронировать», укажите дату и время. Подтверждение придёт на почту.",
  },
  {
    question: "Какая разница между баней и сауной?",
    answer:
      "Баня — влажная парилка (60-70°C, влажность 50-70%). Сауна — сухая (70-100°C, низкая влажность). Выбирайте по предпочтениям.",
  },
  {
    question: "Можно ли вернуть деньги за отмену?",
    answer:
      "Да, если отменить за 24 часа до бронирования. Подробнее в условиях каждого заведения.",
  },
  {
    question: "Сколько стоит аренда бани на компанию?",
    answer:
      "Зависит от заведения и времени. Смотрите цены в карточке каждой бани, фильтруйте по бюджету.",
  },
]

export const metadata: Metadata = {
  title: "Часто задаваемые вопросы о банях и саунах",
  description: "Ответы на вопросы о бронировании, типах саун, ценах и услугах.",
  alternates: { canonical: `${siteUrl()}/faq` },
}

export default function FAQPage() {
  const jsonLd = faqJsonLd(FAQS)

  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(jsonLd) }}
      />
      <div className="container mx-auto py-12 px-4 max-w-2xl">
        <h1 className="text-4xl font-bold mb-8">Часто задаваемые вопросы</h1>
        <div className="space-y-6">
          {FAQS.map((faq, idx) => (
            <details key={idx} className="border-b pb-4">
              <summary className="text-lg font-semibold cursor-pointer hover:text-blue-600">
                {faq.question}
              </summary>
              <p className="mt-4 text-gray-700 leading-relaxed">{faq.answer}</p>
            </details>
          ))}
        </div>
      </div>
    </>
  )
}
```

**Проверить:**

```bash
# https://search.google.com/test/rich-results
# Вставить URL https://yoursite.com/faq
```

---

### 5. Добавить мониторинг Web Vitals (15 минут)

**Файл: `src/components/seo/web-vitals.tsx`** (создать новый)

```typescript
"use client"

import { useEffect } from "react"
import { getCLS, getFID, getFCP, getLCP, getTTFB, Metric } from "web-vitals"

function sendMetrics(metric: Metric) {
  console.log(`${metric.name}: ${metric.value.toFixed(2)}ms (${metric.rating})`)

  // Опционально: отправить на сервер аналитики
  if (typeof window !== "undefined" && "gtag" in window) {
    ;(window as any).gtag?.("event", "page_view", {
      page_path: window.location.pathname,
      [`metric_${metric.name}`]: metric.value,
      metric_rating: metric.rating,
    })
  }
}

export function WebVitalsReporter() {
  useEffect(() => {
    getCLS(sendMetrics)
    getFID(sendMetrics)
    getFCP(sendMetrics)
    getLCP(sendMetrics)
    getTTFB(sendMetrics)
  }, [])

  return null
}
```

**Файл: `src/app/layout.tsx`** (добавить компонент)

```typescript
import { WebVitalsReporter } from "@/components/seo/web-vitals"

export default function RootLayout({ children }: ...) {
  return (
    <html lang="ru">
      <body>
        <WebVitalsReporter />
        {/* ... */}
      </body>
    </html>
  )
}
```

**Проверить в Chrome DevTools:**

```bash
npm run dev
# Открыть DevTools → Console
# Должны видеть вывод вроде:
# CLS: 0.05 (good)
# FID: 45.20 (good)
# LCP: 1500.30 (good)
```

---

### 6. Проверить JSON-LD валидацию (10 минут)

**Скрипт для проверки:**

```bash
# Установить инструмент
npm install -D jsonschema

# Проверить все JSON-LD в проекте:
npx next build

# Открыть продакшн версию:
npm run start

# Перейти на: https://search.google.com/test/rich-results
# Вставить URLs:
# - https://localhost:3000
# - https://localhost:3000/venues/city/moskva
# - https://localhost:3000/venues/{any-slug}
# - https://localhost:3000/masters/{any-slug}
```

**Что проверять:**
- ✅ Нет ошибок (Error)
- ✅ Нет предупреждений (Warning)
- ✅ Все required поля присутствуют

---

## 🔧 Дополнительные оптимизации

### Добавить RSS Feed

**Файл: `src/app/feed.xml/route.ts`** (создать новый)

```typescript
import { getVenues } from "@/lib/api"
import { siteUrl } from "@/lib/seo-site"

export const dynamic = "force-dynamic"

export async function GET() {
  const base = siteUrl()
  
  try {
    const { venues } = await getVenues({ 
      page: 1, 
      page_size: 50, 
      sort_by: "rating" 
    })

    const rss = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
  <channel>
    <title>БаняГид — Каталог бань и саун</title>
    <link>${base}</link>
    <description>Самые рейтинговые бани и сауны в России</description>
    <language>ru</language>
    <atom:link href="${base}/feed.xml" rel="self" type="application/rss+xml" />
    ${venues
      .map(
        (v) => `
    <item>
      <title>${escapeXml(v.name)}</title>
      <link>${base}/venues/${encodeURIComponent(v.slug)}</link>
      <description>${escapeXml(v.description?.slice(0, 200) || "")}</description>
      <category>${v.kind || "Баня"}</category>
      <pubDate>${new Date(v.created_at).toUTCString()}</pubDate>
      <guid>${base}/venues/${encodeURIComponent(v.slug)}</guid>
    </item>
      `
      )
      .join("")}
  </channel>
</rss>`

    return new Response(rss, {
      headers: {
        "Content-Type": "application/xml; charset=utf-8",
        "Cache-Control": "public, s-maxage=3600, stale-while-revalidate=86400",
      },
    })
  } catch {
    return new Response("Error generating feed", { status: 500 })
  }
}

function escapeXml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;")
}
```

**Добавить в robots.txt:**

```
Sitemap: https://yourdomain.com/sitemap.xml
Sitemap: https://yourdomain.com/feed.xml
```

---

### Добавить мета-теги для рассказов (AMP + Web Stories)

**Опционально для соцсетей:**

```typescript
// src/app/layout.tsx
export const metadata: Metadata = {
  // ... существующее
  alternates: {
    canonical: siteUrl(),
    ampHtml: `${siteUrl()}/amp/`, // если AMP есть
  },
}
```

---

## 📊 Тестирование и валидация

### Чеклист перед запуском

```markdown
## Pre-Launch SEO Checklist

- [ ] PageSpeed Insights: LCP < 2.5s
- [ ] PageSpeed Insights: CLS < 0.1
- [ ] PageSpeed Insights: FID < 100ms
- [ ] Mobile-Friendly Test: Passed
- [ ] Rich Results Test: Нет ошибок
- [ ] robots.txt: Доступен (GET /robots.txt → 200)
- [ ] sitemap.xml: Доступен (GET /sitemap.xml → 200)
- [ ] Meta tags: title, description на всех страницах
- [ ] OG tags: Имеются изображения (width=1200, height=630)
- [ ] Canonical URLs: На всех страницах (особенно с параметрами)
- [ ] Структурированные данные:
  - [ ] HealthAndBeautyBusiness на /venues/{slug}
  - [ ] Person на /masters/{slug}
  - [ ] BreadcrumbList на списках
  - [ ] FAQPage (если есть)
- [ ] Навигация: Breadcrumbs в UI
- [ ] 404 страница: Красивая, с ссылкой на главную
- [ ] Internal linking: Ссылки на города, типы саун
- [ ] Images: Все с alt, width, height
- [ ] Accessibility: ARIA-labels, semantic HTML

## Post-Launch Monitoring

- [ ] Google Search Console: сайт добавлен
- [ ] Yandex Webmaster: сайт добавлен
- [ ] Google Analytics 4: отслеживание установлено
- [ ] Мониторинг ошибок индексации: еженедельно
- [ ] Мониторинг органического трафика: еженедельно
- [ ] Мониторинг позиций в поиске: ежемесячно
```

---

## 📚 Полезные ресурсы

| Ресурс | URL | Для чего |
|--------|-----|---------|
| Next.js SEO | https://nextjs.org/learn-react/seo/introduction | Официальный гайд |
| Schema.org | https://schema.org | Спецификация структурированных данных |
| Google Search Central | https://developers.google.com/search | Гайдлайны Google |
| Lighthouse | chrome://extensions → DevTools | Полная аудит сайта |
| GTmetrix | https://gtmetrix.com | Анализ производительности |
| AXE DevTools | https://www.deque.com/axe/devtools/ | Проверка доступности |

---

**Автор:** Implementation Guide  
**Дата:** 2026-05-13  
**Статус:** Ready to implement

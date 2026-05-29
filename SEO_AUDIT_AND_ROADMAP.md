# SEO Аудит и Roadmap — БаняГид

## 📊 Текущее состояние SEO (что уже сделано)

### ✅ Реализовано

#### 1. **Техническое SEO**
- ✅ **robots.txt** — настроены блокировки для служебных путей, параметров фасетирования
  - Правильно заблокированы `/admin`, `/owner`, `/my`, `/auth`, `/api`
  - Заблокированы дублирующие параметры: `?q=`, `?city=`, `?price_*=`, `?sort=`, `?page=`
  - Указан sitemap

- ✅ **Sitemap.xml** (динамический, разбит на шарды)
  - Шард 0: статические + hub-страницы по городам (55 URL)
  - Шард 1+: страницы заведений (до 45к URL за шард)
  - Шард N: страницы мастеров (до 45к URL за шард)
  - Структура справляется с масштабом (если растёт кол-во заведений)

- ✅ **Структурированные данные (Schema.org)**
  - `HealthAndBeautyBusiness` для заведений с:
    - `aggregateRating` (рейтинг + кол-во отзывов)
    - `PostalAddress` (адрес, город)
    - `GeoCoordinates` (широта, долгота)
  - `Person` для мастеров (если реализовано)
  - Безопасная сериализация JSON-LD (экранирование `</script>`)

- ✅ **Meta Tags**
  - `metadataBase` (для абсолютных URLs в OG)
  - Open Graph (`og:title`, `og:description`, `og:image`, `og:url`, `og:locale`)
  - Canonical URLs (важно для предотвращения дублей)
  - Title templates с | разделителем

- ✅ **Интернационализация**
  - `lang="ru"` на корне
  - `locale: "ru_RU"` в OG
  - Cyrillic subset в Geist font для CLS
  - Корректное отображение текста без фалбэков

- ✅ **Аналитика**
  - Yandex Metrika (компонент `YandexMetrika`)

#### 2. **Контент и структура**
- ✅ **SSR на главной** — `getVenues()` запрос для популярных заведений
- ✅ **Уникальные descriptions** на страницах деталей (обрезаны до ~160 символов)
- ✅ **City-hub pages** (`/venues/city/{slug}`) — города + типы парения
- ✅ **OG Images** — извлечение cover-фото из галереи

---

## 🚨 Что хорошо, но требует доработки или мониторинга

### ⚠️ Зоны внимания

| Область | Статус | Описание | Приоритет |
|---------|--------|---------|-----------|
| **Breadcrumb Schema** | ❌ Отсутствует | `BreadcrumbList` для навигации (например, `/venues > Москва > Сауна`) | 🟡 Средний |
| **FAQs Schema** | ❌ Отсутствует | Структурированные вопросы-ответы (если есть FAQ на сайте) | 🟡 Средний |
| **Lazy-load изображений** | ⚠️ Неизвестно | Нужна проверка: используется ли `loading="lazy"`? Влияет на LCP | 🔴 Высокий |
| **Web Vitals** | ⚠️ Неизвестно | CLS, LCP, FID — критичны для ранкирования | 🔴 Высокий |
| **Image optimization** | ⚠️ Вероятно нужна | WebP, srcset, правильные размеры для каждого контекста | 🟡 Средний |
| **Hreflang (мультиязык)** | ❌ Не применяется | Если есть англ. версия, нужны `hreflang` ссылки | 🔴 Если есть i18n |
| **RSS/Feed** | ❌ Отсутствует | Опциональный плюс (для новостей о новых заведениях) | 🟢 Низкий |
| **Structured data tests** | ❌ Не видны результаты | Нужна проверка в `schema.org/validator` | 🔴 Высокий |

---

## 📋 Детальные рекомендации и checklist

### 🔴 СРОЧНО (Приоритет 1)

#### 1.1 Проверить Web Vitals в реальных условиях
```bash
# Установить package для сбора метрик
npm install web-vitals

# Интегрировать в nextjs config
```

**Что делать:**
- Запустить PageSpeed Insights для главной, страницы заведения, города
- Если LCP > 2.5s — оптимизировать изображения (WebP, lazy-load)
- Если CLS > 0.1 — найти и исправить shifts (шрифты, изображения без размеров)

**Файлы для проверки:**
- `frontend/src/components/` — изображения в Hero, Popular Venues
- Все изображения в галереях заведений

---

#### 1.2 Добавить Breadcrumb Schema
**Зачем:** Google покажет breadcrumb в результатах поиска, улучшит UX в SERPs.

**Пример для `/venues/city/moskva/banya`:**
```typescript
// src/lib/breadcrumb-jsonld.ts
export function breadcrumbJsonLd(items: Array<{ label: string; url: string }>) {
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

**Где добавить:**
- `/venues/city/[citySlug]/page.tsx` — хлебная крошка: Главная > {Город}
- `/venues/city/[citySlug]/[kind]/page.tsx` — Главная > {Город} > {Тип парения}
- `/venues/[slug]/page.tsx` — Главная > Заведения > {Название}

---

#### 1.3 Проверить и исправить изображения
**Проблема:** Если фото не оптимизированы, LCP может быть медленным.

**Действия:**
```bash
# Проверить, используется ли Next.js Image component
grep -r "import Image" frontend/src --include="*.tsx"

# Должно быть:
import Image from "next/image"

# А не:
<img src={...} />
```

**Для каждого изображения:**
- Указать `width` и `height` (предотвращает CLS)
- Добавить `loading="lazy"` для внеэкранных изображений
- Использовать `priority` для hero-образов
- Добавить `alt` для SEO и a11y

**Пример:**
```tsx
<Image
  src={venue.image_url}
  alt={`${venue.name} — баня в ${venue.city}`}
  width={1200}
  height={630}
  priority // hero image
  sizes="(max-width: 768px) 100vw, (max-width: 1200px) 50vw, 33vw"
/>
```

---

### 🟡 ВАЖНО (Приоритет 2)

#### 2.1 Добавить FAQ Schema (если есть часто задаваемые вопросы)
**Зачем:** Google покажет выпадающие вопросы в SERPs.

**Где:** На главной или на отдельной странице `/faq`

```typescript
// src/lib/faq-jsonld.ts
export function faqJsonLd(faqs: Array<{ question: string; answer: string }>) {
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

---

#### 2.2 Добавить Twitter Card meta tags
**Текущее состояние:** Open Graph указаны, но Twitter Card нет.

```typescript
// src/app/layout.tsx
export const metadata: Metadata = {
  // ... существующее
  twitter: {
    card: "summary_large_image",
    title: "БаняГид",
    description: "Бани и сауны с отзывами",
    images: ["/og-home.jpg"],
  },
}
```

---

#### 2.3 Оптимизировать города и типы парения для SEO
**Текущее:** SEO_CITY_HUBS покрывает 10 городов.

**Рекомендация:**
- Расширить список городов, если есть заведения (на основе реальных данных)
- Для каждого города написать уникальный intro (сейчас хороший текст, но проверить на дублирование)
- Добавить `SEO_VENUE_KIND_DESCRIPTIONS` для каждого типа (сауна, баня русская, финская и т.д.)

```typescript
// src/lib/seo-city-hubs.ts
export const SEO_VENUE_KINDS: Record<string, { name: string; description: string }> = {
  "banya-russkaya": {
    name: "Русская баня",
    description: "Традиционная русская парилка с берёзовыми вениками...",
  },
  "sauna-finskaya": {
    name: "Финская сауна",
    description: "Сухая жаркая парилка с температурой 70-100°C...",
  },
  // ...
}
```

---

#### 2.4 Добавить пагинацию (Pagination Schema)
**Если есть:** `/venues?page=2`, `/masters?page=3`

```typescript
// src/lib/pagination-jsonld.ts
export function paginationJsonLd(currentUrl: string, nextUrl?: string, prevUrl?: string) {
  const out: Record<string, any> = {
    "@context": "https://schema.org",
  }
  if (nextUrl) out.next = nextUrl
  if (prevUrl) out.previous = prevUrl
  return out
}
```

**Добавить в `<head>` страниц списков:**
```tsx
<link rel="next" href={nextPageUrl} />
<link rel="prev" href={prevPageUrl} />
```

---

#### 2.5 Проверить mobile-friendly
```bash
# Запустить тест на реальном мобильном размере
npm run dev
# Открыть DevTools → Responsive Mode → iPhone 12
# Проверить: читаемость текста, размер кнопок, скорость
```

---

### 🟢 РЕКОМЕНДУЕТСЯ (Приоритет 3)

#### 3.1 Добавить Analytics и Search Console интеграцию
```typescript
// src/app/layout.tsx
// Google Analytics 4 (если нужен)
import { GoogleAnalytics } from "@next/third-parties/google"

export default function RootLayout({ children }: {...}) {
  return (
    <html>
      <GoogleAnalytics gaId="G-XXXXXXXXXX" />
      {/* ... */}
    </html>
  )
}
```

**Действия:**
- Подключить Google Search Console (добавить сайт, скачать verification файл)
- Подключить Google Analytics 4
- Мониторить: ошибки индексации, кликабельность в SERPs, фаворы мобильные vs desktop

---

#### 3.2 Добавить structured data для Review/AggregateRating
**Если у заведений есть отзывы внутри страницы:**

```typescript
export function reviewListJsonLd(reviews: Review[]) {
  return {
    "@context": "https://schema.org",
    "@type": "ItemList",
    itemListElement: reviews.map((r, idx) => ({
      "@type": "Review",
      position: idx + 1,
      reviewRating: {
        "@type": "Rating",
        ratingValue: r.rating,
        bestRating: 5,
        worstRating: 1,
      },
      reviewBody: r.text,
      author: {
        "@type": "Person",
        name: r.author_name,
      },
      datePublished: r.created_at,
    })),
  }
}
```

---

#### 3.3 Добавить RSS Feed (опционально)
```typescript
// src/app/feed.xml/route.ts
export async function GET() {
  const venues = await getVenues({ page: 1, page_size: 50 })
  const rss = `<?xml version="1.0" encoding="UTF-8" ?>
    <rss version="2.0">
      <channel>
        <title>БаняГид — Новые заведения</title>
        <link>${siteUrl()}</link>
        <description>Свежие добавления в каталог</description>
        ${venues.venues.map(v => `
          <item>
            <title>${v.name}</title>
            <link>${siteUrl()}/venues/${encodeURIComponent(v.slug)}</link>
            <description>${v.description?.slice(0, 200)}</description>
            <pubDate>${new Date(v.created_at).toUTCString()}</pubDate>
          </item>
        `).join('')}
      </channel>
    </rss>`
  return new Response(rss, {
    headers: { "Content-Type": "application/xml" },
  })
}
```

---

#### 3.4 Добавить Redirect для старых URL (если были)
```typescript
// next.config.js
module.exports = {
  redirects: async () => [
    {
      source: "/old-venues/:slug",
      destination: "/venues/:slug",
      permanent: true, // 301
    },
  ],
}
```

---

## 🛠 Чеклист имплементации

### Неделя 1 (Критичное)
- [ ] Запустить PageSpeed Insights для топ-3 страниц
- [ ] Исправить изображения (Next.js Image, width/height, lazy-load)
- [ ] Проверить JSON-LD в schema.org validator
- [ ] Добавить Breadcrumb Schema на страницы списков и деталей

### Неделя 2 (Важное)
- [ ] Добавить Twitter Card
- [ ] Расширить SEO_CITY_HUBS (если нужно больше городов)
- [ ] Добавить FAQ Schema (если есть FAQ)
- [ ] Протестировать на мобильных устройствах

### Неделя 3 (Поддержка)
- [ ] Подключить Google Search Console
- [ ] Подключить Google Analytics 4
- [ ] Добавить мониторинг Core Web Vitals
- [ ] Документировать SEO процессы в CLAUDE.local.md

---

## 🔍 Инструменты для проверки

| Инструмент | Ссылка | Зачем |
|------------|--------|--------|
| PageSpeed Insights | https://pagespeed.web.dev | Проверить LCP, CLS, FID |
| Mobile-Friendly Test | https://search.google.com/test/mobile-friendly | Мобильная оптимизация |
| Rich Results Test | https://search.google.com/test/rich-results | Валидация Schema.org |
| Lighthouse | Chrome DevTools → Lighthouse | Полная аудит SEO + Perf |
| Google Search Console | https://search.google.com/search-console | Мониторинг индексации, ошибок |
| Yoast SEO | https://yoast.com/seo-tools/ | Анализ ключевых слов |

---

## 📈 KPIs для отслеживания

После реализации рекомендаций мониторьте:

1. **Organic Traffic** (Google Analytics)
   - Целевой: +50% за 6 месяцев
   
2. **Позиции в поиске** (Search Console)
   - Целевой: топ-3 для "баня рядом [город]"
   
3. **Core Web Vitals**
   - LCP < 2.5s
   - CLS < 0.1
   - FID < 100ms
   
4. **Индексация**
   - Search Console → Coverage → Indexed pages
   - Целевой: 95%+ покрытие важных страниц

---

## 📝 Дополнительные заметки

- **Языковые фильтры:** Если будет англ. версия, добавить `hreflang` вместо `x-default`
- **AMP:** Не требуется для Вашего проекта (современные бы эпоха Next.js достаточно)
- **PWA:** Опциональный плюс (offline + fast loading)
- **Доменная политика:** Убедитесь, что `metadataBase` + robots.txt + canonical совпадают на одном домене

---

**Автор:** SEO Audit  
**Дата:** 2026-05-13  
**Статус:** Рекомендовано к имплементации

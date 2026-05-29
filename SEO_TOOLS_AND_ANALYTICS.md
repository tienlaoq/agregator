# SEO Tools & Analytics — Инструменты и услуги для интеграции

## 📌 Приоритет интеграции

| Статус | Инструмент | Затраты | Сложность | Зачем |
|--------|-----------|---------|-----------|--------|
| 🔴 Обязательно | Google Search Console | Бесплатно | 5 мин | Мониторинг индексации, ошибок, позиций |
| 🔴 Обязательно | Google Analytics 4 | Бесплатно | 10 мин | Отслеживание трафика, поведения |
| 🔴 Обязательно | Яндекс.Вебмастер | Бесплатно | 5 мин | Индексация в Яндексе |
| 🟡 Рекомендуется | Яндекс.Метрика | Бесплатно | Уже установлен | Аналитика (уже в проекте) |
| 🟡 Рекомендуется | ahrefs Lite | $99/мес | 10 мин | Анализ бэклинков, позиций |
| 🟡 Рекомендуется | Semrush | $119/мес | 10 мин | Аналитика конкурентов, ключевые слова |
| 🟢 Опционально | Google Search Central | Бесплатно | 5 мин | Актуальная информация от Google |
| 🟢 Опционально | Screaming Frog SEO Spider | $199 (одноразово) | 30 мин | Аудит сайта (скорость, дубли, ошибки) |

---

## 🚀 Быстрая интеграция (День 1)

### 1. Google Search Console (GSC)

**Зачем:** 
- Узнать, как Google видит сайт
- Отслеживать ошибки индексации (4xx, 5xx)
- Мониторить позиции, клики, CTR
- Получать алерты о проблемах

**Шаги:**

```bash
# 1. Перейти на https://search.google.com/search-console
# 2. Нажать "Add Property"
# 3. Выбрать "URL prefix" (https://yourdomain.com)
# 4. Выбрать способ верификации:

# Способ A: HTML file (рекомендуется)
# - Скачать verification файл
# - Положить в public/google...html
# - Добавить в robots.ts или next.config.js redirect

# Способ B: HTML tag (быстрее)
# - Скопировать meta tag
# - Добавить в src/app/layout.tsx:

export const metadata: Metadata = {
  // ...
  verification: {
    google: "YOUR_GOOGLE_VERIFICATION_CODE_HERE",
  },
}

# Способ C: DNS record
# - Добавить TXT запись у провайдера домена (Namecheap, Register.ru и т.д.)
```

**После верификации:**

```
Google Search Console → Settings → Sitelinks
Google Search Console → Settings → Search Appearance
Google Search Console → Links → Top referring sites
Google Search Console → Coverage → Indexed pages
```

**Дашборд мониторить еженедельно:**
- Coverage (ошибки индексации)
- Performance (позиции, клики, CTR)
- Mobile Usability (мобильные ошибки)

---

### 2. Google Analytics 4 (GA4)

**Зачем:**
- Отслеживать приход посетителей
- Видеть поведение пользователей (страницы, время на сайте)
- Конверсии (бронирования, оставленные контакты)
- Мобильный vs. desktop трафик

**Шаги:**

```bash
# 1. Перейти на https://analytics.google.com
# 2. Create → New Property
# 3. Выбрать регион (Russia)
# 4. Скопировать Measurement ID (G-XXXXXXXXXX)
```

**Добавить в проект:**

```bash
npm install @next/third-parties
```

**Файл: `src/app/layout.tsx`**

```typescript
import { GoogleAnalytics } from "@next/third-parties/google"

export default function RootLayout({ children }: {...}) {
  return (
    <html lang="ru">
      <body>
        <GoogleAnalytics gaId="G-YOUR_MEASUREMENT_ID" />
        {children}
      </body>
    </html>
  )
}
```

**Дополнительная настройка:**

```typescript
// src/app/layout.tsx
import Script from "next/script"

export default function RootLayout({ children }: {...}) {
  return (
    <html>
      <head>
        {/* GA4 global site tag */}
        <Script
          strategy="afterInteractive"
          src="https://www.googletagmanager.com/gtag/js?id=G-YOUR_ID"
        />
        <Script
          id="google-analytics"
          strategy="afterInteractive"
          dangerouslySetInnerHTML={{
            __html: `
              window.dataLayer = window.dataLayer || [];
              function gtag(){dataLayer.push(arguments);}
              gtag('js', new Date());
              gtag('config', 'G-YOUR_ID');
            `,
          }}
        />
      </head>
      <body>
        {children}
      </body>
    </html>
  )
}
```

**Отслеживать события (например, бронирование):**

```typescript
// src/components/booking-button.tsx
"use client"

export function BookingButton({ venueId }: { venueId: string }) {
  const handleClick = () => {
    // Отправить событие в GA4
    if (typeof window !== "undefined" && "gtag" in window) {
      ;(window as any).gtag?.("event", "begin_checkout", {
        currency: "RUB",
        value: 5000,
        items: [
          {
            item_id: venueId,
            item_name: "Бронирование бани",
          },
        ],
      })
    }
    // ... остальная логика бронирования
  }

  return <button onClick={handleClick}>Забронировать</button>
}
```

**Мониторить в GA4:**
- Real-time users (сколько сейчас на сайте)
- Top pages (какие страницы популярны)
- Traffic sources (откуда приходят посетители)
- Conversions (если настроите события)

---

### 3. Яндекс.Вебмастер (Yandex Webmaster)

**Зачем:**
- Индексация в Яндексе (половина русскоязычного трафика)
- Статистика кликов в Яндекс.Поиске
- Мониторинг ошибок

**Шаги:**

```bash
# 1. Перейти на https://webmaster.yandex.ru
# 2. Войти через Яндекс.ID (создать если нет)
# 3. Add site → Ввести https://yourdomain.com
# 4. Верификация (выбрать один из методов):

# Способ A: Meta tag (рекомендуется)
export const metadata: Metadata = {
  // ...
  // <meta name="yandex-verification" content="XXXXXXXX" />
}

# Способ B: HTML file
# - Скачать verification file
# - Положить в public/yandex_...html
```

**После верификации:**

```
Яндекс.Вебмастер → Indexing → Indexed pages
Яндекс.Вебмастер → Indexing → Errors
Яндекс.Вебмастер → Searchability → User agent (проверить как Яндекс видит сайт)
```

---

## 🎯 Расширенная аналитика (Неделя 2)

### 4. ahrefs Lite ($99/мес)

**Зачем:**
- Анализ бэклинков (кто ссылается на сайт)
- Позиции по ключевым словам
- Анализ конкурентов

**Бесплатная альтернатива:** SEMrush Site Audit (free)

**Что проверять:**
- Backlinks: есть ли ссылки на сайт (важно для ранкирования)
- Referring domains: с каких сайтов приходят ссылки
- Keyword rankings: позиции в поиске по запросам

---

### 5. Semrush ($119/мес)

**Зачем:**
- Анализ ключевых слов
- SEO audit (полный аудит сайта)
- Анализ конкурентов
- Tracking positions (отслеживание позиций)

**Бесплатная альтернатива:**
- Google Keyword Planner (от Google)
- Ubersuggest (ограниченный доступ)

---

## 🛠 Инструменты для локального тестирования (Бесплатно)

### Lighthouse (встроен в Chrome)

```bash
# 1. Запустить сайт
npm run dev

# 2. Открыть DevTools (F12)
# 3. Lighthouse → Generate report

# Проверяет:
# - Performance (LCP, CLS, FID)
# - Accessibility (a11y)
# - Best Practices
# - SEO
# - PWA
```

### Screaming Frog SEO Spider ($199 одноразово)

```bash
# 1. Скачать с https://www.screamingfrog.co.uk/seo-spider/
# 2. Запустить: File → List → Enter URL https://localhost:3000
# 3. Start → Crawl site

# Проверяет:
# - Broken links (404)
# - Duplicate titles/descriptions
# - Missing meta tags
# - Redirect chains
# - Page size, load time
# - Images without alt
```

**Бесплатная альтернатива:** sitebulb.com (trial 14 дней)

---

## 📊 Мониторинг позиций (Ranking Tracker)

### Опция 1: Google Search Console (бесплатно, но ограничено)

```
Google Search Console → Performance
- Показывает топ-100 запросов
- Средние позиции
- Клики и CTR
```

### Опция 2: SE Ranking ($65/мес)

- Мониторит 1000+ ключевых слов
- Еженедельные отчёты о позициях
- Уведомления об изменениях

### Опция 3: Semrush Position Tracking (в Semrush)

- Отслеживание позиций в Google/Яндекс/Bing
- История изменений
- Сравнение с конкурентами

---

## 🔔 Alerts & Notifications Setup

### Настроить оповещения в Google Search Console

```
Settings → Notifications
- [ ] Indexing Issues (ошибки индексации)
- [ ] Index Coverage Problems
- [ ] Mobile Usability Issues
- [ ] Search Appearance Issues
```

### Настроить оповещения в Яндекс.Вебмастер

```
Settings → Notifications
- [ ] Ошибки сканирования (crawl errors)
- [ ] Проблемы индексации
- [ ] Virus alerts
```

---

## 📈 Еженедельный SEO Чеклист (автоматизируемо)

Можно создать **Scheduled Task** в Cowork:

```markdown
## Weekly SEO Report

### Google Search Console
- [ ] Coverage: нет ли новых ошибок
- [ ] Performance: клики, позиции, CTR
- [ ] URL Inspection: проверить 1-2 случайные страницы

### Google Analytics
- [ ] Sessions & Users: сравнить с неделей ранее
- [ ] Top Pages: какие страницы приносят трафик
- [ ] Traffic Source: органический трафик растёт?

### Яндекс.Вебмастер
- [ ] Indexed pages: мониторить динамику
- [ ] Errors: есть ли ошибки индексации

### Ручная проверка
- [ ] Запустить 1 PageSpeed Insights
- [ ] Проверить, нет ли 404 ошибок
- [ ] Убедиться, что RSS feed обновляется
```

**Создать автоматический еженедельный отчёт:**

```bash
# Использовать /schedule skill в Cowork:
# "Создай мне еженедельный SEO отчёт каждый понедельник в 9:00"
```

---

## 💰 Бюджет SEO Tools (годовой)

| Инструмент | Стоимость | Альтернатива |
|-----------|-----------|-------------|
| Google Search Console | **Бесплатно** | - |
| Google Analytics 4 | **Бесплатно** | Yandex.Metrika ✅ (уже используется) |
| Яндекс.Вебмастер | **Бесплатно** | - |
| ahrefs Lite | $99/мес = **$1,188/год** | Semrush Lite $119/мес |
| SE Ranking | $65/мес = **$780/год** | Semrush Lite |
| Screaming Frog | **$199** (одноразово) | Sitebulb.com $99/год |
| **ИТОГО (минимум)** | **$0** (бесплатные сервисы) | - |
| **ИТОГО (оптимально)** | **~$1,200/год** | ahrefs + SE Ranking |

**Рекомендуемый стек:**
- Google Search Console + Analytics (бесплатно)
- Яндекс.Вебмастер (бесплатно)
- ahrefs Lite ($99/мес) — для анализа бэклинков
- **Итого: ~$1,200/год**

---

## 🚀 Полный план интеграции (2 недели)

### День 1-2 (Понедельник-Вторник)
- [ ] Настроить Google Search Console (30 мин)
- [ ] Настроить Google Analytics 4 (30 мин)
- [ ] Настроить Яндекс.Вебмастер (20 мин)
- [ ] Добавить verification meta tags (10 мин)

### День 3-4 (Среда-Четверг)
- [ ] Запустить Lighthouse audit
- [ ] Исправить критичные ошибки (LCP, CLS)
- [ ] Проверить JSON-LD в rich results test

### День 5 (Пятница)
- [ ] Запустить Screaming Frog (если есть бюджет)
- [ ] Создать SEO Monitoring Dashboard

### Неделя 2
- [ ] Подписаться на ahrefs/SE Ranking (если решено)
- [ ] Настроить еженедельный мониторинг
- [ ] Создать отчёт о базовых метриках

---

## 📚 Документация для команды

Создайте в проекте файл `docs/SEO_MONITORING.md`:

```markdown
# SEO Monitoring Guide для разработчиков

## Когда вносить изменения в SEO

- [ ] После добавления нового функционала → обновить robots.txt/sitemap
- [ ] После изменения URL → добавить redirect 301
- [ ] После изменения title/description → проверить в GSC Performance
- [ ] После добавления нового контента → проверить индексацию через 1 неделю

## Как проверить SEO перед деплоем

```bash
npm run build
npm run start

# Открыть https://pagespeed.web.dev
# Открыть https://search.google.com/test/rich-results
# Проверить консоль на ошибки (не должно быть console.error)
```

## Ссылки для доступа

- Google Search Console: https://search.google.com/search-console
- Google Analytics: https://analytics.google.com
- Яндекс.Вебмастер: https://webmaster.yandex.ru
- ahrefs: https://ahrefs.com
```

---

## 🎯 Успешная интеграция = результат в 3 месяца

Если правильно настроить SEO, можно ожидать:

| Метрика | До | Через 3 мес | Через 6 мес |
|---------|-------|-------------|------------|
| Organic sessions/месяц | 100 | 500-1000 | 2000+ |
| Top-3 в Google | 0 | 5-10 | 20+ |
| Average position | - | 15-20 | 8-12 |
| Click-through rate | ~2% | 3-4% | 5%+ |

**Условия:**
- Качественный контент (уникальные описания)
- Хорошая скорость (LCP < 2.5s)
- Структурированные данные (Schema.org)
- Регулярное обновление контента
- Лояльная аудитория (repeat visitors)

---

**Автор:** Tools & Analytics Guide  
**Дата:** 2026-05-13  
**Версия:** 1.0  
**Статус:** Ready to implement

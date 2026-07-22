This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

## Getting Started

First, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

## SEO (продакшен)

- **`NEXT_PUBLIC_SITE_URL`** — публичный origin без хвоста `/` (например `https://example.com`). Нужен для canonical, Open Graph и `sitemap.xml`.
- Опционально **`NEXT_PUBLIC_YANDEX_METRIKA_ID`** — числовой id [Яндекс.Метрики](https://metrika.yandex.ru/).
- Опционально **`NEXT_PUBLIC_YANDEX_VERIFICATION`** — значение из meta `yandex-verification` в [Яндекс.Вебмастере](https://webmaster.yandex.ru/) (способ подтверждения «Метатег»). Рендерится в `<head>` для подтверждения прав на домен.
- После деплоя: [Яндекс.Вебмастер](https://webmaster.yandex.ru/) и [Google Search Console](https://search.google.com/search-console) — верификация домена, отправка `sitemap.xml`, при необходимости регион сайта.
- Позиции в поиске **не гарантируются**; зависят от конкуренции, возраста домена, контента и внешних ссылок.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Deploy on Vercel

The easiest way to deploy your Next.js app is to use the [Vercel Platform](https://vercel.com/new?utm_medium=default-template&filter=next.js&utm_source=create-next-app&utm_campaign=create-next-app-readme) from the creators of Next.js.

Check out our [Next.js deployment documentation](https://nextjs.org/docs/app/building-your-application/deploying) for more details.

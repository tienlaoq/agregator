import type { Metadata } from "next"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { CTASection } from "@/components/banya/cta-section"
import { siteUrl } from "@/lib/seo-site"
import { safeJsonLdStringify } from "@/lib/seo-jsonld"
import {
  Flame,
  Search,
  CalendarCheck,
  ShieldCheck,
  Star,
  Wallet,
  HeartHandshake,
  BadgeCheck,
  Users,
  MapPin,
  ArrowRight,
  Sparkles,
} from "lucide-react"

const pageUrl = `${siteUrl()}/about`

export const metadata: Metadata = {
  title: "О нас — миссия и принципы БаняГид",
  description:
    "БаняГид — платформа для поиска и онлайн-бронирования бань и саун. Запускаемся: собираем проверенные заведения, выстраиваем систему честных отзывов и прозрачных цен.",
  alternates: { canonical: pageUrl },
  robots: { index: true, follow: true },
  openGraph: {
    title: "О нас — миссия и принципы БаняГид",
    description:
      "Платформа для поиска и бронирования бань и саун. Готовимся к запуску — рассказываем, как будем работать.",
    url: pageUrl,
    images: [{ url: `${siteUrl()}/og-home.jpg`, width: 1200, height: 630, alt: "БаняГид" }],
  },
}

const steps = [
  {
    icon: Search,
    title: "Найдите",
    description: "Фильтры по городу, цене, рейтингу и услугам — нужная баня в пару кликов.",
  },
  {
    icon: Star,
    title: "Сравните",
    description: "Фото, цены и честные отзывы посетителей с подтверждённым визитом.",
  },
  {
    icon: CalendarCheck,
    title: "Забронируйте",
    description: "Свободное время видно онлайн. Бронь без звонков, в любое время суток.",
  },
  {
    icon: Flame,
    title: "Попаритесь",
    description: "Приходите к назначенному часу. Оплата онлайн или на месте — как удобно.",
  },
]

const values = [
  {
    icon: BadgeCheck,
    title: "Только проверенные заведения",
    description:
      "Каждую площадку модерируем перед публикацией: реальный адрес, контакты, фото. Никаких объявлений из случайных групп.",
  },
  {
    icon: ShieldCheck,
    title: "Честные отзывы",
    description:
      "Оставить оценку сможет только гость с подтверждённым визитом — чтобы рейтингу можно было доверять.",
  },
  {
    icon: Wallet,
    title: "Прозрачные цены",
    description:
      "Стоимость видна заранее. Без скрытых комиссий и сюрпризов при оплате.",
  },
  {
    icon: HeartHandshake,
    title: "Сертифицированные мастера",
    description:
      "Подключаем пар-мастеров с подтверждённой квалификацией — для тех, кто ценит настоящую банную культуру.",
  },
  {
    icon: CalendarCheck,
    title: "Бронирование 24/7",
    description:
      "Свободные слоты в реальном времени. Меньше звонков — больше отдыха.",
  },
  {
    icon: Users,
    title: "Площадка для двоих",
    description:
      "Удобно гостям и выгодно владельцам: новые клиенты и аналитика в одном кабинете.",
  },
]

const organizationJsonLd = {
  "@context": "https://schema.org",
  "@type": "Organization",
  name: "БаняГид",
  url: siteUrl(),
  logo: `${siteUrl()}/icon.png`,
  description:
    "Платформа для поиска и онлайн-бронирования бань и саун в России. Готовится к запуску.",
  areaServed: { "@type": "Country", name: "Россия" },
}

export default function AboutPage() {
  return (
    <>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: safeJsonLdStringify(organizationJsonLd) }}
      />

      {/* Hero */}
      <section className="relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-b from-[#78350F] via-[#92400E] to-[#1C1917]" />
        <div className="relative mx-auto flex max-w-4xl flex-col items-center px-4 py-20 text-center md:py-28">
          <span className="mb-6 inline-flex items-center gap-2 rounded-full border border-white/20 bg-white/10 px-4 py-1.5 text-sm font-medium text-white backdrop-blur">
            <Sparkles className="h-4 w-4" />
            Скоро запуск
          </span>
          <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary-foreground/15">
            <Flame className="h-8 w-8 text-primary-foreground" />
          </div>
          <h1 className="mb-4 max-w-3xl text-balance text-4xl font-bold tracking-tight text-white md:text-5xl">
            Делаем поход в баню простым и предсказуемым
          </h1>
          <p className="max-w-2xl text-pretty text-lg text-white/90 md:text-xl">
            БаняГид — это платформа, где можно будет найти, сравнить и забронировать проверенную
            баню или сауну рядом с вами. Без звонков, без неизвестности, без переплат.
          </p>
        </div>
      </section>

      {/* Mission / story */}
      <section className="bg-background py-16 md:py-24">
        <div className="container mx-auto max-w-3xl px-4">
          <span className="mb-3 block text-sm font-semibold uppercase tracking-wide text-primary">
            Наша миссия
          </span>
          <h2 className="mb-6 text-balance text-3xl font-bold text-foreground md:text-4xl">
            Выбрать баню должно быть так же просто, как заказать столик
          </h2>
          <div className="space-y-4 text-lg leading-relaxed text-muted-foreground">
            <p>
              Чтобы попариться, людям приходится обзванивать заведения, искать актуальные цены в
              чатах и надеяться, что свободное время ещё осталось. Отзывов толком нет, фото
              устарели, а гарантий — никаких.
            </p>
            <p>
              Мы собираем проверенные бани и сауны в одном месте: с реальными отзывами, актуальными
              ценами и онлайн-бронированием. Сейчас платформа в стадии запуска — постепенно
              подключаем первых партнёров и доводим сервис до того уровня, на котором не стыдно
              рекомендовать друзьям.
            </p>
          </div>
        </div>
      </section>

      {/* How it works */}
      <section className="bg-background py-16 md:py-24">
        <div className="container mx-auto px-4">
          <div className="mb-12 text-center">
            <h2 className="mb-3 text-balance text-3xl font-bold text-foreground md:text-4xl">
              Как это работает
            </h2>
            <p className="mx-auto max-w-xl text-pretty text-lg text-muted-foreground">
              Четыре шага от идеи попариться до тёплой парной
            </p>
          </div>
          <div className="grid gap-6 md:grid-cols-4">
            {steps.map((step, idx) => (
              <Card key={step.title} className="relative border-border bg-card">
                <CardContent className="flex flex-col items-center p-8 text-center">
                  <span className="absolute right-4 top-4 text-3xl font-bold text-primary/15">
                    {idx + 1}
                  </span>
                  <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-primary/10">
                    <step.icon className="h-7 w-7 text-primary" />
                  </div>
                  <h3 className="mb-2 text-xl font-semibold text-card-foreground">{step.title}</h3>
                  <p className="text-sm text-muted-foreground">{step.description}</p>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </section>

      {/* Values */}
      <section className="bg-secondary/40 py-16 md:py-24">
        <div className="container mx-auto px-4">
          <div className="mb-12 text-center">
            <h2 className="mb-3 text-balance text-3xl font-bold text-foreground md:text-4xl">
              Принципы платформы
            </h2>
            <p className="mx-auto max-w-xl text-pretty text-lg text-muted-foreground">
              На чём строим сервис с первого дня
            </p>
          </div>
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {values.map((v) => (
              <Card key={v.title} className="border-border bg-card transition-shadow hover:shadow-lg">
                <CardContent className="flex gap-4 p-6">
                  <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-primary/10">
                    <v.icon className="h-6 w-6 text-primary" />
                  </div>
                  <div>
                    <h3 className="mb-1 font-semibold text-card-foreground">{v.title}</h3>
                    <p className="text-sm text-muted-foreground">{v.description}</p>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </section>

      {/* Guest CTA */}
      <section className="bg-background py-16 md:py-24">
        <div className="container mx-auto px-4">
          <div className="mx-auto flex max-w-2xl flex-col items-center text-center">
            <div className="mb-6 flex h-16 w-16 items-center justify-center rounded-full bg-primary/10">
              <MapPin className="h-8 w-8 text-primary" />
            </div>
            <h2 className="mb-4 text-balance text-3xl font-bold text-foreground md:text-4xl">
              Готовы попариться?
            </h2>
            <p className="mb-8 text-pretty text-lg text-muted-foreground">
              Найдите проверенную баню или сауну рядом с вами и забронируйте удобное время онлайн.
            </p>
            <Button size="lg" asChild className="gap-2 px-10">
              <Link href="/venues#catalog">
                Открыть каталог
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
          </div>
        </div>
      </section>

      {/* Owner / partner CTA */}
      <CTASection />
    </>
  )
}

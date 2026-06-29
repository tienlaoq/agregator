import Link from "next/link"
import { Clock, Flame, LifeBuoy, Mail, MessageCircle, Phone, Send, ShieldCheck } from "lucide-react"
import { SEO_CITY_HUBS } from "@/lib/seo-city-hubs"

// TODO: заменить плейсхолдеры реальными данными перед продом.
// Это публичные контакты/реквизиты (не PII по политике вебхуков) — берутся из договора с эквайером.
const SITE_PHONE = "8 800 000-00-00"
const SITE_PHONE_HREF = "tel:+78000000000"
const SITE_EMAIL = "hello@banyagid.ru"
const SUPPORT_HOURS = "Пн–Вс, 9:00–21:00"

const LEGAL_ENTITY = "ИП Иванов Иван Иванович"
const LEGAL_INN = "770000000000"
const LEGAL_OGRN = "000000000000000" // ОГРНИП для ИП (15 цифр); для ООО — ОГРН (13 цифр), поправить лейбл ниже

const footerCities = SEO_CITY_HUBS.slice(0, 6)

// Курируемые посадочные «город × тип» — основной SEO-таргет («баня в [город]»).
// Падежи в названиях заданы вручную: автогенерация из name (им. падеж) дала бы «Бани в Москва».
const popularLinks = [
  { href: "/venues/city/moskva/banya", label: "Бани в Москве" },
  { href: "/venues/city/moskva/sauna", label: "Сауны в Москве" },
  { href: "/venues/city/sankt-peterburg/banya", label: "Бани в Петербурге" },
  { href: "/venues/city/ekaterinburg/sauna", label: "Сауны в Екатеринбурге" },
  { href: "/venues/city/kazan/hammam", label: "Хаммамы в Казани" },
]

const socialLinks = [
  { href: "https://t.me/banyagid", label: "Telegram", icon: Send },
  { href: "https://vk.com/banyagid", label: "ВКонтакте", icon: VkIcon },
  { href: "https://wa.me/78000000000", label: "WhatsApp", icon: MessageCircle },
]

const paymentMethods = ["Мир", "Visa", "Mastercard", "СБП"]

const legalLinks = [
  { href: "/offer", label: "Публичная оферта" },
  { href: "/privacy", label: "Политика конфиденциальности" },
  { href: "/consent", label: "Согласие на обработку ПД" },
  { href: "/terms", label: "Пользовательское соглашение" },
]

const linkClass = "text-muted-foreground transition-colors hover:text-foreground"

export function Footer() {
  const year = new Date().getFullYear()

  return (
    <footer className="border-t border-border bg-card py-12">
      <div className="container mx-auto px-4">
        <div className="grid gap-8 md:grid-cols-2 lg:grid-cols-12">
          <div className="md:col-span-2 lg:col-span-4">
            <Link href="/" className="mb-4 flex items-center gap-2">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary">
                <Flame className="h-5 w-5 text-primary-foreground" />
              </div>
              <span className="text-xl font-bold text-card-foreground">БаняГид</span>
            </Link>
            <p className="max-w-xs text-sm text-muted-foreground">
              Каталог бань и саун с отзывами и онлайн-бронированием. Подборки по городам и типу парения.
            </p>
            <div className="mt-5 flex gap-2">
              {socialLinks.map((s) => (
                <a
                  key={s.label}
                  href={s.href}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label={s.label}
                  className="flex h-9 w-9 items-center justify-center rounded-md border border-border text-muted-foreground transition-colors hover:border-foreground/30 hover:text-foreground"
                >
                  <s.icon className="h-[18px] w-[18px]" />
                </a>
              ))}
            </div>
          </div>

          <nav aria-label="Навигация" className="lg:col-span-2">
            <h4 className="mb-4 font-semibold text-card-foreground">Навигация</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/venues" className={linkClass}>
                  Каталог заведений
                </Link>
              </li>
              <li>
                <Link href="/masters" className={linkClass}>
                  Мастера
                </Link>
              </li>
              <li>
                <Link href="/about" className={linkClass}>
                  О нас
                </Link>
              </li>
              <li>
                <Link href="/partner" className={linkClass}>
                  Для владельцев
                </Link>
              </li>
            </ul>
          </nav>

          <nav aria-label="Города" className="lg:col-span-2">
            <h4 className="mb-4 font-semibold text-card-foreground">Города</h4>
            <ul className="space-y-2 text-sm">
              {footerCities.map((c) => (
                <li key={c.slug}>
                  <Link href={`/venues/city/${c.slug}`} className={linkClass}>
                    {c.name}
                  </Link>
                </li>
              ))}
              <li>
                <Link href="/venues" className={linkClass}>
                  Все города в каталоге
                </Link>
              </li>
            </ul>
          </nav>

          <nav aria-label="Популярные подборки" className="lg:col-span-2">
            <h4 className="mb-4 font-semibold text-card-foreground">Подборки</h4>
            <ul className="space-y-2 text-sm">
              {popularLinks.map((p) => (
                <li key={p.href}>
                  <Link href={p.href} className={linkClass}>
                    {p.label}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>

          <div className="lg:col-span-2">
            <h4 className="mb-4 font-semibold text-card-foreground">Контакты</h4>
            <ul className="space-y-2.5 text-sm">
              <li>
                <a href={SITE_PHONE_HREF} className={`flex items-center gap-2 ${linkClass}`}>
                  <Phone className="h-4 w-4 shrink-0" />
                  {SITE_PHONE}
                </a>
              </li>
              <li>
                <a href={`mailto:${SITE_EMAIL}`} className={`flex items-center gap-2 ${linkClass}`}>
                  <Mail className="h-4 w-4 shrink-0" />
                  {SITE_EMAIL}
                </a>
              </li>
              <li className="flex items-center gap-2 text-muted-foreground">
                <Clock className="h-4 w-4 shrink-0" />
                {SUPPORT_HOURS}
              </li>
              <li>
                <Link href="/support" className={`flex items-center gap-2 ${linkClass}`}>
                  <LifeBuoy className="h-4 w-4 shrink-0" />
                  Центр помощи
                </Link>
              </li>
            </ul>
          </div>
        </div>

        <div className="mt-10 flex flex-wrap items-center gap-2">
          <span className="mr-1 inline-flex items-center gap-1.5 text-sm text-muted-foreground">
            <ShieldCheck className="h-4 w-4" />
            Безопасная оплата
          </span>
          {paymentMethods.map((m) => (
            <span
              key={m}
              className="rounded-md border border-border px-2.5 py-1 text-xs font-medium text-muted-foreground"
            >
              {m}
            </span>
          ))}
        </div>

        <div className="mt-8 flex flex-col gap-4 border-t border-border pt-8 text-sm text-muted-foreground md:flex-row md:items-start md:justify-between">
          <div className="space-y-1">
            <p>
              © {year} БаняГид · {LEGAL_ENTITY}
            </p>
            <p>
              ИНН {LEGAL_INN} · ОГРНИП {LEGAL_OGRN}
            </p>
            <p className="max-w-md text-xs text-muted-foreground/80">
              Сервис является информационной площадкой и не оказывает банных услуг напрямую.
            </p>
          </div>
          <ul className="flex flex-wrap gap-x-4 gap-y-2 md:justify-end">
            {legalLinks.map((l) => (
              <li key={l.href}>
                <Link href={l.href} className={linkClass}>
                  {l.label}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </footer>
  )
}

function VkIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" className={className} fill="currentColor" aria-hidden="true">
      <path d="M15.07 2H8.93C3.33 2 2 3.33 2 8.93v6.14C2 20.67 3.33 22 8.93 22h6.14c5.6 0 6.93-1.33 6.93-6.93V8.93C22 3.33 20.68 2 15.07 2zm3.15 14.27h-1.46c-.55 0-.72-.44-1.71-1.44-.86-.83-1.24-.94-1.45-.94-.3 0-.39.08-.39.5v1.32c0 .36-.12.58-1.07.58-1.57 0-3.32-.95-4.55-2.73-1.85-2.6-2.36-4.55-2.36-4.95 0-.22.08-.42.5-.42h1.46c.38 0 .52.17.67.58.72 2.07 1.92 3.88 2.42 3.88.19 0 .27-.08.27-.56v-2.04c-.06-.97-.58-1.05-.58-1.4 0-.16.14-.33.36-.33h2.3c.32 0 .43.17.43.55v3.09c0 .32.14.43.23.43.19 0 .34-.11.69-.46 1.07-1.2 1.84-3.05 1.84-3.05.1-.22.27-.42.65-.42h1.46c.44 0 .53.22.44.53-.18.85-1.97 3.37-1.97 3.37-.15.25-.21.36 0 .64.15.21.65.64.98 1.03.61.69 1.07 1.27 1.2 1.67.12.4-.08.6-.49.6z" />
    </svg>
  )
}

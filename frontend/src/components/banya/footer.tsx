import Link from "next/link"
import { Flame } from "lucide-react"
import { SEO_CITY_HUBS } from "@/lib/seo-city-hubs"

const footerCities = SEO_CITY_HUBS.slice(0, 8)

export function Footer() {
  return (
    <footer className="border-t border-border bg-card py-12">
      <div className="container mx-auto px-4">
        <div className="grid gap-8 md:grid-cols-4">
          <div className="md:col-span-1">
            <Link href="/" className="mb-4 flex items-center gap-2">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary">
                <Flame className="h-5 w-5 text-primary-foreground" />
              </div>
              <span className="text-xl font-bold text-card-foreground">БаняГид</span>
            </Link>
            <p className="text-sm text-muted-foreground">
              Каталог бань и саун с отзывами и онлайн-бронированием. Подборки по городам и типу парения.
            </p>
          </div>

          <div>
            <h4 className="mb-4 font-semibold text-card-foreground">Навигация</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/venues" className="text-muted-foreground transition-colors hover:text-foreground">
                  Каталог заведений
                </Link>
              </li>
              <li>
                <Link href="/masters" className="text-muted-foreground transition-colors hover:text-foreground">
                  Мастера
                </Link>
              </li>
              <li>
                <Link href="/about" className="text-muted-foreground transition-colors hover:text-foreground">
                  О нас
                </Link>
              </li>
              <li>
                <Link href="/partner" className="text-muted-foreground transition-colors hover:text-foreground">
                  Для владельцев
                </Link>
              </li>
              <li>
                <Link href="/support" className="text-muted-foreground transition-colors hover:text-foreground">
                  Поддержка
                </Link>
              </li>
            </ul>
          </div>

          <div>
            <h4 className="mb-4 font-semibold text-card-foreground">Города</h4>
            <ul className="space-y-2 text-sm">
              {footerCities.map((c) => (
                <li key={c.slug}>
                  <Link
                    href={`/venues/city/${c.slug}`}
                    className="text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {c.name}
                  </Link>
                </li>
              ))}
              <li>
                <Link href="/venues" className="text-muted-foreground transition-colors hover:text-foreground">
                  Все города в каталоге
                </Link>
              </li>
            </ul>
          </div>

          <div>
            <h4 className="mb-4 font-semibold text-card-foreground">Документы</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/support" className="text-muted-foreground transition-colors hover:text-foreground">
                  Помощь и контакты
                </Link>
              </li>
              <li>
                <Link href="/privacy" className="text-muted-foreground transition-colors hover:text-foreground">
                  Политика конфиденциальности
                </Link>
              </li>
              <li>
                <Link href="/terms" className="text-muted-foreground transition-colors hover:text-foreground">
                  Пользовательское соглашение
                </Link>
              </li>
            </ul>
          </div>
        </div>

        <div className="mt-12 border-t border-border pt-8 text-center text-sm text-muted-foreground">
          <p>© {new Date().getFullYear()} БаняГид. Все права защищены.</p>
        </div>
      </div>
    </footer>
  )
}

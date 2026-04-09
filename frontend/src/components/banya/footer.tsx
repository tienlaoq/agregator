import Link from "next/link"
import { Flame } from "lucide-react"

export function Footer() {
  return (
    <footer className="border-t border-border bg-card py-12">
      <div className="container mx-auto px-4">
        <div className="grid gap-8 md:grid-cols-4">
          {/* Logo & Description */}
          <div className="md:col-span-1">
            <Link href="/" className="mb-4 flex items-center gap-2">
              <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary">
                <Flame className="h-5 w-5 text-primary-foreground" />
              </div>
              <span className="text-xl font-bold text-card-foreground">БаняГид</span>
            </Link>
            <p className="text-sm text-muted-foreground">
              Агрегатор бань и саун России. Найди идеальное место для отдыха и релаксации.
            </p>
          </div>

          {/* Navigation */}
          <div>
            <h4 className="mb-4 font-semibold text-card-foreground">Навигация</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="/venues" className="text-muted-foreground transition-colors hover:text-foreground">
                  Каталог
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
            </ul>
          </div>

          {/* Cities */}
          <div>
            <h4 className="mb-4 font-semibold text-card-foreground">Города</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Москва
                </Link>
              </li>
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Санкт-Петербург
                </Link>
              </li>
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Казань
                </Link>
              </li>
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Новосибирск
                </Link>
              </li>
            </ul>
          </div>

          {/* Support */}
          <div>
            <h4 className="mb-4 font-semibold text-card-foreground">Поддержка</h4>
            <ul className="space-y-2 text-sm">
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Помощь
                </Link>
              </li>
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Контакты
                </Link>
              </li>
              <li>
                <Link href="#" className="text-muted-foreground transition-colors hover:text-foreground">
                  Политика конфиденциальности
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

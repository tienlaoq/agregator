import Link from "next/link";

export function Footer() {
  return (
    <footer className="mt-auto border-t bg-muted/30">
      <div className="mx-auto max-w-7xl px-4 py-8">
        <div className="grid gap-8 sm:grid-cols-3">
          <div>
            <h3 className="mb-3 text-lg font-bold text-primary">🔥 БаняГид</h3>
            <p className="text-sm text-muted-foreground">
              Агрегатор бань и саун России. Найдите идеальное место для отдыха.
            </p>
          </div>
          <div>
            <h4 className="mb-3 text-sm font-semibold">Навигация</h4>
            <ul className="space-y-2 text-sm text-muted-foreground">
              <li>
                <Link href="/venues" className="hover:text-primary">
                  Каталог
                </Link>
              </li>
              <li>
                <Link href="/auth/register" className="hover:text-primary">
                  Регистрация
                </Link>
              </li>
              <li>
                <Link href="/auth/login" className="hover:text-primary">
                  Вход
                </Link>
              </li>
            </ul>
          </div>
          <div>
            <h4 className="mb-3 text-sm font-semibold">Для владельцев</h4>
            <ul className="space-y-2 text-sm text-muted-foreground">
              <li>
                <Link href="/owner/venues" className="hover:text-primary">
                  Панель управления
                </Link>
              </li>
              <li>
                <Link href="/owner/venues/new" className="hover:text-primary">
                  Добавить заведение
                </Link>
              </li>
            </ul>
          </div>
        </div>
        <div className="mt-8 border-t pt-6 text-center text-xs text-muted-foreground">
          &copy; {new Date().getFullYear()} БаняГид. Все права защищены.
        </div>
      </div>
    </footer>
  );
}

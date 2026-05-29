import type { Metadata } from "next"
import Link from "next/link"

export const metadata: Metadata = {
  title: "Пользовательское соглашение",
  description:
    "Условия использования сервиса БаняГид: каталог, бронирование, кабинеты владельца и мастера. Заглушка для последующей юридической доработки.",
  robots: { index: true, follow: true },
}

export default function TermsPage() {
  return (
    <div className="container mx-auto max-w-3xl px-4 py-12">
      <nav className="mb-6 text-sm text-muted-foreground">
        <Link href="/" className="hover:text-foreground">
          Главная
        </Link>
        {" / "}
        <span className="text-foreground">Соглашение</span>
      </nav>
      <h1 className="mb-6 text-3xl font-bold text-foreground">Пользовательское соглашение</h1>
      <div className="max-w-none space-y-4 text-muted-foreground leading-relaxed">
        <p>
          Здесь размещаются условия оказания информационных услуг, правила бронирования, ограничение
          ответственности агрегатора и ссылки на оферту для владельцев. Замените этот текст версией,
          подготовленной юристом под вашу юрисдикцию и модель платежей.
        </p>
        <p>
          Партнёрская программа:{" "}
          <Link href="/partner" className="text-primary underline">
            для владельцев
          </Link>
          .
        </p>
      </div>
    </div>
  )
}

import type { Metadata } from "next"
import Link from "next/link"

export const metadata: Metadata = {
  title: "Согласие на обработку персональных данных",
  description:
    "Согласие на обработку персональных данных пользователей сервиса БаняГид по 152-ФЗ. Заглушка для последующей юридической доработки.",
  robots: { index: true, follow: true },
}

export default function ConsentPage() {
  return (
    <div className="container mx-auto max-w-3xl px-4 py-12">
      <nav className="mb-6 text-sm text-muted-foreground">
        <Link href="/" className="hover:text-foreground">
          Главная
        </Link>
        {" / "}
        <span className="text-foreground">Согласие на обработку ПД</span>
      </nav>
      <h1 className="mb-6 text-3xl font-bold text-foreground">
        Согласие на обработку персональных данных
      </h1>
      <div className="max-w-none space-y-4 text-muted-foreground leading-relaxed">
        <p>
          Здесь размещается текст согласия субъекта на обработку персональных данных в соответствии с
          Федеральным законом № 152-ФЗ: перечень обрабатываемых данных, цели обработки, перечень
          действий и срок действия согласия, порядок отзыва. Замените этот текст версией,
          подготовленной юристом под вашу модель сбора данных.
        </p>
        <p>
          Как именно обрабатываются данные, описано в{" "}
          <Link href="/privacy" className="text-primary underline">
            политике конфиденциальности
          </Link>
          .
        </p>
      </div>
    </div>
  )
}

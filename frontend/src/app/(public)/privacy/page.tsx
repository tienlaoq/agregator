import type { Metadata } from "next"
import Link from "next/link"

export const metadata: Metadata = {
  title: "Политика конфиденциальности",
  description:
    "Как БаняГид обрабатывает персональные данные пользователей и владельцев заведений. Общие положения; уточните юридические формулировки с вашим юристом.",
  robots: { index: true, follow: true },
}

export default function PrivacyPage() {
  return (
    <div className="container mx-auto max-w-3xl px-4 py-12">
      <nav className="mb-6 text-sm text-muted-foreground">
        <Link href="/" className="hover:text-foreground">
          Главная
        </Link>
        {" / "}
        <span className="text-foreground">Конфиденциальность</span>
      </nav>
      <h1 className="mb-6 text-3xl font-bold text-foreground">Политика конфиденциальности</h1>
      <div className="max-w-none space-y-4 text-muted-foreground leading-relaxed">
        <p>
          Эта страница задаёт общий каркас для поисковых систем и пользователей. Актуальный юридический текст
          (оператор персональных данных, цели обработки, сроки хранения, права субъекта) должен быть
          согласован с вашей организацией и размещён здесь вместо заглушки.
        </p>
        <p>
          Обратная связь по данным:{" "}
          <Link href="/support" className="text-primary underline">
            раздел поддержки
          </Link>
          .
        </p>
      </div>
    </div>
  )
}

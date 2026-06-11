import type { Metadata } from "next"
import Link from "next/link"

export const metadata: Metadata = {
  title: "Публичная оферта",
  description:
    "Публичная оферта сервиса БаняГид: условия посредничества при бронировании, порядок оплаты и возвратов. Заглушка для последующей юридической доработки.",
  robots: { index: true, follow: true },
}

export default function OfferPage() {
  return (
    <div className="container mx-auto max-w-3xl px-4 py-12">
      <nav className="mb-6 text-sm text-muted-foreground">
        <Link href="/" className="hover:text-foreground">
          Главная
        </Link>
        {" / "}
        <span className="text-foreground">Оферта</span>
      </nav>
      <h1 className="mb-6 text-3xl font-bold text-foreground">Публичная оферта</h1>
      <div className="max-w-none space-y-4 text-muted-foreground leading-relaxed">
        <p>
          Здесь размещается публичная оферта на оказание информационно-посреднических услуг: предмет
          договора, момент акцепта, порядок онлайн-оплаты через платёжного провайдера, условия
          возврата и ограничение ответственности агрегатора. Замените этот текст версией,
          подготовленной юристом под вашу юрисдикцию и модель платежей.
        </p>
        <p>
          Реквизиты исполнителя и контактные данные указаны в подвале сайта. Сопутствующие документы:{" "}
          <Link href="/privacy" className="text-primary underline">
            политика конфиденциальности
          </Link>{" "}
          и{" "}
          <Link href="/terms" className="text-primary underline">
            пользовательское соглашение
          </Link>
          .
        </p>
      </div>
    </div>
  )
}

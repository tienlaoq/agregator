import type { Metadata } from "next"

import { LegalPlaceholder } from "@/components/banya/legal-placeholder"

export const metadata: Metadata = {
  title: "Регламент жалоб",
  description:
    "Порядок подачи и рассмотрения жалоб на заведения и пар-мастеров на БаняГид. Заглушка для юридической доработки.",
  robots: { index: true, follow: true },
}

export default function ComplaintsPage() {
  return (
    <LegalPlaceholder title="Регламент жалоб">
      <p>
        Здесь размещается порядок подачи и рассмотрения жалоб: как сообщить о
        проблеме (в том числе об инциденте с причинением вреда), сроки
        рассмотрения, действия платформы (передача заведению, приостановка
        карточки, блокировка), порядок обратной связи заявителю. Замените этот
        текст версией, подготовленной юристом.
      </p>
    </LegalPlaceholder>
  )
}

import type { Metadata } from "next"

import { LegalPlaceholder } from "@/components/banya/legal-placeholder"

export const metadata: Metadata = {
  title: "Регламент отмен и возвратов",
  description:
    "Правила отмены бронирований и возврата оплаты на БаняГид. Заглушка для юридической доработки.",
  robots: { index: true, follow: true },
}

export default function RefundsPage() {
  return (
    <LegalPlaceholder title="Регламент отмен и возвратов">
      <p>
        Здесь размещаются правила отмены бронирования и возврата денежных
        средств: сроки отмены без удержания, размеры удержаний, порядок и сроки
        возврата на карту/СБП, случаи невозврата, порядок при отмене со стороны
        заведения. Замените этот текст версией, подготовленной юристом.
      </p>
    </LegalPlaceholder>
  )
}

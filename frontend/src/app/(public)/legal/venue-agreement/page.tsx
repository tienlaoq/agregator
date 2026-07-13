import type { Metadata } from "next"

import { LegalPlaceholder } from "@/components/banya/legal-placeholder"

export const metadata: Metadata = {
  title: "Договор с заведением",
  description:
    "Договор-оферта для владельцев бань и саун, размещающих заведения на БаняГид. Заглушка для юридической доработки.",
  robots: { index: true, follow: true },
}

export default function VenueAgreementPage() {
  return (
    <LegalPlaceholder title="Договор с заведением">
      <p>
        Здесь размещается договор-оферта для владельцев заведений: предмет,
        обязанности сторон, соблюдение техники безопасности и санитарных норм,
        порядок уведомления платформы об инцидентах с посетителями,
        ответственность за качество и безопасность услуг, условия оплаты и
        расторжения. Замените этот текст версией, подготовленной юристом.
      </p>
    </LegalPlaceholder>
  )
}

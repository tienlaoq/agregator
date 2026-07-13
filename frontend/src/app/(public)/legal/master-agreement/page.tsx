import type { Metadata } from "next"

import { LegalPlaceholder } from "@/components/banya/legal-placeholder"

export const metadata: Metadata = {
  title: "Договор с пар-мастером",
  description:
    "Договор-оферта для пар-мастеров, оказывающих услуги через БаняГид. Заглушка для юридической доработки.",
  robots: { index: true, follow: true },
}

export default function MasterAgreementPage() {
  return (
    <LegalPlaceholder title="Договор с пар-мастером">
      <p>
        Здесь размещается договор-оферта для пар-мастеров: предмет, требования к
        квалификации и допуску, ответственность за вред здоровью клиента (в том
        числе по неосторожности), обязанность уведомлять платформу об
        инцидентах, условия выплат и расторжения. Замените этот текст версией,
        подготовленной юристом.
      </p>
    </LegalPlaceholder>
  )
}

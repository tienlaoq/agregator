import type { Metadata } from "next"

import { LegalPlaceholder } from "@/components/banya/legal-placeholder"

export const metadata: Metadata = {
  title: "Лицензионный договор",
  description:
    "Условия использования платформы БаняГид (лицензионное соглашение). Заглушка для юридической доработки.",
  robots: { index: true, follow: true },
}

export default function LicensePage() {
  return (
    <LegalPlaceholder title="Лицензионный договор">
      <p>
        Здесь размещается лицензионный договор на использование платформы:
        предоставляемые права, ограничения использования, интеллектуальная
        собственность, отказ от гарантий и ответственности информационного
        посредника, срок и порядок изменения условий. Замените этот текст
        версией, подготовленной юристом.
      </p>
    </LegalPlaceholder>
  )
}

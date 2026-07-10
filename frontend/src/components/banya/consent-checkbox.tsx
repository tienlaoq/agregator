"use client"

import Link from "next/link"

import { Checkbox } from "@/components/ui/checkbox"

/**
 * Чекбокс согласия на обработку персональных данных для форм сбора ПДн
 * (регистрация, заявка партнёра/мастера). Требование 152-ФЗ (ст. 9): согласие
 * должно быть конкретным, информированным и однозначным. С 01.09.2025 —
 * отдельное, без предустановленной галочки. Поэтому здесь нет defaultChecked:
 * состояние поднято в форму, а сабмит блокируется, пока `checked !== true`.
 */
export function ConsentCheckbox({
  checked,
  onCheckedChange,
  id = "pd-consent",
  className,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  id?: string
  className?: string
}) {
  return (
    <div className={`flex items-start gap-2.5 ${className ?? ""}`}>
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        className="mt-0.5"
        aria-required
      />
      <label htmlFor={id} className="text-sm leading-snug text-muted-foreground">
        Я даю{" "}
        <Link href="/consent" target="_blank" className="text-primary underline">
          согласие на обработку персональных данных
        </Link>{" "}
        и принимаю{" "}
        <Link href="/privacy" target="_blank" className="text-primary underline">
          политику конфиденциальности
        </Link>
        .
      </label>
    </div>
  )
}

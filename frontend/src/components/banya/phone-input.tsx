"use client"

import { useRef } from "react"
import { Input } from "@/components/ui/input"

function formatPhone(digits: string): string {
  const d = digits.replace(/\D/g, "").slice(0, 11)

  if (d.length === 0) return ""

  const parts: string[] = ["+7"]

  const num = d.startsWith("7") || d.startsWith("8") ? d.slice(1) : d

  if (num.length > 0) parts.push(` (${num.slice(0, 3)}`)
  if (num.length >= 3) parts[1] += ")"
  if (num.length > 3) parts.push(` ${num.slice(3, 6)}`)
  if (num.length > 6) parts.push(`-${num.slice(6, 8)}`)
  if (num.length > 8) parts.push(`-${num.slice(8, 10)}`)

  return parts.join("")
}

function extractDigits(value: string): string {
  return value.replace(/\D/g, "")
}

interface PhoneInputProps {
  value: string
  onChange: (value: string) => void
  id?: string
  required?: boolean
  placeholder?: string
}

export function PhoneInput({
  value,
  onChange,
  id,
  required,
  placeholder = "+7 (999) 123-45-67",
}: PhoneInputProps) {
  const inputRef = useRef<HTMLInputElement>(null)

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value
    const digits = extractDigits(raw)

    let normalized = digits
    if (normalized.startsWith("8")) {
      normalized = "7" + normalized.slice(1)
    }
    if (!normalized.startsWith("7") && normalized.length > 0) {
      normalized = "7" + normalized
    }

    const formatted = formatPhone(normalized)
    onChange(formatted)
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Backspace" && value.length <= 3) {
      e.preventDefault()
      onChange("")
    }
  }

  const handleFocus = () => {
    if (!value) {
      onChange("+7 (")
    }
  }

  const handleBlur = () => {
    if (value === "+7 (" || value === "+7") {
      onChange("")
    }
  }

  return (
    <Input
      ref={inputRef}
      id={id}
      type="tel"
      inputMode="numeric"
      placeholder={placeholder}
      value={value}
      onChange={handleChange}
      onKeyDown={handleKeyDown}
      onFocus={handleFocus}
      onBlur={handleBlur}
      required={required}
      maxLength={18}
    />
  )
}

export function getRawPhone(formatted: string): string {
  const digits = formatted.replace(/\D/g, "")
  if (digits.length < 11) return formatted
  return `+${digits}`
}

"use client"

import { useState, useRef, useEffect } from "react"
import { Input } from "@/components/ui/input"
import { MapPin } from "lucide-react"

const STREET_TYPES: Record<string, string> = {
  "улица": "ул.",
  "ул": "ул.",
  "проспект": "пр-т",
  "просп": "пр-т",
  "пр": "пр-т",
  "переулок": "пер.",
  "пер": "пер.",
  "бульвар": "бул.",
  "бул": "бул.",
  "набережная": "наб.",
  "наб": "наб.",
  "площадь": "пл.",
  "пл": "пл.",
  "шоссе": "ш.",
  "ш": "ш.",
  "проезд": "пр-д",
  "тупик": "туп.",
  "аллея": "ал.",
  "линия": "лин.",
  "микрорайон": "мкр.",
  "мкр": "мкр.",
  "квартал": "кв-л",
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function normalizeAddress(raw: string): string {
  let addr = raw.trim()
  if (!addr) return ""

  addr = addr.replace(/\s+/g, " ")

  addr = addr.replace(/,\s*/g, ", ")

  // "дом 15" / "дом15" → "д. 15"
  addr = addr.replace(/\bдом\s*\.?\s*(\d)/gi, "д. $1")
  // "д15" / "д 15" (without dot) → "д. 15"
  addr = addr.replace(/\bд\s+(\d)/gi, "д. $1")
  addr = addr.replace(/\bд(\d)/gi, "д. $1")
  // "корпус 2" / "корп 2" → "к. 2"
  addr = addr.replace(/\b(?:корпус|корп)\.?\s*(\d)/gi, "к. $1")
  addr = addr.replace(/\bк\s+(\d)/gi, "к. $1")
  addr = addr.replace(/\bк(\d)/gi, "к. $1")
  // "строение 1" / "стр 1" → "стр. 1"
  addr = addr.replace(/\b(?:строение|стр)\.?\s*(\d)/gi, "стр. $1")
  // "квартира 5" / "кв 5" → "кв. 5"
  addr = addr.replace(/\b(?:квартира|кв)\.?\s*(\d)/gi, "кв. $1")

  for (const [full, abbr] of Object.entries(STREET_TYPES)) {
    const regex = new RegExp(`\\b${full}\\.?\\s`, "gi")
    addr = addr.replace(regex, `${abbr} `)
  }

  const parts = addr.split(", ").map((p) => capitalize(p.trim()))
  addr = parts.join(", ")

  return addr
}

function generateTemplates(input: string, city?: string): string[] {
  if (input.length < 2) return []

  const q = input.toLowerCase().trim()
  const templates: string[] = []

  const streetNames = [
    "Ленина", "Мира", "Пушкина", "Гагарина", "Советская", "Центральная",
    "Молодёжная", "Садовая", "Лесная", "Новая", "Школьная", "Парковая",
    "Комсомольская", "Октябрьская", "Первомайская", "Заводская",
  ]

  const prefixes = ["ул.", "пр-т", "пер.", "бул.", "наб.", "ш."]

  for (const prefix of prefixes) {
    for (const name of streetNames) {
      const full = `${prefix} ${name}`
      if (full.toLowerCase().includes(q) || name.toLowerCase().startsWith(q)) {
        templates.push(full)
      }
    }
  }

  if (/^\d/.test(q)) {
    templates.push(`д. ${q}`)
  }

  return [...new Set(templates)].slice(0, 6)
}

interface AddressSuggestProps {
  value: string
  onChange: (value: string) => void
  city?: string
  placeholder?: string
}

export function AddressSuggest({
  value,
  onChange,
  city,
  placeholder = "ул. Банная, д. 15",
}: AddressSuggestProps) {
  const [suggestions, setSuggestions] = useState<string[]>([])
  const [open, setOpen] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (value.length >= 2) {
      setSuggestions(generateTemplates(value, city))
    } else {
      setSuggestions([])
    }
  }, [value, city])

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [])

  const handleBlur = () => {
    setTimeout(() => {
      if (value.trim()) {
        onChange(normalizeAddress(value))
      }
      setOpen(false)
    }, 200)
  }

  return (
    <div ref={wrapperRef} className="relative">
      <Input
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={handleBlur}
        placeholder={placeholder}
      />
      {open && suggestions.length > 0 && (
        <div className="absolute top-full z-50 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
          {suggestions.map((s, i) => (
            <button
              key={i}
              type="button"
              className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground"
              onMouseDown={(e) => {
                e.preventDefault()
                onChange(s)
                setOpen(false)
              }}
            >
              <MapPin className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              <span>{s}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

export { normalizeAddress }

"use client"

import { useState, useRef, useEffect, useCallback } from "react"
import { Input } from "@/components/ui/input"
import { MapPin, Loader2 } from "lucide-react"
import { isStreetSuggestReady } from "@/lib/address-suggest-rules"

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

  addr = addr.replace(/\bдом\s*\.?\s*(\d)/gi, "д. $1")
  addr = addr.replace(/\bд\s+(\d)/gi, "д. $1")
  addr = addr.replace(/\bд(\d)/gi, "д. $1")
  addr = addr.replace(/\b(?:корпус|корп)\.?\s*(\d)/gi, "к. $1")
  addr = addr.replace(/\bк\s+(\d)/gi, "к. $1")
  addr = addr.replace(/\bк(\d)/gi, "к. $1")
  addr = addr.replace(/\b(?:строение|стр)\.?\s*(\d)/gi, "стр. $1")
  addr = addr.replace(/\b(?:квартира|кв)\.?\s*(\d)/gi, "кв. $1")

  for (const [full, abbr] of Object.entries(STREET_TYPES)) {
    const regex = new RegExp(`\\b${full}\\.?\\s`, "gi")
    addr = addr.replace(regex, `${abbr} `)
  }

  const parts = addr.split(", ").map((p) => capitalize(p.trim()))
  addr = parts.join(", ")

  return addr
}

/** Same rule as API: meaningful street text after optional type prefix. */
function extractStreetCore(segment: string): string {
  let s = segment.trim()
  s = s.replace(
    /^(?:ул\.?|улица|пр-?т\.?|проспект|пер\.?|переулок|бул\.?|бульвар|наб\.?|набережная|ш\.?|шоссе|пр-д\.?|проезд|туп\.?|тупик|ал\.?|аллея|лин\.?|линия|мкр\.?|микрорайон)\s+/i,
    "",
  )
  return s.trim()
}

function shouldRequestSuggest(value: string): boolean {
  const firstSegment = value.split(",")[0]?.trim() ?? value.trim()
  if (firstSegment.length < 2) return false
  const core = extractStreetCore(firstSegment)
  const needLen = Math.max(4, Math.ceil(firstSegment.length / 2))
  return core.length >= needLen
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
  const [loading, setLoading] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  const fetchSuggestions = useCallback(
    async (street: string, cityName: string) => {
      abortRef.current?.abort()
      if (!cityName.trim() || !isStreetSuggestReady(street)) {
        setSuggestions([])
        setLoading(false)
        return
      }
      const ac = new AbortController()
      abortRef.current = ac
      setLoading(true)
      try {
        const q = new URLSearchParams({ city: cityName.trim(), street: street.trim() })
        const res = await fetch(`/api/address-suggest?${q}`, { signal: ac.signal })
        if (!res.ok) {
          setSuggestions([])
          return
        }
        const data = (await res.json()) as { suggestions?: string[] }
        setSuggestions(Array.isArray(data.suggestions) ? data.suggestions : [])
      } catch (e) {
        if ((e as Error).name === "AbortError") return
        setSuggestions([])
      } finally {
        if (!ac.signal.aborted) setLoading(false)
      }
    },
    [],
  )

  useEffect(() => {
    if (!city?.trim()) {
      setSuggestions([])
      setLoading(false)
      return
    }
    const t = window.setTimeout(() => {
      void fetchSuggestions(value, city)
    }, 380)
    return () => {
      window.clearTimeout(t)
    }
  }, [value, city, fetchSuggestions])

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

  const cityMissing = !city?.trim()
  const showPanel =
    open && !cityMissing && (loading || suggestions.length > 0)

  return (
    <div className="space-y-1">
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
          autoComplete="street-address"
        />
        {showPanel && (
          <div className="absolute top-full z-50 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
          {loading && suggestions.length === 0 && (
            <div className="flex items-center gap-2 px-2 py-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 shrink-0 animate-spin" />
              Поиск адреса…
            </div>
          )}
          {suggestions.map((s, i) => (
            <button
              key={`${s}-${i}`}
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
      {cityMissing && (
        <p className="text-xs text-muted-foreground">
          Сначала выберите город — подсказки улиц привязаны к нему.
        </p>
      )}
    </div>
  )
}

export { normalizeAddress }

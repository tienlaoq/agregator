"use client"

import { useEffect, useState } from "react"
import { format } from "date-fns"
import { getVenueAvailability } from "@/lib/api"

interface UseSlotAvailabilityOptions {
  /** slug заведения */
  slug: string | undefined
  /** Выбранная дата */
  date: Date | undefined
  /** Продолжительность слота в минутах (для фильтрации подходящих окон) */
  durationMin?: number
}

interface UseSlotAvailabilityResult {
  availableSlots: string[]
  slotsLoading: boolean
  /** Set для O(1) проверки — `availableStartSet.has(time)` */
  availableStartSet: Set<string>
}

/**
 * Загружает доступные временны́е слоты для заведения на выбранную дату.
 * При смене даты или slug сбрасывает слоты и делает повторный запрос.
 */
export function useSlotAvailability({
  slug,
  date,
  durationMin = 120,
}: UseSlotAvailabilityOptions): UseSlotAvailabilityResult {
  const [availableSlots, setAvailableSlots] = useState<string[]>([])
  const [slotsLoading, setSlotsLoading] = useState(false)

  useEffect(() => {
    if (!slug || !date) {
      setAvailableSlots([])
      return
    }
    const iso = format(date, "yyyy-MM-dd")
    let active = true
    setSlotsLoading(true)
    setAvailableSlots([])

    getVenueAvailability(slug, iso, durationMin)
      .then((r) => {
        if (!active) return
        setAvailableSlots(r.available_slots ?? [])
      })
      .catch(() => {
        if (!active) return
        setAvailableSlots([])
      })
      .finally(() => {
        if (active) setSlotsLoading(false)
      })

    return () => {
      active = false
    }
  }, [slug, date, durationMin])

  return {
    availableSlots,
    slotsLoading,
    availableStartSet: new Set(availableSlots),
  }
}

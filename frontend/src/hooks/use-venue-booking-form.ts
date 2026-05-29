"use client"

import { useEffect, useMemo, useState } from "react"
import { format, isBefore, startOfDay } from "date-fns"
import { createBooking, formatApiErrorMessage } from "@/lib/api"
import {
  defaultEndTimeForDuration,
  endTimeOptionsThirtyMinutes,
  slotLengthMinutes,
  thirtyMinuteStartGridFrom10To22,
} from "@/lib/booking-slot-time"
import { venueServiceDurationMinutes } from "@/lib/types"
import type { Venue } from "@/lib/types"
import { useSlotAvailability } from "@/hooks/use-slot-availability"

// ─── Helpers ────────────────────────────────────────────────────────────────

/** Длительность слота для проверки доступности: максимум среди выбранных пакетов. */
export function maxSlotMinutesForSelectedServices(
  ids: string[],
  services: Venue["services"],
): number {
  if (ids.length === 0) return 120
  let m = 30
  for (const id of ids) {
    const s = services?.find((x) => x.id === id)
    if (!s) continue
    const d = venueServiceDurationMinutes(s)
    m = Math.max(m, d > 0 ? d : 120)
  }
  return Math.min(720, m)
}

/** Почасовая база для подсказки: max(цена заведения, выбранные залы). */
export function effectiveHourlyRateRub(venue: Venue, hallIds: string[]): number {
  let base = Math.max(0, Number(venue.price_from) || 0)
  for (const id of hallIds) {
    const h = venue.halls?.find((x) => x.id === id)
    if (h && Number(h.price_from) > 0) {
      base = Math.max(base, Number(h.price_from))
    }
  }
  return base
}

// ─── Hook ────────────────────────────────────────────────────────────────────

interface UseVenueBookingFormOptions {
  venue: Venue | null
  /** Slug заведения — при смене сбрасывает выбор услуг/залов */
  slug: string
}

/**
 * Управляет полной формой бронирования venue.
 * Включает useSlotAvailability внутри себя, чтобы slotDurationMin (вычисленный
 * из выбранных услуг) передавался в запрос доступных слотов без circular dependency.
 */
export function useVenueBookingForm({ venue, slug }: UseVenueBookingFormOptions) {
  const [date, setDate] = useState<Date>()
  const [time, setTime] = useState("")
  const [timeTo, setTimeTo] = useState("")
  const [selectedServiceIds, setSelectedServiceIds] = useState<string[]>([])
  const [selectedHallIds, setSelectedHallIds] = useState<string[]>([])
  const [guests, setGuests] = useState(2)
  const [booking, setBooking] = useState(false)
  const [bookingMsg, setBookingMsg] = useState("")

  // Сбрасываем услуги/залы при смене заведения
  useEffect(() => {
    setSelectedServiceIds([])
    setSelectedHallIds([])
  }, [slug, venue?.id])

  // Сбрасываем дату если она в прошлом (при смене slug)
  useEffect(() => {
    setDate((d) => {
      if (!d) return d
      return isBefore(startOfDay(d), startOfDay(new Date())) ? undefined : d
    })
  }, [slug])

  const slotDurationMin = useMemo(() => {
    if (time && timeTo) {
      const m = slotLengthMinutes(time, timeTo)
      if (m != null && m >= 30 && m <= 720) return m
    }
    if (selectedServiceIds.length > 0 && venue) {
      return maxSlotMinutesForSelectedServices(selectedServiceIds, venue.services)
    }
    return 120
  }, [time, timeTo, selectedServiceIds, venue])

  // Слоты: вызываем хук здесь, передавая slotDurationMin — нет circular dependency.
  const { availableSlots, slotsLoading, availableStartSet } = useSlotAvailability({
    slug,
    date,
    durationMin: slotDurationMin,
  })

  const slotValid = useMemo(() => {
    if (!time || !timeTo) return false
    const m = slotLengthMinutes(time, timeTo)
    return m != null && m >= 30 && m <= 720 && m % 30 === 0
  }, [time, timeTo])

  const visitEndOptions = useMemo(() => endTimeOptionsThirtyMinutes(time), [time])
  const startTimeGrid = useMemo(() => thirtyMinuteStartGridFrom10To22(), [])

  const priceHint = useMemo(() => {
    if (!venue) return null
    const hourlyBase = effectiveHourlyRateRub(venue, selectedHallIds)
    if (selectedServiceIds.length > 0) {
      let sumRub = 0
      let hourlySlots = 0
      for (const id of selectedServiceIds) {
        const s = venue.services?.find((x) => x.id === id)
        if (!s) continue
        if (Number(s.price) > 0) sumRub += Number(s.price)
        else hourlySlots++
      }
      if (hourlySlots > 1) return null
      if (hourlySlots === 1 && hourlyBase > 0 && slotValid) {
        const mins = slotLengthMinutes(time, timeTo)
        if (mins != null) sumRub += hourlyBase * Math.ceil(mins / 60)
      }
      if (sumRub > 0) return `${sumRub.toLocaleString("ru-RU")} ₽`
      return null
    }
    if (hourlyBase > 0 && slotValid) {
      const mins = slotLengthMinutes(time, timeTo)
      if (mins != null) {
        return `≈ ${(hourlyBase * Math.ceil(mins / 60)).toLocaleString("ru-RU")} ₽`
      }
    }
    return null
  }, [venue, selectedServiceIds, selectedHallIds, time, timeTo, slotValid])

  // Автоустановка timeTo при смене time или набора услуг
  useEffect(() => {
    if (!time || !venue) {
      if (!time) setTimeTo("")
      return
    }
    let addMin = 120
    if (selectedServiceIds.length > 0) {
      addMin = Math.max(30, maxSlotMinutesForSelectedServices(selectedServiceIds, venue.services))
    }
    setTimeTo(defaultEndTimeForDuration(time, addMin))
  }, [time, selectedServiceIds, venue])

  // Корректируем timeTo если оно выпало за допустимый диапазон
  useEffect(() => {
    if (!time || !timeTo || !venue) return
    const opts = endTimeOptionsThirtyMinutes(time)
    if (opts.length === 0 || opts.includes(timeTo)) return
    let addMin = 120
    if (selectedServiceIds.length > 0) {
      addMin = Math.max(30, maxSlotMinutesForSelectedServices(selectedServiceIds, venue.services))
    }
    setTimeTo(defaultEndTimeForDuration(time, addMin))
  }, [time, timeTo, selectedServiceIds, venue])

  // Сбрасываем time если слот стал недоступен после обновления
  useEffect(() => {
    if (time && !slotsLoading && !availableStartSet.has(time)) {
      setTime("")
    }
  }, [time, slotsLoading, availableStartSet])

  const handleBook = async () => {
    if (!venue || !date || !time || !slotValid || !timeTo) return
    if (isBefore(startOfDay(date), startOfDay(new Date()))) {
      setBookingMsg("Нельзя выбрать прошедшую дату.")
      return
    }
    setBooking(true)
    setBookingMsg("")
    try {
      const b = await createBooking({
        venue_id: venue.id,
        date: format(date, "yyyy-MM-dd"),
        time_from: time,
        time_to: timeTo,
        guests,
        ...(selectedServiceIds.length > 0 ? { service_ids: selectedServiceIds } : {}),
        ...(selectedHallIds.length > 0 ? { hall_ids: selectedHallIds } : {}),
      })
      if (b.payment_url) {
        window.location.assign(b.payment_url)
        return
      }
      setBookingMsg("Бронирование создано!")
    } catch (e) {
      setBookingMsg(
        formatApiErrorMessage(e, "Не удалось забронировать. Попробуйте позже."),
      )
    } finally {
      setBooking(false)
    }
  }

  return {
    // Date / time
    date, setDate,
    time, setTime,
    timeTo, setTimeTo,
    slotDurationMin,
    slotValid,
    visitEndOptions,
    startTimeGrid,
    // Slots
    availableSlots,
    slotsLoading,
    availableStartSet,
    // Services / halls
    selectedServiceIds, setSelectedServiceIds,
    selectedHallIds, setSelectedHallIds,
    // Guests
    guests, setGuests,
    // Submit
    booking,
    bookingMsg, setBookingMsg,
    priceHint,
    handleBook,
  }
}

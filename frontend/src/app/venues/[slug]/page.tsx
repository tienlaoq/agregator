"use client"

import { useEffect, useMemo, useState } from "react"
import { useParams } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Calendar } from "@/components/ui/calendar"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Progress } from "@/components/ui/progress"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { format, isBefore, startOfDay } from "date-fns"
import { ru } from "date-fns/locale"
import {
  Star,
  MapPin,
  Phone,
  CalendarIcon,
  Users,
  Minus,
  Plus,
  ShieldCheck,
  ImageIcon,
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
} from "lucide-react"
import Link from "next/link"
import {
  createBooking,
  formatApiErrorMessage,
  getVenueAvailability,
  getVenueBySlug,
  getVenueReviews,
  venueMediaUrl,
} from "@/lib/api"
import type { Venue, Review } from "@/lib/types"
import {
  VENUE_SOCIAL_PUBLIC_LABELS,
  VENUE_TYPE_LABELS,
  venueServiceDurationMinutes,
  type VenueSocialLinkKey,
} from "@/lib/types"
import { useAuthStore } from "@/store/auth"
import { cn } from "@/lib/utils"

function socialHref(url: string): string {
  const u = url.trim()
  if (!u) return "#"
  if (/^https?:\/\//i.test(u)) return u
  return `https://${u}`
}

function hhmmToMinutes(s: string): number | null {
  const m = s.match(/^(\d{1,2}):(\d{2})$/)
  if (!m) return null
  const h = parseInt(m[1], 10)
  const min = parseInt(m[2], 10)
  if (
    !Number.isFinite(h) ||
    !Number.isFinite(min) ||
    min < 0 ||
    min > 59 ||
    h < 0 ||
    h > 23
  ) {
    return null
  }
  return h * 60 + min
}

/** Сетка «время начала»: 10:00–22:00 с шагом 30 мин (совпадает с api-gateway). */
function thirtyMinuteStartGridFrom10To22(): string[] {
  const out: string[] = []
  for (let m = 10 * 60; m <= 22 * 60; m += 30) {
    out.push(
      `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`,
    )
  }
  return out
}

/** Окончание визита в тот же день: от +30 мин до +12 ч, шаг 30 минут. */
function endTimeOptionsThirtyMinutes(startHHMM: string): string[] {
  const startTotal = hhmmToMinutes(startHHMM)
  if (startTotal == null) return []
  const out: string[] = []
  for (let delta = 30; delta <= 720; delta += 30) {
    const total = startTotal + delta
    if (total >= 24 * 60) break
    const eh = Math.floor(total / 60)
    const em = total % 60
    out.push(`${String(eh).padStart(2, "0")}:${String(em).padStart(2, "0")}`)
  }
  return out
}

function defaultEndTimeForDuration(
  startHHMM: string,
  preferredDurMin: number,
): string {
  const opts = endTimeOptionsThirtyMinutes(startHHMM)
  if (opts.length === 0) return ""
  const startM = hhmmToMinutes(startHHMM)
  if (startM == null) return opts[0] ?? ""
  const need = Math.max(30, Math.min(720, preferredDurMin))
  for (const o of opts) {
    const om = hhmmToMinutes(o)
    if (om != null && om - startM >= need) return o
  }
  return opts[opts.length - 1] ?? ""
}

/** Длина интервала в минутах (тот же день, to > from). */
function slotLengthMinutes(from: string, to: string): number | null {
  const [fh, fm] = from.split(":").map((x) => parseInt(x, 10))
  const [th, tm] = to.split(":").map((x) => parseInt(x, 10))
  if (![fh, fm, th, tm].every((n) => Number.isFinite(n))) return null
  const v = th * 60 + tm - (fh * 60 + fm)
  return v > 0 ? v : null
}

/** Слайды карусели: порядок как в кабинете (обложка → sort_order), без дублей по URL. */
function venueCarouselSlides(venue: Venue): { id: string; src: string }[] {
  const seen = new Set<string>()
  const out: { id: string; src: string }[] = []

  const sorted = [...(venue.photos ?? [])].sort((a, b) => {
    if (Boolean(a.is_cover) !== Boolean(b.is_cover)) return a.is_cover ? -1 : 1
    return (a.sort_order ?? 0) - (b.sort_order ?? 0)
  })
  for (const p of sorted) {
    const src = venueMediaUrl(p.url)
    if (!src || seen.has(src)) continue
    seen.add(src)
    out.push({ id: p.id, src })
  }

  const legacy = venue.image_url?.trim()
  if (legacy) {
    const src = venueMediaUrl(legacy)
    if (src && !seen.has(src)) {
      out.push({ id: "legacy-image-url", src })
    }
  }

  return out
}

export default function VenueDetailPage() {
  const params = useParams()
  const slug = params.slug as string
  const user = useAuthStore((s) => s.user)

  const [venue, setVenue] = useState<Venue | null>(null)
  const [reviews, setReviews] = useState<Review[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  const [date, setDate] = useState<Date>()
  const [time, setTime] = useState("")
  const [timeTo, setTimeTo] = useState("")
  /** «none» — почасовая оплата по price_from; иначе id услуги */
  const [serviceId, setServiceId] = useState<string>("none")
  const [guests, setGuests] = useState(2)
  const [booking, setBooking] = useState(false)
  const [bookingMsg, setBookingMsg] = useState("")
  const [availableSlots, setAvailableSlots] = useState<string[]>([])
  const [slotsLoading, setSlotsLoading] = useState(false)
  const [galleryIndex, setGalleryIndex] = useState(0)

  const slotDurationMin = useMemo(() => {
    if (time && timeTo) {
      const m = slotLengthMinutes(time, timeTo)
      if (m != null && m >= 30 && m <= 720) return m
    }
    if (serviceId !== "none" && venue) {
      const s = venue.services?.find((x) => x.id === serviceId)
      if (s) {
        const d = venueServiceDurationMinutes(s)
        return Math.min(720, Math.max(30, d > 0 ? d : 120))
      }
    }
    return 120
  }, [time, timeTo, serviceId, venue])

  const slotValid = useMemo(() => {
    if (!time || !timeTo) return false
    const m = slotLengthMinutes(time, timeTo)
    return m != null && m >= 30 && m <= 720 && m % 30 === 0
  }, [time, timeTo])

  const visitEndOptions = useMemo(() => endTimeOptionsThirtyMinutes(time), [time])

  const startTimeGrid = useMemo(() => thirtyMinuteStartGridFrom10To22(), [])
  const availableStartSet = useMemo(
    () => new Set(availableSlots),
    [availableSlots],
  )

  const priceHint = useMemo(() => {
    if (!venue) return null
    if (serviceId !== "none") {
      const s = venue.services?.find((x) => x.id === serviceId)
      if (s && Number(s.price) > 0) {
        return `${Number(s.price).toLocaleString("ru-RU")} ₽`
      }
    }
    if (venue.price_from > 0 && slotValid) {
      const mins = slotLengthMinutes(time, timeTo)
      if (mins != null) {
        const hours = Math.ceil(mins / 60)
        return `≈ ${(venue.price_from * hours).toLocaleString("ru-RU")} ₽`
      }
    }
    return null
  }, [venue, serviceId, time, timeTo, slotValid])

  useEffect(() => {
    if (!slug) return
    setLoading(true)
    getVenueBySlug(slug)
      .then((v) => {
        setVenue(v)
        return getVenueReviews(v.id)
      })
      .then((r) => setReviews(r))
      .catch(() => setError("Заведение не найдено"))
      .finally(() => setLoading(false))
  }, [slug])

  useEffect(() => {
    setGalleryIndex(0)
  }, [slug, venue?.id])

  useEffect(() => {
    setDate((d) => {
      if (!d) return d
      return isBefore(startOfDay(d), startOfDay(new Date())) ? undefined : d
    })
  }, [slug])

  useEffect(() => {
    if (!slug || !date) {
      setAvailableSlots([])
      return
    }
    const iso = format(date, "yyyy-MM-dd")
    setSlotsLoading(true)
    setAvailableSlots([])
    getVenueAvailability(slug, iso, slotDurationMin)
      .then((r) => setAvailableSlots(r.available_slots ?? []))
      .catch(() => setAvailableSlots([]))
      .finally(() => setSlotsLoading(false))
  }, [slug, date, slotDurationMin])

  useEffect(() => {
    if (!time || !venue) {
      if (!time) setTimeTo("")
      return
    }
    let addMin = 120
    if (serviceId !== "none") {
      const s = venue.services?.find((x) => x.id === serviceId)
      if (s) {
        const d = venueServiceDurationMinutes(s)
        addMin = Math.max(30, d > 0 ? d : 120)
      }
    }
    setTimeTo(defaultEndTimeForDuration(time, addMin))
  }, [time, serviceId, venue])

  useEffect(() => {
    if (!time || !timeTo || !venue) return
    const opts = endTimeOptionsThirtyMinutes(time)
    if (opts.length === 0 || opts.includes(timeTo)) return
    let addMin = 120
    if (serviceId !== "none") {
      const s = venue.services?.find((x) => x.id === serviceId)
      if (s) {
        const d = venueServiceDurationMinutes(s)
        addMin = Math.max(30, d > 0 ? d : 120)
      }
    }
    setTimeTo(defaultEndTimeForDuration(time, addMin))
  }, [time, timeTo, serviceId, venue])

  useEffect(() => {
    if (time && availableSlots.length > 0 && !availableStartSet.has(time)) {
      setTime("")
    }
  }, [time, availableSlots.length, availableStartSet])

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
        ...(serviceId !== "none" ? { service_id: serviceId } : {}),
      })
      if (b.payment_url) {
        window.location.assign(b.payment_url)
        return
      }
      setBookingMsg("Бронирование создано!")
    } catch (e) {
      setBookingMsg(
        formatApiErrorMessage(
          e,
          "Не удалось забронировать. Попробуйте позже.",
        ),
      )
    } finally {
      setBooking(false)
    }
  }

  if (loading) {
    return (
      <div className="container mx-auto px-4 py-16">
        <div className="grid gap-8 lg:grid-cols-3">
          <div className="lg:col-span-2 space-y-6">
            <div className="h-80 animate-pulse rounded-xl bg-muted" />
            <div className="h-8 w-2/3 animate-pulse rounded bg-muted" />
            <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
          </div>
          <div className="h-96 animate-pulse rounded-xl bg-muted" />
        </div>
      </div>
    )
  }

  if (error || !venue) {
    return (
      <div className="container mx-auto flex min-h-[50vh] flex-col items-center justify-center px-4 py-16 text-center">
        <h2 className="mb-2 text-2xl font-bold">{error || "Заведение не найдено"}</h2>
        <Link href="/venues">
          <Button variant="outline" className="mt-4 gap-2">
            <ArrowLeft className="h-4 w-4" />
            К каталогу
          </Button>
        </Link>
      </div>
    )
  }

  const ratingBreakdown: Record<number, number> = { 5: 0, 4: 0, 3: 0, 2: 0, 1: 0 }
  reviews.forEach((r) => {
    const star = Math.min(5, Math.max(1, Math.round(r.rating)))
    ratingBreakdown[star]++
  })
  const totalReviews = reviews.length

  const carouselSlides = venueCarouselSlides(venue)
  const galleryActiveIndex =
    carouselSlides.length > 0
      ? Math.min(galleryIndex, carouselSlides.length - 1)
      : 0
  const activeSlide = carouselSlides[galleryActiveIndex]

  return (
    <section className="bg-background py-10 md:py-16">
      <div className="container mx-auto px-4">
        <Link href="/venues" className="mb-6 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" />
          Назад к каталогу
        </Link>

        <div className="grid gap-8 lg:grid-cols-3">
          {/* Main Content */}
          <div className="lg:col-span-2">
            {/* Галерея: крупное фото + ряд миниатюр под ним */}
            <div className="mb-8">
              {carouselSlides.length > 0 && activeSlide ? (
                <div className="space-y-3">
                  <div
                    className="relative outline-none"
                    role="region"
                    aria-label={`Фотографии: ${venue.name}`}
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (carouselSlides.length < 2) return
                      if (e.key === "ArrowLeft") {
                        e.preventDefault()
                        setGalleryIndex(
                          (i) =>
                            (i - 1 + carouselSlides.length) %
                            carouselSlides.length,
                        )
                      } else if (e.key === "ArrowRight") {
                        e.preventDefault()
                        setGalleryIndex((i) => (i + 1) % carouselSlides.length)
                      }
                    }}
                  >
                    <div className="relative aspect-[16/9] overflow-hidden rounded-xl bg-muted">
                      <img
                        src={activeSlide.src}
                        alt={`${venue.name} — фото ${galleryActiveIndex + 1} из ${carouselSlides.length}`}
                        className="h-full w-full object-cover"
                      />
                    </div>
                    {carouselSlides.length > 1 ? (
                      <>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="absolute left-2 top-1/2 z-10 size-9 -translate-y-1/2 border-border/80 bg-background/90 shadow-sm hover:bg-background"
                          aria-label="Предыдущее фото"
                          onClick={() =>
                            setGalleryIndex(
                              (i) =>
                                (i - 1 + carouselSlides.length) %
                                carouselSlides.length,
                            )
                          }
                        >
                          <ChevronLeft className="size-5" />
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="absolute right-2 top-1/2 z-10 size-9 -translate-y-1/2 border-border/80 bg-background/90 shadow-sm hover:bg-background"
                          aria-label="Следующее фото"
                          onClick={() =>
                            setGalleryIndex(
                              (i) => (i + 1) % carouselSlides.length,
                            )
                          }
                        >
                          <ChevronRight className="size-5" />
                        </Button>
                      </>
                    ) : null}
                  </div>
                  <div
                    className="flex gap-2 overflow-x-auto pb-1 pt-0.5 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
                    role="tablist"
                    aria-label="Миниатюры фотографий"
                  >
                    {carouselSlides.map((slide, i) => (
                      <button
                        key={slide.id}
                        type="button"
                        role="tab"
                        aria-selected={i === galleryActiveIndex}
                        aria-label={`Фото ${i + 1}`}
                        onClick={() => setGalleryIndex(i)}
                        className={cn(
                          "relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-muted transition-[opacity,box-shadow]",
                          i === galleryActiveIndex
                            ? "ring-2 ring-primary ring-offset-2 ring-offset-background"
                            : "opacity-90 hover:opacity-100",
                        )}
                      >
                        <img
                          src={slide.src}
                          alt=""
                          className="h-full w-full object-cover"
                        />
                      </button>
                    ))}
                  </div>
                </div>
              ) : (
                <div className="flex aspect-[16/9] items-center justify-center rounded-xl bg-muted">
                  <div className="flex flex-col items-center gap-2 text-muted-foreground/40">
                    <ImageIcon className="h-16 w-16" />
                    <span className="text-sm">Фото ещё не добавлены</span>
                  </div>
                </div>
              )}
            </div>

            {/* Venue Header */}
            <div className="mb-8">
              <div className="mb-4 flex flex-wrap items-center gap-3">
                <Badge className="bg-primary/10 text-primary border-primary/20">
                  {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
                </Badge>
                {venue.rating > 0 && (
                  <div className="flex items-center gap-1">
                    <Star className="h-5 w-5 fill-amber-400 text-amber-400" />
                    <span className="font-semibold">{venue.rating.toFixed(1)}</span>
                    <span className="text-muted-foreground">({venue.review_count} отзывов)</span>
                  </div>
                )}
              </div>
              <h1 className="mb-4 text-3xl font-bold text-foreground md:text-4xl">{venue.name}</h1>
              <div className="flex flex-col gap-2 text-muted-foreground">
                <div className="flex items-center gap-2">
                  <MapPin className="h-5 w-5 shrink-0" />
                  {venue.city}{venue.address ? `, ${venue.address}` : ""}
                </div>
                {venue.phone && (
                  <div className="flex items-center gap-2">
                    <Phone className="h-5 w-5 shrink-0" />
                    {venue.phone}
                  </div>
                )}
                {venue.social_links &&
                  typeof venue.social_links === "object" &&
                  !Array.isArray(venue.social_links) && (
                    <div className="mt-1 flex flex-wrap gap-x-4 gap-y-2">
                      {(Object.entries(venue.social_links) as [string, unknown][])
                        .filter(
                          ([, u]) =>
                            typeof u === "string" && u.trim().length > 0,
                        )
                        .map(([key, u]) => {
                          const url = String(u).trim()
                          const label =
                            VENUE_SOCIAL_PUBLIC_LABELS[
                              key as VenueSocialLinkKey
                            ] ?? key
                          return (
                            <a
                              key={key}
                              href={socialHref(url)}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
                            >
                              <ExternalLink className="h-4 w-4 shrink-0" />
                              {label}
                            </a>
                          )
                        })}
                    </div>
                  )}
              </div>
            </div>

            {/* Price */}
            {venue.price_from > 0 && (
              <div className="mb-8">
                <span className="text-2xl font-bold text-primary">
                  от {venue.price_from.toLocaleString("ru-RU")} ₽/час
                </span>
              </div>
            )}

            {/* Description */}
            {venue.description && (
              <div className="mb-8">
                <h2 className="mb-4 text-xl font-semibold text-foreground">Описание</h2>
                <p className="leading-relaxed text-muted-foreground">{venue.description}</p>
              </div>
            )}

            {/* Services */}
            {venue.services && venue.services.length > 0 && (
              <div className="mb-8">
                <h2 className="mb-4 text-xl font-semibold text-foreground">Услуги и цены</h2>
                <div className="space-y-3">
                  {venue.services.map((svc) => (
                    <Card key={svc.id} className="border-border">
                      <CardContent className="flex items-center justify-between p-4">
                        <div>
                          <p className="font-medium text-card-foreground">{svc.name}</p>
                          {venueServiceDurationMinutes(svc) > 0 && (
                            <p className="text-sm text-muted-foreground">
                              {venueServiceDurationMinutes(svc)} мин
                            </p>
                          )}
                        </div>
                        {svc.price > 0 && (
                          <span className="font-semibold text-primary">
                            {svc.price.toLocaleString("ru-RU")} ₽
                          </span>
                        )}
                      </CardContent>
                    </Card>
                  ))}
                </div>
              </div>
            )}

            {/* Залы */}
            {venue.halls && venue.halls.length > 0 ? (
              <div className="mb-8">
                <h2 className="mb-4 text-xl font-semibold text-foreground">Залы</h2>
                <div className="space-y-6">
                  {venue.halls.map((hall) => {
                    const hPhotos = [...(hall.photos ?? [])].sort(
                      (a, b) =>
                        (a.sort_order ?? 0) - (b.sort_order ?? 0),
                    )
                    return (
                      <Card key={hall.id} className="border-border overflow-hidden">
                        <CardContent className="p-0">
                          {hPhotos.length > 0 ? (
                            <div className="grid grid-cols-2 gap-0.5 sm:grid-cols-3 md:grid-cols-4">
                              {hPhotos.slice(0, 8).map((p) => (
                                <div
                                  key={p.id}
                                  className="relative aspect-[4/3] bg-muted"
                                >
                                  <img
                                    src={venueMediaUrl(p.url)}
                                    alt=""
                                    className="h-full w-full object-cover"
                                  />
                                </div>
                              ))}
                            </div>
                          ) : null}
                          <div className="space-y-3 p-4 sm:p-5">
                            <h3 className="text-lg font-semibold text-foreground">
                              {hall.name}
                            </h3>
                            <div className="flex flex-wrap gap-3 text-sm text-muted-foreground">
                              {hall.price_from > 0 ? (
                                <span>
                                  от{" "}
                                  <span className="font-semibold text-primary">
                                    {hall.price_from.toLocaleString("ru-RU")} ₽
                                  </span>
                                  /час
                                </span>
                              ) : null}
                              {hall.capacity > 0 ? (
                                <span>до {hall.capacity} гостей</span>
                              ) : null}
                            </div>
                            {hall.amenities && hall.amenities.length > 0 ? (
                              <div className="flex flex-wrap gap-2">
                                {hall.amenities.map((a) => (
                                  <Badge
                                    key={`${hall.id}-${a}`}
                                    variant="secondary"
                                    className="px-3 py-1 text-sm"
                                  >
                                    {a}
                                  </Badge>
                                ))}
                              </div>
                            ) : null}
                          </div>
                        </CardContent>
                      </Card>
                    )
                  })}
                </div>
              </div>
            ) : venue.amenities && venue.amenities.length > 0 ? (
              <div className="mb-8">
                <h2 className="mb-4 text-xl font-semibold text-foreground">Удобства</h2>
                <div className="flex flex-wrap gap-2">
                  {venue.amenities.map((a) => (
                    <Badge key={a} variant="secondary" className="px-3 py-1 text-sm">
                      {a}
                    </Badge>
                  ))}
                </div>
              </div>
            ) : null}

            {/* Reviews */}
            <div>
              <h2 className="mb-6 text-xl font-semibold text-foreground">
                Отзывы {totalReviews > 0 && `(${totalReviews})`}
              </h2>

              {totalReviews > 0 ? (
                <>
                  {/* Rating Summary */}
                  <Card className="mb-6 border-border">
                    <CardContent className="p-6">
                      <div className="flex flex-col gap-6 md:flex-row md:items-center">
                        <div className="text-center md:pr-8 md:border-r md:border-border">
                          <div className="text-5xl font-bold text-foreground">{venue.rating.toFixed(1)}</div>
                          <div className="mt-2 flex justify-center gap-1">
                            {[1, 2, 3, 4, 5].map((star) => (
                              <Star
                                key={star}
                                className={`h-5 w-5 ${
                                  star <= Math.round(venue.rating)
                                    ? "fill-amber-400 text-amber-400"
                                    : "text-muted"
                                }`}
                              />
                            ))}
                          </div>
                          <p className="mt-1 text-sm text-muted-foreground">{totalReviews} отзывов</p>
                        </div>
                        <div className="flex-1 space-y-2">
                          {[5, 4, 3, 2, 1].map((stars) => (
                            <div key={stars} className="flex items-center gap-3">
                              <span className="w-3 text-sm text-muted-foreground">{stars}</span>
                              <Star className="h-4 w-4 fill-amber-400 text-amber-400" />
                              <Progress
                                value={totalReviews > 0 ? (ratingBreakdown[stars] / totalReviews) * 100 : 0}
                                className="h-2 flex-1"
                              />
                              <span className="w-8 text-sm text-muted-foreground">{ratingBreakdown[stars]}</span>
                            </div>
                          ))}
                        </div>
                      </div>
                    </CardContent>
                  </Card>

                  {/* Review List */}
                  <div className="space-y-4">
                    {reviews.map((review) => (
                      <Card key={review.id} className="border-border">
                        <CardContent className="p-5">
                          <div className="mb-3 flex items-center justify-between">
                            <div className="flex items-center gap-3">
                              <Avatar>
                                <AvatarFallback className="bg-primary/10 text-primary">
                                  {review.user_name.charAt(0)}
                                </AvatarFallback>
                              </Avatar>
                              <div>
                                <p className="font-medium text-card-foreground">{review.user_name}</p>
                                <p className="text-sm text-muted-foreground">
                                  {new Date(review.created_at).toLocaleDateString("ru-RU", {
                                    day: "numeric",
                                    month: "long",
                                    year: "numeric",
                                  })}
                                </p>
                              </div>
                            </div>
                            <div className="flex items-center gap-1">
                              {[1, 2, 3, 4, 5].map((star) => (
                                <Star
                                  key={star}
                                  className={`h-4 w-4 ${
                                    star <= review.rating
                                      ? "fill-amber-400 text-amber-400"
                                      : "text-muted"
                                  }`}
                                />
                              ))}
                            </div>
                          </div>
                          <p className="text-muted-foreground">{review.text}</p>
                          {review.verified && (
                            <Badge variant="secondary" className="mt-3 gap-1">
                              <ShieldCheck className="h-3 w-3" />
                              Подтверждённый визит
                            </Badge>
                          )}
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                </>
              ) : (
                <Card className="border-border">
                  <CardContent className="py-10 text-center text-muted-foreground">
                    Пока нет отзывов. Будьте первым!
                  </CardContent>
                </Card>
              )}
            </div>
          </div>

          {/* Booking Sidebar */}
          <div className="lg:col-span-1">
            <Card className="sticky top-24 border-border">
              <CardHeader>
                <CardTitle className="text-xl text-card-foreground">Забронировать</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                {!user && (
                  <p className="text-sm text-muted-foreground">
                    <Link href="/auth/login" className="text-primary underline">Войдите</Link>, чтобы забронировать
                  </p>
                )}

                <div>
                  <label className="mb-2 block text-sm font-medium text-card-foreground">Дата</label>
                  <Popover>
                    <PopoverTrigger asChild>
                      <Button variant="outline" className="w-full justify-start gap-2 text-left font-normal">
                        <CalendarIcon className="h-4 w-4" />
                        {date ? format(date, "d MMMM yyyy", { locale: ru }) : "Выберите дату"}
                      </Button>
                    </PopoverTrigger>
                    <PopoverContent className="w-auto p-0" align="start">
                      <Calendar
                        mode="single"
                        selected={date}
                        onSelect={setDate}
                        initialFocus
                        disabled={(d) =>
                          isBefore(startOfDay(d), startOfDay(new Date()))
                        }
                      />
                    </PopoverContent>
                  </Popover>
                </div>

                {venue.services && venue.services.length > 0 && (
                  <div className="space-y-2">
                    <Label className="text-card-foreground">Услуга</Label>
                    <Select value={serviceId} onValueChange={setServiceId}>
                      <SelectTrigger>
                        <SelectValue placeholder="Как бронируете" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">Почасово (без пакета)</SelectItem>
                        {venue.services.map((s) => (
                          <SelectItem key={s.id} value={s.id}>
                            {s.name}
                            {Number(s.price) > 0
                              ? ` · ${Number(s.price).toLocaleString("ru-RU")} ₽`
                              : ""}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <p className="text-xs text-muted-foreground">
                      Пакет — фиксированная цена; почасово — по тарифу «от N ₽/час» за выбранную длительность.
                    </p>
                  </div>
                )}

                <div className="space-y-2">
                  <Label className="text-card-foreground">Время начала</Label>
                  <Select
                    value={time}
                    onValueChange={setTime}
                    disabled={!date || slotsLoading}
                  >
                    <SelectTrigger className="w-full font-normal">
                      <SelectValue
                        placeholder={
                          !date
                            ? "Сначала выберите дату"
                            : slotsLoading
                              ? "Проверяем свободные слоты…"
                              : "Выберите время"
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {!slotsLoading && date && availableSlots.length === 0 ? (
                        <SelectItem value="_none" disabled>
                          Нет свободных окон на эту дату
                        </SelectItem>
                      ) : (
                        startTimeGrid.map((t) => (
                          <SelectItem
                            key={t}
                            value={t}
                            disabled={!availableStartSet.has(t)}
                          >
                            {t}
                          </SelectItem>
                        ))
                      )}
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label className="text-card-foreground">Окончание визита</Label>
                  <Select
                    value={timeTo}
                    onValueChange={setTimeTo}
                    disabled={!time || visitEndOptions.length === 0}
                  >
                    <SelectTrigger className="w-full font-normal">
                      <SelectValue
                        placeholder={
                          !time
                            ? "Сначала выберите начало"
                            : "Выберите время окончания"
                        }
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {visitEndOptions.map((t) => (
                        <SelectItem key={t} value={t}>
                          {t}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {Boolean(time && timeTo && !slotValid) && (
                    <p className="text-xs text-destructive">
                      Проверьте интервал: длительность от 30 мин до 12 ч, шаг 30 минут.
                    </p>
                  )}
                </div>

                <div>
                  <label className="mb-2 block text-sm font-medium text-card-foreground">Гости</label>
                  <div className="flex items-center gap-3">
                    <Button variant="outline" size="icon" onClick={() => setGuests(Math.max(1, guests - 1))}>
                      <Minus className="h-4 w-4" />
                    </Button>
                    <div className="flex items-center gap-2">
                      <Users className="h-4 w-4 text-muted-foreground" />
                      <span className="w-8 text-center font-medium">{guests}</span>
                    </div>
                    <Button variant="outline" size="icon" onClick={() => setGuests(Math.min(20, guests + 1))}>
                      <Plus className="h-4 w-4" />
                    </Button>
                  </div>
                </div>

                <div className="border-t border-border pt-4">
                  {priceHint && (
                    <div className="mb-4 flex items-center justify-between gap-2">
                      <span className="text-muted-foreground">К оплате</span>
                      <span className="text-2xl font-bold text-foreground">{priceHint}</span>
                    </div>
                  )}
                  {!priceHint && (
                    <p className="mb-4 text-sm text-muted-foreground">
                      Стоимость появится после выбора услуги или длительности.
                    </p>
                  )}
                  <Button
                    className="w-full"
                    size="lg"
                    disabled={!user || !date || !time || !slotValid || booking}
                    onClick={handleBook}
                  >
                    {booking ? "Переход к оплате…" : "Забронировать"}
                  </Button>
                  {bookingMsg && (
                    <p className={`mt-2 text-sm text-center ${bookingMsg.includes("создано") ? "text-green-600" : "text-destructive"}`}>
                      {bookingMsg}
                    </p>
                  )}
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </div>
    </section>
  )
}

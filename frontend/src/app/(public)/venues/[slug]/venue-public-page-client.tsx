"use client"

import { useEffect, useState } from "react"
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
  Check,
} from "lucide-react"
import Link from "next/link"
import { FramedImg } from "@/components/banya/framed-image"
import {
  getVenueBySlug,
  getVenueReviews,
  venueMediaUrl,
} from "@/lib/api"
import type { Venue, Review, VenueHallPhoto } from "@/lib/types"
import {
  VENUE_SOCIAL_PUBLIC_LABELS,
  VENUE_TYPE_LABELS,
  venueServiceDurationMinutes,
  type VenueSocialLinkKey,
} from "@/lib/types"
import { useAuthStore } from "@/store/auth"
import { cn } from "@/lib/utils"
import { ReviewList } from "@/components/review-list"
import { useGallery } from "@/hooks/use-gallery"
import { useSlotAvailability } from "@/hooks/use-slot-availability"
import { useVenueBookingForm } from "@/hooks/use-venue-booking-form"

function socialHref(url: string): string {
  const u = url.trim()
  if (!u) return "#"
  if (/^https?:\/\//i.test(u)) return u
  return `https://${u}`
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

/**
 * Адаптивная раскладка фото зала: одно — широким баннером, дальше — мозаика
 * «герой + сетка». Ряды всегда заполнены целиком (никакого пустого угла, как
 * в прежней фикс-сетке col-4), лишние фото скрыты под «+N». Каждое фото видно
 * целиком, без обрезки (FramedImg, как в основной галерее).
 */
function HallPhotos({ photos }: { photos: VenueHallPhoto[] }) {
  const sorted = [...photos].sort((a, b) => {
    if (Boolean(a.is_cover) !== Boolean(b.is_cover)) return a.is_cover ? -1 : 1
    return (a.sort_order ?? 0) - (b.sort_order ?? 0)
  })
  const n = sorted.length
  if (n === 0) return null

  const tile = "relative overflow-hidden bg-muted"

  // Одно фото — баннер во всю ширину карточки.
  if (n === 1) {
    return (
      <div className={cn(tile, "aspect-[16/9]")}>
        <FramedImg src={venueMediaUrl(sorted[0].url)} alt="" />
      </div>
    )
  }

  // Два или четыре — ровная сетка без героя (ряды заполнены полностью).
  if (n === 2 || n === 4) {
    return (
      <div className={cn("grid gap-1", n === 2 ? "grid-cols-2" : "grid-cols-2 grid-rows-2")}>
        {sorted.map((p) => (
          <div key={p.id} className={cn(tile, "aspect-[4/3]")}>
            <FramedImg src={venueMediaUrl(p.url)} alt="" />
          </div>
        ))}
      </div>
    )
  }

  // Три или пять+ — «герой + сетка». Герой 2×2, остаток ровно заполняет ряды.
  const wide = n >= 5
  const thumbs = sorted.slice(1, wide ? 5 : 3)
  const hidden = n - 1 - thumbs.length
  return (
    <div
      className={cn(
        "grid aspect-[16/9] grid-rows-2 gap-1",
        wide ? "grid-cols-4" : "grid-cols-3",
      )}
    >
      <div className={cn(tile, "col-span-2 row-span-2")}>
        <FramedImg src={venueMediaUrl(sorted[0].url)} alt="" />
      </div>
      {thumbs.map((p, i) => (
        <div key={p.id} className={tile}>
          <FramedImg src={venueMediaUrl(p.url)} alt="" />
          {i === thumbs.length - 1 && hidden > 0 ? (
            <div className="absolute inset-0 flex items-center justify-center bg-black/55 text-lg font-semibold text-white">
              +{hidden}
            </div>
          ) : null}
        </div>
      ))}
    </div>
  )
}

export function VenuePublicPageClient({
  slug,
  initialVenue,
}: {
  slug: string
  initialVenue?: Venue | null
}) {
  const user = useAuthStore((s) => s.user)

  // ─── Данные заведения и отзывов ─────────────────────────────────────────
  const [venue, setVenue] = useState<Venue | null>(initialVenue ?? null)
  const [reviews, setReviews] = useState<Review[]>([])
  const [loading, setLoading] = useState(!initialVenue)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!slug) return
    let active = true
    const bootstrapVenue = initialVenue != null && initialVenue.slug === slug ? initialVenue : null

    if (!bootstrapVenue) {
      setLoading(true)
    } else {
      setVenue(bootstrapVenue)
      setLoading(false)
    }
    setError("")

    const load = async () => {
      let currentVenue = bootstrapVenue

      if (!currentVenue) {
        try {
          currentVenue = await getVenueBySlug(slug)
          if (!active) return
          setVenue(currentVenue)
        } catch {
          if (!active) return
          setError("Заведение не найдено")
          setLoading(false)
          return
        }
      } else {
        try {
          const freshVenue = await getVenueBySlug(slug)
          if (!active) return
          currentVenue = freshVenue
          setVenue(freshVenue)
        } catch {
          // keep SSR venue and continue with its reviews
        }
      }

      try {
        const r = await getVenueReviews(currentVenue.id)
        if (!active) return
        setReviews(r)
      } catch {
        if (!active) return
        setReviews([])
      } finally {
        if (active) setLoading(false)
      }
    }

    void load()
    return () => { active = false }
  }, [slug, initialVenue])

  const reloadReviews = async () => {
    if (!venue?.id) return
    try {
      const r = await getVenueReviews(venue.id)
      setReviews(r)
    } catch {
      // keep current state on refresh errors
    }
  }

  // ─── Форма бронирования + слоты ─────────────────────────────────────────
  // useVenueBookingForm вызывает useSlotAvailability внутри себя —
  // это единственный правильный способ избежать circular dependency между
  // slotDurationMin (из формы) и availableSlots (нужны форме).
  const {
    date, setDate,
    time, setTime,
    timeTo, setTimeTo,
    slotDurationMin,
    slotValid,
    visitEndOptions,
    startTimeGrid,
    availableStartSet,
    availableSlots,
    slotsLoading,
    selectedServiceIds, setSelectedServiceIds,
    selectedHallIds, setSelectedHallIds,
    guests, setGuests,
    booking,
    bookingMsg, setBookingMsg,
    priceHint,
    handleBook,
  } = useVenueBookingForm({ venue, slug })

  // ─── Галерея ────────────────────────────────────────────────────────────
  const carouselSlides = venue ? venueCarouselSlides(venue) : []
  const gallery = useGallery(carouselSlides, `${slug}-${venue?.id ?? ""}`)

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
              {gallery.count > 0 && gallery.current ? (
                <div className="space-y-3">
                  <div
                    className="relative outline-none"
                    role="region"
                    aria-label={`Фотографии: ${venue.name}`}
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (gallery.count < 2) return
                      if (e.key === "ArrowLeft") { e.preventDefault(); gallery.prev() }
                      else if (e.key === "ArrowRight") { e.preventDefault(); gallery.next() }
                    }}
                  >
                    <div className="relative aspect-[16/9] overflow-hidden rounded-xl bg-muted">
                      <FramedImg
                        src={gallery.current.src}
                        alt={`${venue.name} — фото ${gallery.index + 1} из ${gallery.count}`}
                      />
                    </div>
                    {gallery.count > 1 ? (
                      <>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="absolute left-2 top-1/2 z-10 size-9 -translate-y-1/2 border-border/80 bg-background/90 shadow-sm hover:bg-background"
                          aria-label="Предыдущее фото"
                          onClick={gallery.prev}
                        >
                          <ChevronLeft className="size-5" />
                        </Button>
                        <Button
                          type="button"
                          variant="outline"
                          size="icon"
                          className="absolute right-2 top-1/2 z-10 size-9 -translate-y-1/2 border-border/80 bg-background/90 shadow-sm hover:bg-background"
                          aria-label="Следующее фото"
                          onClick={gallery.next}
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
                        aria-selected={i === gallery.index}
                        aria-label={`Фото ${i + 1}`}
                        onClick={() => gallery.openAt(i)}
                        className={cn(
                          "relative h-16 w-16 shrink-0 overflow-hidden rounded-md bg-muted transition-[opacity,box-shadow]",
                          i === gallery.index
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
                <h2 className="mb-2 text-xl font-semibold text-foreground">Услуги и цены</h2>
                <p className="mb-4 text-sm text-muted-foreground">
                  Нажмите на карточку, чтобы добавить услугу в бронь или убрать её. Можно выбрать несколько пакетов
                  одновременно; время визита и стоимость справа подстроятся под выбор.
                </p>
                <div className="space-y-3">
                  {venue.services.map((svc) => {
                    const selected = selectedServiceIds.includes(svc.id)
                    return (
                      <Card
                        key={svc.id}
                        role="button"
                        tabIndex={0}
                        aria-pressed={selected}
                        onClick={() =>
                          setSelectedServiceIds((prev) =>
                            prev.includes(svc.id) ? prev.filter((x) => x !== svc.id) : [...prev, svc.id],
                          )
                        }
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault()
                            setSelectedServiceIds((prev) =>
                              prev.includes(svc.id) ? prev.filter((x) => x !== svc.id) : [...prev, svc.id],
                            )
                          }
                        }}
                        className={cn(
                          "cursor-pointer border-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                          selected ? "border-primary bg-primary/5" : "border-border hover:border-primary/40",
                        )}
                      >
                        <CardContent className="relative flex items-center justify-between gap-4 p-4 pr-14">
                          {selected ? (
                            <div
                              className="absolute right-3 top-1/2 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full bg-primary text-primary-foreground"
                              aria-hidden
                            >
                              <Check className="h-4 w-4" />
                            </div>
                          ) : null}
                          <div className="min-w-0">
                            <p className="font-medium text-card-foreground">{svc.name}</p>
                            {venueServiceDurationMinutes(svc) > 0 && (
                              <p className="text-sm text-muted-foreground">
                                {venueServiceDurationMinutes(svc)} мин
                              </p>
                            )}
                          </div>
                          {svc.price > 0 ? (
                            <span className="shrink-0 font-semibold text-primary">
                              {svc.price.toLocaleString("ru-RU")} ₽
                            </span>
                          ) : (
                            <span className="shrink-0 text-sm text-muted-foreground">Почасово</span>
                          )}
                        </CardContent>
                      </Card>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Залы */}
            {venue.halls && venue.halls.length > 0 ? (
              <div className="mb-8">
                <h2 className="mb-2 text-xl font-semibold text-foreground">Залы</h2>
                <p className="mb-4 text-sm text-muted-foreground">
                  Нажмите на карточку зала, чтобы добавить его в бронь или убрать. Можно выбрать несколько залов;
                  почасовая часть стоимости справа считается по максимальной ставке среди заведения и выбранных залов.
                </p>
                <div className="space-y-6">
                  {venue.halls.map((hall) => {
                    const hallSelected = selectedHallIds.includes(hall.id)
                    return (
                      <Card
                        key={hall.id}
                        role="button"
                        tabIndex={0}
                        aria-pressed={hallSelected}
                        onClick={() =>
                          setSelectedHallIds((prev) =>
                            prev.includes(hall.id)
                              ? prev.filter((x) => x !== hall.id)
                              : [...prev, hall.id],
                          )
                        }
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault()
                            setSelectedHallIds((prev) =>
                              prev.includes(hall.id)
                                ? prev.filter((x) => x !== hall.id)
                                : [...prev, hall.id],
                            )
                          }
                        }}
                        className={cn(
                          "cursor-pointer overflow-hidden border-2 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                          hallSelected
                            ? "border-primary bg-primary/5"
                            : "border-border hover:border-primary/40",
                        )}
                      >
                        <CardContent className="relative p-0">
                          {hallSelected ? (
                            <div
                              className="absolute right-3 top-3 z-10 flex h-8 w-8 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-sm"
                              aria-hidden
                            >
                              <Check className="h-4 w-4" />
                            </div>
                          ) : null}
                          <HallPhotos photos={hall.photos ?? []} />
                          <div className="space-y-3 p-4 pr-14 sm:p-5 sm:pr-16">
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
              <ReviewList targetId={venue.id} targetType="venue" reviews={reviews} onReviewAdded={reloadReviews} />
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

                {venue.halls && venue.halls.length > 0 && (
                  <div className="space-y-2">
                    <Label className="text-card-foreground">Залы в брони</Label>
                    {selectedHallIds.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        Не выбрано — почасовая ставка по цене заведения. Выберите залы на карточках слева.
                      </p>
                    ) : (
                      <ul className="max-h-40 space-y-1.5 overflow-y-auto rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
                        {selectedHallIds.map((id) => {
                          const h = venue.halls?.find((x) => x.id === id)
                          if (!h) return null
                          return (
                            <li key={id} className="flex justify-between gap-2">
                              <span className="line-clamp-2 text-card-foreground">{h.name}</span>
                              {h.price_from > 0 ? (
                                <span className="shrink-0 text-muted-foreground">
                                  от {h.price_from.toLocaleString("ru-RU")} ₽/ч
                                </span>
                              ) : (
                                <span className="shrink-0 text-muted-foreground">—</span>
                              )}
                            </li>
                          )
                        })}
                      </ul>
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="w-full"
                      disabled={selectedHallIds.length === 0}
                      onClick={() => setSelectedHallIds([])}
                    >
                      Снять выбор залов
                    </Button>
                  </div>
                )}

                {venue.services && venue.services.length > 0 && (
                  <div className="space-y-2">
                    <Label className="text-card-foreground">Услуги в брони</Label>
                    {selectedServiceIds.length === 0 ? (
                      <p className="text-sm text-muted-foreground">
                        Почасовой тариф. Выберите пакеты на карточках слева — они появятся здесь.
                      </p>
                    ) : (
                      <ul className="max-h-48 space-y-1.5 overflow-y-auto rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
                        {selectedServiceIds.map((id) => {
                          const s = venue.services?.find((x) => x.id === id)
                          if (!s) return null
                          return (
                            <li key={id} className="flex justify-between gap-2">
                              <span className="line-clamp-2 text-card-foreground">{s.name}</span>
                              {Number(s.price) > 0 ? (
                                <span className="shrink-0 font-medium text-primary">
                                  {Number(s.price).toLocaleString("ru-RU")} ₽
                                </span>
                              ) : (
                                <span className="shrink-0 text-muted-foreground">почасово</span>
                              )}
                            </li>
                          )
                        })}
                      </ul>
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="w-full"
                      disabled={selectedServiceIds.length === 0}
                      onClick={() => setSelectedServiceIds([])}
                    >
                      Только почасово
                    </Button>
                    <p className="text-xs text-muted-foreground">
                      Фиксированные пакеты суммируются; не более одной услуги без цены (почасовой тариф) в одной брони.
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

"use client"

import { useEffect, useRef, useState } from "react"
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
  Clock,
  DoorOpen,
  Maximize2,
  X,
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
  formatWorkingHours,
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
  const [activeTab, setActiveTab] = useState("services")
  // Пока идёт программный скролл по клику на вкладку — гасим scroll-spy,
  // чтобы промежуточные секции не перебивали выбранную.
  const suppressSpy = useRef(false)

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

  // Подсветка активного раздела в липких вкладках (scroll-spy).
  useEffect(() => {
    if (!venue) return
    const ids = ["services", "halls", "reviews", "about"]
    const obs = new IntersectionObserver(
      (entries) => {
        if (suppressSpy.current) return
        for (const e of entries) {
          if (e.isIntersecting) setActiveTab(e.target.id)
        }
      },
      { rootMargin: "-45% 0px -50% 0px" },
    )
    for (const id of ids) {
      const el = document.getElementById(id)
      if (el) obs.observe(el)
    }
    return () => obs.disconnect()
  }, [venue])

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

  const verified = venue.status === "active"
  const venueHours = formatWorkingHours(venue.working_hours)
  const hallsCount = venue.halls?.length ?? 0
  const capacity = venue.capacity ?? 0
  const socialEntries =
    venue.social_links && typeof venue.social_links === "object" && !Array.isArray(venue.social_links)
      ? (Object.entries(venue.social_links) as [string, unknown][]).filter(
          ([, u]) => typeof u === "string" && u.trim().length > 0,
        )
      : []
  const hasAbout =
    Boolean(venue.description) || Boolean(venueHours) || capacity > 0 || hallsCount > 0 || Boolean(venue.phone) || socialEntries.length > 0

  const tabs = [
    { id: "services", label: "Услуги", show: (venue.services?.length ?? 0) > 0 },
    { id: "halls", label: "Залы", show: hallsCount > 0 },
    { id: "reviews", label: "Отзывы", show: true },
    { id: "about", label: "О месте", show: hasAbout },
  ].filter((t) => t.show)
  const currentTab = tabs.some((t) => t.id === activeTab) ? activeTab : tabs[0]?.id

  const scrollToSection = (id: string) => {
    suppressSpy.current = true
    setActiveTab(id)
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth", block: "start" })
    window.setTimeout(() => {
      suppressSpy.current = false
    }, 700)
  }

  return (
    <section className="bg-background py-10 md:py-16">
      <div className="container mx-auto px-4">
        <Link href="/venues" className="mb-6 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="h-4 w-4" />
          Назад к каталогу
        </Link>

        {/* Галерея — мозаика во всю ширину (клик открывает лайтбокс) */}
        {carouselSlides.length > 0 && gallery.current ? (
          <div className="mb-8">
            {/* мобайл: одно фото-баннер */}
            <button
              type="button"
              onClick={() => gallery.openAt(0)}
              aria-label="Открыть фотографии"
              className="group relative block aspect-[4/3] w-full cursor-zoom-in overflow-hidden rounded-2xl bg-muted md:hidden"
            >
              <FramedImg src={carouselSlides[0].src} alt={venue.name} />
              {carouselSlides.length > 1 ? (
                <span className="absolute bottom-3 right-3 inline-flex items-center gap-1.5 rounded-full bg-black/60 px-3 py-1.5 text-xs font-medium text-white">
                  <ImageIcon className="h-3.5 w-3.5" />
                  {carouselSlides.length} фото
                </span>
              ) : null}
            </button>

            {/* десктоп: герой + до 4 миниатюр */}
            <div className="hidden h-[400px] grid-cols-4 grid-rows-2 gap-1.5 overflow-hidden rounded-2xl md:grid">
              <button
                type="button"
                onClick={() => gallery.openAt(0)}
                aria-label="Открыть фото 1"
                className="group relative col-span-2 row-span-2 cursor-zoom-in overflow-hidden bg-muted"
              >
                <FramedImg src={carouselSlides[0].src} alt={venue.name} />
                <span className="pointer-events-none absolute left-3 top-3 z-10 inline-flex items-center gap-1 rounded-md bg-black/55 px-2 py-1 text-xs text-white opacity-0 transition-opacity group-hover:opacity-100">
                  <Maximize2 className="h-3.5 w-3.5" />
                  Развернуть
                </span>
              </button>
              {carouselSlides.slice(1, 5).map((slide, i) => {
                const idx = i + 1
                const extra = carouselSlides.length - 5
                return (
                  <button
                    key={slide.id}
                    type="button"
                    onClick={() => gallery.openAt(idx)}
                    aria-label={`Открыть фото ${idx + 1}`}
                    className="group relative cursor-zoom-in overflow-hidden bg-muted"
                  >
                    <FramedImg src={slide.src} alt="" />
                    {i === 3 && extra > 0 ? (
                      <div className="absolute inset-0 flex items-center justify-center bg-black/55 text-xl font-semibold text-white">
                        +{extra}
                      </div>
                    ) : null}
                  </button>
                )
              })}
            </div>
          </div>
        ) : (
          <div className="mb-8 flex aspect-[16/7] items-center justify-center rounded-2xl bg-muted">
            <div className="flex flex-col items-center gap-2 text-muted-foreground/40">
              <ImageIcon className="h-16 w-16" />
              <span className="text-sm">Фото ещё не добавлены</span>
            </div>
          </div>
        )}

        {/* Видео */}
        {venue.videos?.length ? (
          <div className="mb-8">
            <h2 className="mb-3 text-lg font-semibold">Видео</h2>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {[...venue.videos]
                .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
                .map((v) => (
                  <video
                    key={v.id}
                    src={venueMediaUrl(v.url)}
                    controls
                    preload="metadata"
                    className="aspect-video w-full rounded-2xl bg-black object-contain"
                  />
                ))}
            </div>
          </div>
        ) : null}

        {/* Липкие вкладки-разделы */}
        <div className="sticky top-16 z-30 -mx-4 mb-8 flex gap-1.5 overflow-x-auto border-b border-border bg-background/95 px-4 py-3 backdrop-blur [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {tabs.map((t) => (
            <button
              key={t.id}
              type="button"
              onClick={() => scrollToSection(t.id)}
              className={cn(
                "shrink-0 rounded-full px-4 py-1.5 text-sm transition-colors",
                currentTab === t.id
                  ? "border border-border bg-card font-medium text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t.label}
            </button>
          ))}
        </div>

        <div className="grid gap-8 lg:grid-cols-3">
          {/* Main Content */}
          <div className="lg:col-span-2">
            {/* Venue Header */}
            <div className="mb-8">
              <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                {venue.rating > 0 && (
                  <span className="inline-flex items-center gap-1.5 text-foreground">
                    <Star className="h-4 w-4 fill-amber-400 text-amber-400" />
                    <span className="font-medium">{venue.rating.toFixed(1)}</span>
                    <span className="text-muted-foreground">({venue.review_count} отзывов)</span>
                  </span>
                )}
                {verified && (
                  <>
                    {venue.rating > 0 && <span className="text-border" aria-hidden>·</span>}
                    <span className="inline-flex items-center gap-1.5 text-emerald-700">
                      <ShieldCheck className="h-4 w-4" />
                      Проверено
                    </span>
                  </>
                )}
              </div>
              <h1 className="mb-2 text-3xl font-bold text-foreground md:text-4xl">{venue.name}</h1>
              <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-sm text-muted-foreground">
                <MapPin className="h-4 w-4 shrink-0" />
                <span>{venue.city}{venue.address ? `, ${venue.address}` : ""}</span>
                <span aria-hidden>·</span>
                <span>{VENUE_TYPE_LABELS[venue.type] ?? venue.type}</span>
              </div>
            </div>

            {/* Services */}
            {venue.services && venue.services.length > 0 && (
              <div id="services" className="mb-10 scroll-mt-32">
                <h2 className="mb-1 text-xl font-semibold text-foreground">Услуги и цены</h2>
                <p className="mb-3 text-sm text-muted-foreground">
                  Выберите пакеты — время визита и стоимость справа подстроятся. Можно выбрать несколько.
                </p>
                <div className="divide-y divide-border border-y border-border">
                  {venue.services.map((svc) => {
                    const selected = selectedServiceIds.includes(svc.id)
                    const toggle = () =>
                      setSelectedServiceIds((prev) =>
                        prev.includes(svc.id) ? prev.filter((x) => x !== svc.id) : [...prev, svc.id],
                      )
                    return (
                      <div
                        key={svc.id}
                        role="button"
                        tabIndex={0}
                        aria-pressed={selected}
                        onClick={toggle}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault()
                            toggle()
                          }
                        }}
                        className={cn(
                          "flex cursor-pointer items-center gap-4 py-4 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                          selected && "bg-primary/5",
                        )}
                      >
                        <div className="min-w-0 flex-1">
                          <p className="font-medium text-card-foreground">{svc.name}</p>
                          {venueServiceDurationMinutes(svc) > 0 && (
                            <p className="mt-0.5 text-sm text-muted-foreground">
                              {venueServiceDurationMinutes(svc)} мин
                            </p>
                          )}
                          <p className="mt-1.5 text-sm font-medium text-foreground">
                            {svc.price > 0 ? `${svc.price.toLocaleString("ru-RU")} ₽` : "Почасово"}
                          </p>
                        </div>
                        <span
                          className={cn(
                            "inline-flex shrink-0 items-center gap-1.5 rounded-full px-4 py-2 text-sm font-medium transition-colors",
                            selected
                              ? "bg-primary text-primary-foreground"
                              : "border border-input bg-background text-foreground",
                          )}
                        >
                          {selected && <Check className="h-4 w-4" />}
                          {selected ? "Выбрано" : "Выбрать"}
                        </span>
                      </div>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Залы */}
            {venue.halls && venue.halls.length > 0 ? (
              <div id="halls" className="mb-10 scroll-mt-32">
                <h2 className="mb-1 text-xl font-semibold text-foreground">Залы</h2>
                <p className="mb-4 text-sm text-muted-foreground">
                  Выберите залы для брони. Почасовая ставка справа считается по максимальной среди заведения и выбранных залов.
                </p>
                <div className="space-y-4">
                  {venue.halls.map((hall) => {
                    const hallSelected = selectedHallIds.includes(hall.id)
                    const toggleHall = () =>
                      setSelectedHallIds((prev) =>
                        prev.includes(hall.id) ? prev.filter((x) => x !== hall.id) : [...prev, hall.id],
                      )
                    return (
                      <div
                        key={hall.id}
                        role="button"
                        tabIndex={0}
                        aria-pressed={hallSelected}
                        onClick={toggleHall}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault()
                            toggleHall()
                          }
                        }}
                        className={cn(
                          "cursor-pointer overflow-hidden rounded-2xl border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                          hallSelected ? "border-primary bg-primary/5" : "border-border hover:border-primary/40",
                        )}
                      >
                        <HallPhotos photos={hall.photos ?? []} />
                        <div className="flex items-center gap-4 p-4 sm:p-5">
                          <div className="min-w-0 flex-1 space-y-2">
                            <h3 className="text-lg font-semibold text-foreground">{hall.name}</h3>
                            <div className="flex flex-wrap gap-x-3 gap-y-1 text-sm text-muted-foreground">
                              {hall.price_from > 0 ? (
                                <span>
                                  от{" "}
                                  <span className="font-semibold text-foreground">
                                    {hall.price_from.toLocaleString("ru-RU")} ₽
                                  </span>
                                  /час
                                </span>
                              ) : null}
                              {hall.capacity > 0 ? <span>до {hall.capacity} гостей</span> : null}
                            </div>
                            {hall.amenities && hall.amenities.length > 0 ? (
                              <div className="flex flex-wrap gap-2 pt-1">
                                {hall.amenities.map((a) => (
                                  <Badge key={`${hall.id}-${a}`} variant="secondary" className="px-3 py-1 text-sm">
                                    {a}
                                  </Badge>
                                ))}
                              </div>
                            ) : null}
                          </div>
                          <span
                            className={cn(
                              "inline-flex shrink-0 items-center gap-1.5 self-start rounded-full px-4 py-2 text-sm font-medium transition-colors",
                              hallSelected
                                ? "bg-primary text-primary-foreground"
                                : "border border-input bg-background text-foreground",
                            )}
                          >
                            {hallSelected && <Check className="h-4 w-4" />}
                            {hallSelected ? "Выбрано" : "Выбрать"}
                          </span>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>
            ) : venue.amenities && venue.amenities.length > 0 ? (
              <div className="mb-10">
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
            <div id="reviews" className="mb-10 scroll-mt-32">
              <h2 className="mb-6 text-xl font-semibold text-foreground">
                Отзывы {totalReviews > 0 && `(${totalReviews})`}
              </h2>
              <ReviewList targetId={venue.id} targetType="venue" reviews={reviews} onReviewAdded={reloadReviews} />
            </div>

            {/* О месте */}
            {hasAbout && (
              <div id="about" className="scroll-mt-32">
                <h2 className="mb-4 text-xl font-semibold text-foreground">О месте</h2>
                {venue.description && (
                  <p className="mb-5 leading-relaxed text-muted-foreground">{venue.description}</p>
                )}
                {(venueHours || capacity > 0 || hallsCount > 0) && (
                  <div className="mb-5 flex flex-wrap gap-3">
                    {venueHours && (
                      <div className="flex min-w-[8.5rem] items-center gap-2.5 rounded-xl border border-border bg-card px-3.5 py-2.5">
                        <Clock className="h-5 w-5 shrink-0 text-primary" />
                        <div className="leading-tight">
                          <div className="text-xs text-muted-foreground">Часы работы</div>
                          <div className="text-sm font-medium text-foreground">{venueHours}</div>
                        </div>
                      </div>
                    )}
                    {capacity > 0 && (
                      <div className="flex min-w-[8.5rem] items-center gap-2.5 rounded-xl border border-border bg-card px-3.5 py-2.5">
                        <Users className="h-5 w-5 shrink-0 text-primary" />
                        <div className="leading-tight">
                          <div className="text-xs text-muted-foreground">Вместимость</div>
                          <div className="text-sm font-medium text-foreground">до {capacity} чел.</div>
                        </div>
                      </div>
                    )}
                    {hallsCount > 0 && (
                      <div className="flex min-w-[8.5rem] items-center gap-2.5 rounded-xl border border-border bg-card px-3.5 py-2.5">
                        <DoorOpen className="h-5 w-5 shrink-0 text-primary" />
                        <div className="leading-tight">
                          <div className="text-xs text-muted-foreground">Залов</div>
                          <div className="text-sm font-medium text-foreground">{hallsCount}</div>
                        </div>
                      </div>
                    )}
                  </div>
                )}
                {(venue.phone || socialEntries.length > 0) && (
                  <div className="flex flex-col gap-2 text-sm text-muted-foreground">
                    {venue.phone && (
                      <div className="flex items-center gap-2">
                        <Phone className="h-4 w-4 shrink-0" />
                        {venue.phone}
                      </div>
                    )}
                    {socialEntries.length > 0 && (
                      <div className="flex flex-wrap gap-x-4 gap-y-2">
                        {socialEntries.map(([key, u]) => {
                          const url = String(u).trim()
                          const label = VENUE_SOCIAL_PUBLIC_LABELS[key as VenueSocialLinkKey] ?? key
                          return (
                            <a
                              key={key}
                              href={socialHref(url)}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1.5 font-medium text-primary hover:underline"
                            >
                              <ExternalLink className="h-4 w-4 shrink-0" />
                              {label}
                            </a>
                          )
                        })}
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Booking Sidebar */}
          <div className="lg:col-span-1">
            <Card className="sticky top-24 rounded-2xl border-border">
              <CardHeader>
                <div className="flex items-baseline justify-between gap-2">
                  <CardTitle className="text-xl text-card-foreground">Забронировать</CardTitle>
                  {venue.price_from > 0 && (
                    <span className="shrink-0 text-sm text-muted-foreground">
                      от{" "}
                      <span className="font-semibold text-primary">
                        {venue.price_from.toLocaleString("ru-RU")} ₽
                      </span>
                      /час
                    </span>
                  )}
                </div>
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
                    className="w-full rounded-full"
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

      {gallery.lightboxOpen && gallery.current && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/90 p-4"
          role="dialog"
          aria-modal="true"
          aria-label={`Фото: ${venue.name}`}
          onClick={gallery.closeLightbox}
        >
          <button
            type="button"
            aria-label="Закрыть"
            onClick={gallery.closeLightbox}
            className="absolute right-3 top-3 z-10 inline-flex size-10 items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/20"
          >
            <X className="size-5" />
          </button>

          {gallery.count > 1 && (
            <button
              type="button"
              aria-label="Предыдущее фото"
              onClick={(e) => { e.stopPropagation(); gallery.prev() }}
              className="absolute left-3 top-1/2 z-10 inline-flex size-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/20"
            >
              <ChevronLeft className="size-6" />
            </button>
          )}

          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={gallery.current.src}
            alt={`${venue.name} — фото ${gallery.index + 1} из ${gallery.count}`}
            onClick={(e) => e.stopPropagation()}
            className="max-h-[90vh] max-w-[92vw] object-contain"
          />

          {gallery.count > 1 && (
            <button
              type="button"
              aria-label="Следующее фото"
              onClick={(e) => { e.stopPropagation(); gallery.next() }}
              className="absolute right-3 top-1/2 z-10 inline-flex size-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/20"
            >
              <ChevronRight className="size-6" />
            </button>
          )}

          {gallery.count > 1 && (
            <div className="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-full bg-black/50 px-3 py-1 text-sm text-white">
              {gallery.index + 1} / {gallery.count}
            </div>
          )}
        </div>
      )}
    </section>
  )
}

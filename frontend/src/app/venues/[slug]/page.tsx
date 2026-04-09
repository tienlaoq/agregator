"use client"

import { useEffect, useState } from "react"
import { useParams } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
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
import { format } from "date-fns"
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
} from "lucide-react"
import Link from "next/link"
import { getVenueBySlug, getVenueReviews, createBooking } from "@/lib/api"
import type { Venue, Review } from "@/lib/types"
import { VENUE_TYPE_LABELS } from "@/lib/types"
import { useAuthStore } from "@/store/auth"

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
  const [guests, setGuests] = useState(2)
  const [booking, setBooking] = useState(false)
  const [bookingMsg, setBookingMsg] = useState("")

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

  const handleBook = async () => {
    if (!venue || !date || !time) return
    setBooking(true)
    setBookingMsg("")
    try {
      await createBooking({
        venue_id: venue.id,
        date: format(date, "yyyy-MM-dd"),
        time_from: time,
        guests,
      })
      setBookingMsg("Бронирование создано!")
    } catch {
      setBookingMsg("Не удалось забронировать. Попробуйте позже.")
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
            {/* Photo area */}
            <div className="mb-8">
              {venue.image_url ? (
                <div className="relative aspect-[16/9] overflow-hidden rounded-xl">
                  <img src={venue.image_url} alt={venue.name} className="h-full w-full object-cover" />
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
                          {svc.duration_minutes > 0 && (
                            <p className="text-sm text-muted-foreground">{svc.duration_minutes} мин</p>
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

            {/* Amenities */}
            {venue.amenities && venue.amenities.length > 0 && (
              <div className="mb-8">
                <h2 className="mb-4 text-xl font-semibold text-foreground">Удобства</h2>
                <div className="flex flex-wrap gap-2">
                  {venue.amenities.map((a) => (
                    <Badge key={a} variant="secondary" className="text-sm px-3 py-1">{a}</Badge>
                  ))}
                </div>
              </div>
            )}

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
                      <Calendar mode="single" selected={date} onSelect={setDate} initialFocus />
                    </PopoverContent>
                  </Popover>
                </div>

                <div>
                  <label className="mb-2 block text-sm font-medium text-card-foreground">Время</label>
                  <Select value={time} onValueChange={setTime}>
                    <SelectTrigger>
                      <SelectValue placeholder="Выберите время" />
                    </SelectTrigger>
                    <SelectContent>
                      {["10:00", "11:00", "12:00", "13:00", "14:00", "15:00", "16:00", "17:00", "18:00", "19:00", "20:00", "21:00", "22:00"].map((t) => (
                        <SelectItem key={t} value={t}>{t}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
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
                  {venue.price_from > 0 && (
                    <div className="mb-4 flex items-center justify-between">
                      <span className="text-muted-foreground">Ориентировочно</span>
                      <span className="text-2xl font-bold text-foreground">
                        {(venue.price_from * 2).toLocaleString("ru-RU")} ₽
                      </span>
                    </div>
                  )}
                  <Button
                    className="w-full"
                    size="lg"
                    disabled={!user || !date || !time || booking}
                    onClick={handleBook}
                  >
                    {booking ? "Бронирование..." : "Забронировать"}
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

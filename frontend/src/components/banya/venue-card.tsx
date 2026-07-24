import Link from "next/link"
import { Card, CardContent } from "@/components/ui/card"
import {
  MapPin,
  Star,
  ImageIcon,
  Images,
  Clock,
  Users,
  ShieldCheck,
  ArrowRight,
} from "lucide-react"
import { FramedImage } from "@/components/banya/framed-image"
import { venueCardImageSrc } from "@/lib/api"
import type { Venue } from "@/lib/types"
import { VENUE_TYPE_LABELS, formatWorkingHours } from "@/lib/types"

interface VenueCardProps {
  venue: Venue
  /**
   * Подсказка для next/image sizes — передаётся снаружи в зависимости от колонок сетки.
   * По умолчанию: мобайл 100vw, планшет 50vw, десктоп 33vw.
   */
  sizes?: string
}

/**
 * Переиспользуемая карточка заведения.
 * Используется в CatalogSection, PopularVenuesSection и любых будущих листингах.
 *
 * Отвечает на 4 вопроса за пару секунд: что это, где, сколько, можно ли доверять.
 * Всё, что требует чтения (услуги, залы, соцсети), — на детальной странице.
 */
export function VenueCard({ venue, sizes = "(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw" }: VenueCardProps) {
  const cardImg = venueCardImageSrc(venue)
  const photoCount = venue.photos?.length ?? 0
  const verified = venue.status === "active"
  const amenities = venue.amenities ?? []
  const workingHours = formatWorkingHours(venue.working_hours)

  return (
    <Link href={`/venues/${venue.slug}`} className="group block h-full min-w-0">
      <Card className="flex h-full flex-col gap-0 overflow-hidden border-border bg-card py-0 transition-all hover:shadow-xl">
        <div className="relative aspect-[4/3] overflow-hidden bg-muted flex items-center justify-center">
          {cardImg ? (
            <FramedImage
              src={cardImg}
              alt={venue.name}
              sizes={sizes}
              className="transition-transform duration-300 group-hover:scale-105"
            />
          ) : (
            <div className="flex flex-col items-center gap-1 text-muted-foreground/40">
              <ImageIcon className="h-8 w-8" />
              <span className="text-xs">Нет фото</span>
            </div>
          )}

          <span className="absolute left-3 top-3 inline-flex items-center gap-1.5 rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary ring-1 ring-inset ring-primary/20 backdrop-blur-sm">
            {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
          </span>

          {verified && (
            <span className="absolute right-3 top-3 inline-flex items-center gap-1.5 rounded-full bg-emerald-50/90 px-2.5 py-1 text-xs font-medium text-emerald-700 backdrop-blur-sm">
              <ShieldCheck className="h-3.5 w-3.5" />
              Проверено
            </span>
          )}

          {photoCount > 1 && (
            <span className="absolute left-3 bottom-3 inline-flex items-center gap-1.5 rounded-md bg-black/55 px-2 py-1 text-xs text-white">
              <Images className="h-3.5 w-3.5" />
              {photoCount}
            </span>
          )}
        </div>

        <CardContent className="flex flex-1 flex-col p-5">
          <div className="mb-1.5 flex items-start justify-between gap-2">
            <h3 className="line-clamp-1 text-lg font-semibold text-card-foreground">{venue.name}</h3>
            {venue.rating > 0 && (
              <span className="inline-flex shrink-0 items-center gap-1 rounded-lg bg-amber-100 px-2 py-0.5 text-sm font-medium text-amber-800">
                <Star className="h-3.5 w-3.5 fill-amber-500 text-amber-500" />
                {venue.rating.toFixed(1)}
              </span>
            )}
          </div>

          <div className="mb-1 flex items-center gap-1.5 text-sm text-muted-foreground">
            <MapPin className="h-4 w-4 shrink-0" />
            <span className="line-clamp-1">
              {venue.city}{venue.address ? `, ${venue.address}` : ""}
            </span>
            {venue.review_count > 0 && (
              <>
                <span aria-hidden>·</span>
                <span className="shrink-0">{venue.review_count} отзывов</span>
              </>
            )}
          </div>

          {(workingHours || (venue.capacity ?? 0) > 0) && (
            <div className="mb-2.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
              {workingHours && (
                <span className="inline-flex items-center gap-1.5">
                  <Clock className="h-4 w-4 shrink-0" />
                  {workingHours}
                </span>
              )}
              {(venue.capacity ?? 0) > 0 && (
                <span className="inline-flex items-center gap-1.5">
                  <Users className="h-4 w-4 shrink-0" />
                  до {venue.capacity} чел.
                </span>
              )}
            </div>
          )}

          {venue.description && (
            <p className="mb-3 line-clamp-2 text-sm text-muted-foreground">{venue.description}</p>
          )}

          {amenities.length > 0 && (
            <div className="mb-4 flex flex-wrap gap-1.5">
              {amenities.slice(0, 3).map((a) => (
                <span
                  key={a}
                  className="max-w-[10rem] truncate rounded-full bg-secondary px-2.5 py-1 text-xs text-secondary-foreground"
                >
                  {a}
                </span>
              ))}
              {amenities.length > 3 && (
                <span className="rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">
                  +{amenities.length - 3}
                </span>
              )}
            </div>
          )}

          <div className="mt-auto flex items-end justify-between gap-3 border-t border-border pt-4">
            <div className="leading-tight">
              {venue.price_from > 0 ? (
                <>
                  <div className="text-xs text-muted-foreground">от</div>
                  <div className="text-lg font-bold text-primary">
                    {venue.price_from.toLocaleString("ru-RU")} ₽
                    <span className="text-sm font-normal text-muted-foreground">/час</span>
                  </div>
                </>
              ) : (
                <div className="text-sm font-medium text-muted-foreground">Цена по запросу</div>
              )}
            </div>
            <span className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-colors group-hover:bg-primary/90">
              Забронировать
              <ArrowRight className="h-4 w-4" />
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

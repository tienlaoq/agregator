import Image from "next/image"
import Link from "next/link"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { MapPin, Star, ImageIcon } from "lucide-react"
import { venueCardImageSrc } from "@/lib/api"
import type { Venue } from "@/lib/types"
import { VENUE_TYPE_LABELS } from "@/lib/types"

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
 */
export function VenueCard({ venue, sizes = "(max-width: 640px) 100vw, (max-width: 1024px) 50vw, 33vw" }: VenueCardProps) {
  const cardImg = venueCardImageSrc(venue)

  return (
    <Link href={`/venues/${venue.slug}`}>
      <Card className="group cursor-pointer overflow-hidden border-border bg-card transition-all hover:shadow-xl h-full">
        <div className="relative aspect-[4/3] overflow-hidden bg-muted flex items-center justify-center">
          {cardImg ? (
            <Image
              src={cardImg}
              alt={venue.name}
              fill
              sizes={sizes}
              className="object-cover transition-transform duration-300 group-hover:scale-105"
            />
          ) : (
            <div className="flex flex-col items-center gap-1 text-muted-foreground/40">
              <ImageIcon className="h-8 w-8" />
              <span className="text-xs">Нет фото</span>
            </div>
          )}
          <Badge className="absolute left-3 top-3 border bg-primary/10 text-primary border-primary/20">
            {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
          </Badge>
        </div>
        <CardContent className="p-5">
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-lg font-semibold text-card-foreground line-clamp-1">{venue.name}</h3>
            <div className="flex items-center gap-1">
              <Star className="h-4 w-4 fill-amber-400 text-amber-400" />
              <span className="text-sm font-medium">{venue.rating?.toFixed(1) || "—"}</span>
              {venue.review_count > 0 && (
                <span className="text-sm text-muted-foreground">({venue.review_count})</span>
              )}
            </div>
          </div>
          <div className="mb-2 flex items-center gap-1 text-sm text-muted-foreground">
            <MapPin className="h-4 w-4 shrink-0" />
            <span className="line-clamp-1">
              {venue.city}{venue.address ? `, ${venue.address}` : ""}
            </span>
          </div>
          {venue.description && (
            <p className="mb-3 text-sm text-muted-foreground line-clamp-2">{venue.description}</p>
          )}
          {venue.amenities && venue.amenities.length > 0 && (
            <div className="mb-3 flex flex-wrap gap-1.5">
              {venue.amenities.slice(0, 3).map((a) => (
                <Badge key={a} variant="secondary" className="text-xs">{a}</Badge>
              ))}
            </div>
          )}
          <div className="flex items-center justify-between">
            <span className="text-lg font-bold text-primary">
              {venue.price_from > 0
                ? `от ${venue.price_from.toLocaleString("ru-RU")} ₽/час`
                : "Цена по запросу"}
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

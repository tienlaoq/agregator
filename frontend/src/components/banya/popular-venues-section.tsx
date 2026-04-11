"use client"

import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { ArrowRight, Star, MapPin, ImageIcon } from "lucide-react"
import Link from "next/link"
import { getVenues, venueCardImageSrc } from "@/lib/api"
import type { Venue } from "@/lib/types"
import { VENUE_TYPE_LABELS } from "@/lib/types"

export function PopularVenuesSection() {
  const [venues, setVenues] = useState<Venue[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getVenues({ page: 1, page_size: 3, sort_by: "rating" })
      .then((data) => setVenues(data.venues ?? []))
      .catch(() => setVenues([]))
      .finally(() => setLoading(false))
  }, [])

  if (!loading && venues.length === 0) return null

  return (
    <section className="bg-secondary/30 py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="mb-10 flex flex-col items-center justify-between gap-4 md:flex-row">
          <div>
            <h2 className="text-3xl font-bold text-foreground md:text-4xl">Популярные заведения</h2>
            <p className="mt-2 text-muted-foreground">Лучшие бани и сауны по отзывам посетителей</p>
          </div>
          <Button variant="outline" asChild className="gap-2">
            <Link href="/venues">
              Смотреть все
              <ArrowRight className="h-4 w-4" />
            </Link>
          </Button>
        </div>

        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {loading
            ? Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="h-80 animate-pulse rounded-xl bg-muted" />
              ))
            : venues.map((venue) => {
                const cardImg = venueCardImageSrc(venue)
                return (
                <Link key={venue.id} href={`/venues/${venue.slug}`}>
                  <Card className="group cursor-pointer overflow-hidden border-border bg-card transition-all hover:shadow-xl h-full">
                    <div className="relative aspect-[4/3] overflow-hidden bg-muted flex items-center justify-center">
                      {cardImg ? (
                        <img src={cardImg} alt={venue.name} className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105" />
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
                        <span className="line-clamp-1">{venue.city}{venue.address ? `, ${venue.address}` : ""}</span>
                      </div>
                      {venue.description && (
                        <p className="mb-3 text-sm text-muted-foreground line-clamp-2">{venue.description}</p>
                      )}
                      <div className="flex items-center justify-between">
                        <span className="text-lg font-bold text-primary">
                          {venue.price_from > 0 ? `от ${venue.price_from.toLocaleString("ru-RU")} ₽/час` : "Цена по запросу"}
                        </span>
                      </div>
                    </CardContent>
                  </Card>
                </Link>
              )})}
        </div>
      </div>
    </section>
  )
}

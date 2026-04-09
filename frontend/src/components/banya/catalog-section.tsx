"use client"

import { useState, useEffect, useCallback } from "react"
import { useSearchParams } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Card, CardContent } from "@/components/ui/card"
import { searchVenues, getVenues } from "@/lib/api"
import type { Venue } from "@/lib/types"
import { VENUE_TYPE_LABELS } from "@/lib/types"
import { Search, MapPin, X, Star, Building2, ImageIcon } from "lucide-react"
import Link from "next/link"

const types = ["all", "banya", "sauna", "hammam"]
const typeLabels: Record<string, string> = { all: "Все типы", banya: "Баня", sauna: "Сауна", hammam: "Хаммам" }
const priceRanges = [
  { label: "Любая цена", min: 0, max: 0 },
  { label: "до 1 500 ₽", min: 0, max: 1500 },
  { label: "1 500–2 500 ₽", min: 1500, max: 2500 },
  { label: "от 2 500 ₽", min: 2500, max: 0 },
]
const ratingOptions = [
  { label: "Любой рейтинг", value: 0 },
  { label: "4.5+", value: 4.5 },
  { label: "4.0+", value: 4.0 },
  { label: "3.5+", value: 3.5 },
]

const PAGE_SIZE = 12

export function CatalogSection() {
  const searchParams = useSearchParams()
  const initialQ = searchParams.get("q") || ""

  const [query, setQuery] = useState(initialQ)
  const [debouncedQuery, setDebouncedQuery] = useState(initialQ)
  const [selectedType, setSelectedType] = useState("all")
  const [selectedPriceIdx, setSelectedPriceIdx] = useState("0")
  const [selectedRatingIdx, setSelectedRatingIdx] = useState("0")
  const [page, setPage] = useState(1)
  const [venues, setVenues] = useState<Venue[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), 400)
    return () => clearTimeout(t)
  }, [query])

  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const priceRange = priceRanges[Number(selectedPriceIdx)]
      const ratingMin = ratingOptions[Number(selectedRatingIdx)].value
      const venueType = selectedType === "all" ? "" : selectedType

      const hasSearch = debouncedQuery || venueType || priceRange.min || priceRange.max || ratingMin

      if (hasSearch) {
        const data = await searchVenues({
          q: debouncedQuery || undefined,
          type: venueType || undefined,
          price_min: priceRange.min || undefined,
          price_max: priceRange.max || undefined,
          rating_min: ratingMin || undefined,
          page,
          page_size: PAGE_SIZE,
        })
        setVenues(data.venues ?? [])
        setTotal(data.total)
      } else {
        const data = await getVenues({ page, page_size: PAGE_SIZE, sort_by: "rating" })
        setVenues(data.venues ?? [])
        setTotal(data.total)
      }
    } catch {
      setVenues([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [debouncedQuery, selectedType, selectedPriceIdx, selectedRatingIdx, page])

  useEffect(() => {
    setPage(1)
  }, [debouncedQuery, selectedType, selectedPriceIdx, selectedRatingIdx])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const activeFilters = [
    selectedType !== "all" && typeLabels[selectedType],
    selectedPriceIdx !== "0" && priceRanges[Number(selectedPriceIdx)].label,
    selectedRatingIdx !== "0" && ratingOptions[Number(selectedRatingIdx)].label,
  ].filter(Boolean)

  const clearAllFilters = () => {
    setQuery("")
    setSelectedType("all")
    setSelectedPriceIdx("0")
    setSelectedRatingIdx("0")
  }

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <section id="catalog" className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <h2 className="mb-8 text-3xl font-bold text-foreground md:text-4xl">Каталог заведений</h2>

        {/* Search and Filters */}
        <div className="mb-6 space-y-4">
          <div className="flex flex-col gap-4 lg:flex-row">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                placeholder="Поиск по названию бани, сауны..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="h-11 pl-10"
              />
            </div>
            <div className="flex flex-wrap gap-3">
              <Select value={selectedType} onValueChange={setSelectedType}>
                <SelectTrigger className="h-11 w-[140px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {types.map((t) => (
                    <SelectItem key={t} value={t}>{typeLabels[t]}</SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={selectedPriceIdx} onValueChange={setSelectedPriceIdx}>
                <SelectTrigger className="h-11 w-[160px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {priceRanges.map((p, i) => (
                    <SelectItem key={i} value={String(i)}>{p.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={selectedRatingIdx} onValueChange={setSelectedRatingIdx}>
                <SelectTrigger className="h-11 w-[160px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ratingOptions.map((r, i) => (
                    <SelectItem key={i} value={String(i)}>{r.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          {/* Active Filters */}
          {activeFilters.length > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm text-muted-foreground">Фильтры:</span>
              {activeFilters.map((filter) => (
                <Badge key={filter as string} variant="secondary" className="text-xs">
                  {filter}
                </Badge>
              ))}
              <Button variant="ghost" size="sm" className="h-6 text-xs" onClick={clearAllFilters}>
                <X className="mr-1 h-3 w-3" />
                Сбросить
              </Button>
            </div>
          )}
        </div>

        {/* Results Count */}
        <p className="mb-6 text-muted-foreground">
          {loading ? "Поиск..." : `Найдено ${total} заведений`}
        </p>

        {/* Venues Grid */}
        {!loading && venues.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center py-16 text-center">
              <Building2 className="mb-4 h-12 w-12 text-muted-foreground/50" />
              <h3 className="mb-2 text-lg font-semibold">Ничего не найдено</h3>
              <p className="mb-4 text-sm text-muted-foreground">
                Попробуйте изменить поисковый запрос или сбросить фильтры
              </p>
              <Button variant="outline" onClick={clearAllFilters}>Сбросить фильтры</Button>
            </CardContent>
          </Card>
        ) : (
          <div className="mb-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {loading
              ? Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="h-80 animate-pulse rounded-xl bg-muted" />
                ))
              : venues.map((venue) => (
                  <Link key={venue.id} href={`/venues/${venue.slug}`}>
                    <Card className="group cursor-pointer overflow-hidden border-border bg-card transition-all hover:shadow-xl h-full">
                      <div className="relative aspect-[4/3] overflow-hidden bg-muted flex items-center justify-center">
                        {venue.image_url ? (
                          <img src={venue.image_url} alt={venue.name} className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105" />
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
                        {venue.amenities && venue.amenities.length > 0 && (
                          <div className="mb-3 flex flex-wrap gap-1.5">
                            {venue.amenities.slice(0, 3).map((a) => (
                              <Badge key={a} variant="secondary" className="text-xs">{a}</Badge>
                            ))}
                          </div>
                        )}
                        <div className="flex items-center justify-between">
                          <span className="text-lg font-bold text-primary">
                            {venue.price_from > 0 ? `от ${venue.price_from.toLocaleString("ru-RU")} ₽/час` : "Цена по запросу"}
                          </span>
                        </div>
                      </CardContent>
                    </Card>
                  </Link>
                ))}
          </div>
        )}

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => p - 1)}
            >
              Назад
            </Button>
            <span className="text-sm text-muted-foreground">
              Страница {page} из {totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Далее
            </Button>
          </div>
        )}
      </div>
    </section>
  )
}

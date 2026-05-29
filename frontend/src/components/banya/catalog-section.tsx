"use client"

import { useState, useEffect, useLayoutEffect } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
import { useQuery, keepPreviousData } from "@tanstack/react-query"
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
import { packCitiesForQuery, parseCitiesFromSearchParams, parseCitiesFromStableKey } from "@/lib/cities-http"
import type { Venue } from "@/lib/types"
import { Search, MapPin, X, Building2, AlertCircle } from "lucide-react"
import { VenueCard } from "@/components/banya/venue-card"

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

// Ключи, которые Next.js подмешивает в query (_rsc и т.д.): если включить их в deps синхронизации,
// строка URL «меняется» без смены q/city — эффект затирает город из поля справа пустым searchParams.get("city").
const NEXT_URL_NOISE_KEYS = new Set(["_rsc", "_next"])

function catalogStableSearchKey(sp: { forEach: (cb: (v: string, k: string) => void) => void }): string {
  const out = new URLSearchParams()
  sp.forEach((value, key) => {
    if (!NEXT_URL_NOISE_KEYS.has(key)) {
      out.append(key, value)
    }
  })
  return out.toString()
}

function normalizeCityList(raw: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const s of raw) {
    const t = s.trim()
    if (!t) continue
    const k = t.toLowerCase()
    if (seen.has(k)) continue
    seen.add(k)
    out.push(t)
  }
  return out
}

function sameCityLists(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const sa = [...a].map((s) => s.toLowerCase()).sort()
  const sb = [...b].map((s) => s.toLowerCase()).sort()
  return sa.every((v, i) => v === sb[i])
}

export type CatalogSectionProps = {
  /** SEO-хаб: город подставляется в фильтр, если в URL ещё нет city/cities */
  hubCity?: string
  /** Тип заведения по умолчанию для хаба (и сброс фильтров) */
  hubDefaultVenueType?: "all" | "banya" | "sauna" | "hammam"
  /** Заголовок блока каталога (например на городской посадочной) */
  catalogTitle?: string
  /**
   * SSR-данные с сервера (page.tsx / layout делает fetch до рендера).
   * Позволяют поисковому боту получить заполненный HTML без JS.
   * Клиент перетирает их при первом интерактивном запросе.
   */
  initialData?: { venues: Venue[]; total: number }
}

export function CatalogSection({
  hubCity,
  hubDefaultVenueType = "all",
  catalogTitle,
  initialData,
}: CatalogSectionProps = {}) {
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const router = useRouter()
  const initialQ = searchParams.get("q") || ""
  const initialCities = parseCitiesFromSearchParams(searchParams)

  const [query, setQuery] = useState(initialQ)
  const [debouncedQuery, setDebouncedQuery] = useState(initialQ)
  const [selectedCities, setSelectedCities] = useState<string[]>(() =>
    initialCities.length > 0 ? initialCities : hubCity ? [hubCity] : [],
  )
  const [cityDraft, setCityDraft] = useState("")
  const [selectedType, setSelectedType] = useState(hubDefaultVenueType)
  const [selectedPriceIdx, setSelectedPriceIdx] = useState("0")
  const [selectedRatingIdx, setSelectedRatingIdx] = useState("0")
  const [page, setPage] = useState(1)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), 400)
    return () => clearTimeout(t)
  }, [query])

  // Города в запросе без debounce: иначе 300–400 мс citiesEff пустой → уходит getVenues и видны все заведения.

  // Только при реальном изменении «наших» параметров (без _rsc), иначе затирается ввод в поле «Город».
  const catalogStableKey = catalogStableSearchKey(searchParams)
  // layout: чтобы state совпал с URL до useEffect с fetch — иначе гонка: уходит List без city,
  // позже приходит ответ и затирает выдачу поиска.
  useLayoutEffect(() => {
    const q = searchParams.get("q") || ""
    const cities = parseCitiesFromSearchParams(searchParams)
    setQuery(q)
    setDebouncedQuery(q)
    const effCities = cities.length > 0 ? cities : hubCity ? [hubCity] : []
    setSelectedCities(effCities)
    setCityDraft("")
    // eslint-disable-next-line react-hooks/exhaustive-deps -- searchParams по смыслу совпадает со stable key; сам ref меняется каждый кадр
  }, [catalogStableKey, hubCity])

  // Синхронизация полей → URL: иначе в адресе остаётся старый ?city= и не совпадает с вводом.
  useEffect(() => {
    const wantQ = debouncedQuery.trim()
    const wantCities = selectedCities
    const p = new URLSearchParams(catalogStableKey)
    const urlQ = (p.get("q") ?? "").trim()
    const urlCities = parseCitiesFromStableKey(catalogStableKey)
    if (urlQ === wantQ && sameCityLists(urlCities, wantCities)) return
    const next = new URLSearchParams()
    if (wantQ) next.set("q", wantQ)
    if (wantCities.length === 1) {
      next.set("city", wantCities[0])
    } else if (wantCities.length > 1) {
      const packed = packCitiesForQuery(wantCities)
      if (packed) next.set("cities", packed)
    }
    const qs = next.toString()
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false })
  }, [debouncedQuery, selectedCities, catalogStableKey, pathname, router])

  // Сбрасываем пагинацию при смене фильтров
  useEffect(() => {
    setPage(1)
  }, [debouncedQuery, selectedCities, selectedType, selectedPriceIdx, selectedRatingIdx])

  // Параметры запроса — мемоизированы в query key для TanStack Query
  const priceRange = priceRanges[Number(selectedPriceIdx)]
  const ratingMin = ratingOptions[Number(selectedRatingIdx)].value
  const venueType = selectedType === "all" ? "" : selectedType
  const qEff = debouncedQuery.trim()
  const hasSearch = Boolean(qEff || selectedCities.length > 0 || venueType || priceRange.min || priceRange.max || ratingMin)

  const queryKey = [
    "catalog",
    hasSearch ? "search" : "list",
    qEff,
    selectedCities,
    venueType,
    priceRange.min,
    priceRange.max,
    ratingMin,
    page,
  ] as const

  const { data, isFetching, isError } = useQuery({
    queryKey,
    queryFn: ({ signal }) =>
      hasSearch
        ? searchVenues(
            {
              q: qEff || undefined,
              city: selectedCities.length > 0 ? selectedCities : undefined,
              type: venueType || undefined,
              price_min: priceRange.min || undefined,
              price_max: priceRange.max || undefined,
              rating_min: ratingMin || undefined,
              page,
              page_size: PAGE_SIZE,
            },
            { signal },
          )
        : getVenues({ page, page_size: PAGE_SIZE, sort_by: "rating" }, { signal }),
    placeholderData: keepPreviousData,
    // SSR-данные — начальные; не ждём первого рендера с пустым экраном
    initialData: !hasSearch && page === 1 && initialData
      ? { venues: initialData.venues, total: initialData.total, page: 1, page_size: PAGE_SIZE }
      : undefined,
    staleTime: 30_000,
    retry: 2,
  })

  const venues: Venue[] = data?.venues ?? []
  const total: number = data?.total ?? 0
  const loading = isFetching && venues.length === 0

  const activeFilters: { key: string; label: string; onRemove?: () => void }[] = [
    ...selectedCities.map((c) => ({
      key: `city:${c}`,
      label: `Город: ${c}`,
      onRemove: () =>
        setSelectedCities((prev) => prev.filter((x) => x.toLowerCase() !== c.toLowerCase())),
    })),
    ...(selectedType !== "all"
      ? [{ key: "type", label: typeLabels[selectedType] as string }]
      : []),
    ...(selectedPriceIdx !== "0"
      ? [{ key: "price", label: priceRanges[Number(selectedPriceIdx)].label }]
      : []),
    ...(selectedRatingIdx !== "0"
      ? [{ key: "rating", label: ratingOptions[Number(selectedRatingIdx)].label }]
      : []),
  ]

  const clearAllFilters = () => {
    setQuery("")
    setDebouncedQuery("")
    setSelectedCities(hubCity ? [hubCity] : [])
    setCityDraft("")
    setSelectedType(hubDefaultVenueType)
    setSelectedPriceIdx("0")
    setSelectedRatingIdx("0")
  }

  const commitCityDraft = () => {
    const t = cityDraft.trim()
    if (!t) return
    setSelectedCities((prev) => {
      if (prev.some((c) => c.toLowerCase() === t.toLowerCase())) return prev
      return [...prev, t]
    })
    setCityDraft("")
  }

  const totalPages = Math.ceil(total / PAGE_SIZE)

  return (
    <section id="catalog" className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <h2 className="mb-8 text-3xl font-bold text-foreground md:text-4xl">
          {catalogTitle ?? "Каталог заведений"}
        </h2>

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
            <div className="relative w-full lg:max-w-[240px]">
              <MapPin className="absolute left-3 top-1/2 z-10 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                placeholder="Город — Enter, добавить"
                value={cityDraft}
                onChange={(e) => setCityDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault()
                    commitCityDraft()
                  }
                }}
                className="h-11 pl-10"
              />
            </div>
            <div className="flex flex-wrap gap-3">
              <Select
                value={selectedType}
                onValueChange={(v) => setSelectedType(v as "all" | "banya" | "sauna" | "hammam")}
              >
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
              {activeFilters.map((f) => (
                <Badge key={f.key} variant="secondary" className="inline-flex items-center gap-1 text-xs">
                  {f.label}
                  {f.onRemove && (
                    <button
                      type="button"
                      className="rounded-sm p-0.5 hover:bg-muted-foreground/20"
                      aria-label="Убрать фильтр"
                      onClick={f.onRemove}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
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
          {loading
            ? "Поиск..."
            : isFetching
              ? `Найдено ${total} заведений (обновление…)`
              : `Найдено ${total} заведений`}
        </p>

        {/* Ошибка сети / сервера — показываем явно вместо молчаливого пустого списка */}
        {isError && (
          <Card className="mb-6 border-destructive/30 bg-destructive/5">
            <CardContent className="flex items-center gap-3 py-4">
              <AlertCircle className="h-5 w-5 shrink-0 text-destructive" />
              <p className="text-sm text-destructive">
                Не удалось загрузить список заведений. Проверьте соединение и попробуйте снова.
              </p>
            </CardContent>
          </Card>
        )}

        {/* Venues Grid */}
        {!loading && !isError && venues.length === 0 ? (
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
                  <VenueCard key={venue.id} venue={venue} />
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

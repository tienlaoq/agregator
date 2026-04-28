"use client"

import { useState, useEffect, useLayoutEffect, useCallback, useRef } from "react"
import { usePathname, useRouter, useSearchParams } from "next/navigation"
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
import { getPublicMastersCatalog, listPublicMasters, masterCardImageSrc, masterCardPriceLabel } from "@/lib/api"
import { packCitiesForQuery, parseCitiesFromSearchParams, parseCitiesFromStableKey } from "@/lib/cities-http"
import type { MasterProfile } from "@/lib/types"
import { Search, MapPin, X, Users, ImageIcon } from "lucide-react"
import Link from "next/link"

/** Значение `none` — без фильтра по API; не путать с `both` («и выезд, и в бане»). */
const workFormatFilterValues = ["none", "mobile", "venue", "both"] as const
const workFormatFilterLabels: Record<(typeof workFormatFilterValues)[number], string> = {
  none: "Любой формат",
  mobile: "Выезд к клиенту",
  venue: "В бане / у заведения",
  both: "И то и другое",
}
const workFormatCardLabels: Record<string, string> = {
  mobile: workFormatFilterLabels.mobile,
  venue: workFormatFilterLabels.venue,
  both: workFormatFilterLabels.both,
}

const priceRanges = [
  { label: "Любая цена", min: 0, max: 0 },
  { label: "до 1 500 ₽", min: 0, max: 1500 },
  { label: "1 500–2 500 ₽", min: 1500, max: 2500 },
  { label: "от 2 500 ₽", min: 2500, max: 0 },
]

const PAGE_SIZE = 12

function isAbortError(e: unknown): boolean {
  if (e instanceof DOMException && e.name === "AbortError") return true
  if (e instanceof Error && e.name === "AbortError") return true
  return false
}

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

function sameCityLists(a: string[], b: string[]): boolean {
  if (a.length !== b.length) return false
  const sa = [...a].map((s) => s.toLowerCase()).sort()
  const sb = [...b].map((s) => s.toLowerCase()).sort()
  return sa.every((v, i) => v === sb[i])
}

export function MastersCatalogSection() {
  const searchParams = useSearchParams()
  const pathname = usePathname()
  const router = useRouter()
  const initialQ = searchParams.get("q") || ""
  const initialCities = parseCitiesFromSearchParams(searchParams)

  const [query, setQuery] = useState(initialQ)
  const [debouncedQuery, setDebouncedQuery] = useState(initialQ)
  const [selectedCities, setSelectedCities] = useState<string[]>(initialCities)
  const [cityDraft, setCityDraft] = useState("")
  const [selectedWorkFormat, setSelectedWorkFormat] = useState<(typeof workFormatFilterValues)[number]>("none")
  const [selectedPriceIdx, setSelectedPriceIdx] = useState("0")
  const [page, setPage] = useState(1)
  const [masters, setMasters] = useState<MasterProfile[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const fetchGen = useRef(0)

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query), 400)
    return () => clearTimeout(t)
  }, [query])

  const catalogStableKey = catalogStableSearchKey(searchParams)
  useLayoutEffect(() => {
    const q = searchParams.get("q") || ""
    const cities = parseCitiesFromSearchParams(searchParams)
    setQuery(q)
    setDebouncedQuery(q)
    setSelectedCities(cities)
    setCityDraft("")
    // eslint-disable-next-line react-hooks/exhaustive-deps -- searchParams по смыслу совпадает со stable key; сам ref меняется каждый кадр
  }, [catalogStableKey])

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

  const fetchData = useCallback(
    async (signal?: AbortSignal) => {
      const gen = ++fetchGen.current
      setLoading(true)
      try {
        const priceRange = priceRanges[Number(selectedPriceIdx)]
        const workFormat = selectedWorkFormat === "none" ? "" : selectedWorkFormat

        const qEff = debouncedQuery.trim()
        const citiesEff = selectedCities

        const hasSearch =
          qEff ||
          citiesEff.length > 0 ||
          workFormat ||
          priceRange.min ||
          priceRange.max

        const fetchInit = signal ? { signal } : undefined

        if (hasSearch) {
          const data = await listPublicMasters(
            {
              q: qEff || undefined,
              city: citiesEff.length > 0 ? citiesEff : undefined,
              work_format: workFormat || undefined,
              price_min: priceRange.min || undefined,
              price_max: priceRange.max || undefined,
              page,
              page_size: PAGE_SIZE,
            },
            fetchInit,
          )
          if (gen !== fetchGen.current) return
          setMasters(data.masters ?? [])
          setTotal(data.total)
        } else {
          const data = await getPublicMastersCatalog(
            { page, page_size: PAGE_SIZE },
            fetchInit,
          )
          if (gen !== fetchGen.current) return
          setMasters(data.masters ?? [])
          setTotal(data.total)
        }
      } catch (e: unknown) {
        if (gen !== fetchGen.current) return
        if (isAbortError(e)) return
        setMasters([])
        setTotal(0)
      } finally {
        if (gen === fetchGen.current) setLoading(false)
      }
    },
    [debouncedQuery, selectedCities, selectedWorkFormat, selectedPriceIdx, page],
  )

  useEffect(() => {
    setPage(1)
  }, [debouncedQuery, selectedCities, selectedWorkFormat, selectedPriceIdx])

  useEffect(() => {
    const ac = new AbortController()
    void fetchData(ac.signal)
    return () => ac.abort()
  }, [fetchData])

  const activeFilters: { key: string; label: string; onRemove?: () => void }[] = [
    ...selectedCities.map((c) => ({
      key: `city:${c}`,
      label: `Город: ${c}`,
      onRemove: () =>
        setSelectedCities((prev) => prev.filter((x) => x.toLowerCase() !== c.toLowerCase())),
    })),
    ...(selectedWorkFormat !== "none"
      ? [{ key: "work_format", label: workFormatFilterLabels[selectedWorkFormat] }]
      : []),
    ...(selectedPriceIdx !== "0"
      ? [{ key: "price", label: priceRanges[Number(selectedPriceIdx)].label }]
      : []),
  ]

  const clearAllFilters = () => {
    setQuery("")
    setDebouncedQuery("")
    setSelectedCities([])
    setCityDraft("")
    setSelectedWorkFormat("none")
    setSelectedPriceIdx("0")
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
    <section id="masters-catalog" className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <h2 className="mb-8 text-3xl font-bold text-foreground md:text-4xl">Пар-мастера</h2>

        <div className="mb-6 space-y-4">
          <div className="flex flex-col gap-4 lg:flex-row">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                placeholder="Поиск по имени мастера, описанию, городу…"
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
                value={selectedWorkFormat}
                onValueChange={(v) => setSelectedWorkFormat(v as (typeof workFormatFilterValues)[number])}
              >
                <SelectTrigger className="h-11 min-w-[200px] max-w-[240px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {workFormatFilterValues.map((t) => (
                    <SelectItem key={t} value={t}>
                      {workFormatFilterLabels[t]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Select value={selectedPriceIdx} onValueChange={setSelectedPriceIdx}>
                <SelectTrigger className="h-11 w-[160px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {priceRanges.map((p, i) => (
                    <SelectItem key={i} value={String(i)}>
                      {p.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

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

        <p className="mb-6 text-muted-foreground">
          {loading ? "Поиск..." : `Найдено ${total} мастеров`}
        </p>

        {!loading && masters.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center py-16 text-center">
              <Users className="mb-4 h-12 w-12 text-muted-foreground/50" />
              <h3 className="mb-2 text-lg font-semibold">Ничего не найдено</h3>
              <p className="mb-4 text-sm text-muted-foreground">
                Попробуйте изменить поисковый запрос или сбросить фильтры
              </p>
              <Button variant="outline" onClick={clearAllFilters}>
                Сбросить фильтры
              </Button>
            </CardContent>
          </Card>
        ) : (
          <div className="mb-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {loading
              ? Array.from({ length: 6 }).map((_, i) => (
                  <div key={i} className="h-80 animate-pulse rounded-xl bg-muted" />
                ))
              : masters.map((m) => {
                  const priceLine = masterCardPriceLabel(m)
                  const cardImg = masterCardImageSrc(m)
                  const wfLabel = workFormatCardLabels[m.work_format] ?? m.work_format
                  return (
                    <Link key={m.id} href={`/masters/${m.slug}`}>
                      <Card className="group h-full cursor-pointer overflow-hidden border-border bg-card transition-all hover:border-primary/40 hover:shadow-xl">
                        <div className="relative flex aspect-[4/3] items-center justify-center overflow-hidden bg-muted">
                          {cardImg ? (
                            <img
                              src={cardImg}
                              alt=""
                              className="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
                            />
                          ) : (
                            <div className="flex flex-col items-center gap-1 text-muted-foreground/40">
                              <ImageIcon className="h-8 w-8" />
                              <span className="text-xs">Нет фото</span>
                            </div>
                          )}
                          <Badge className="absolute left-3 top-3 border border-primary/20 bg-primary/10 text-primary">
                            {wfLabel}
                          </Badge>
                        </div>
                        <CardContent className="p-5">
                          <h3 className="mb-2 line-clamp-1 text-lg font-semibold text-card-foreground">
                            {m.display_name}
                          </h3>
                          <div className="mb-2 flex items-center gap-1 text-sm text-muted-foreground">
                            <MapPin className="h-4 w-4 shrink-0" />
                            <span className="line-clamp-1">{m.city}</span>
                          </div>
                          {m.experience_years > 0 && (
                            <Badge variant="outline" className="mb-2 text-xs">
                              Опыт {m.experience_years} лет
                            </Badge>
                          )}
                          {m.bio ? (
                            <p className="mb-3 line-clamp-2 text-sm text-muted-foreground">{m.bio}</p>
                          ) : null}
                          {priceLine ? (
                            <span className="text-lg font-bold text-primary">{priceLine}</span>
                          ) : null}
                        </CardContent>
                      </Card>
                    </Link>
                  )
                })}
          </div>
        )}

        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              Назад
            </Button>
            <span className="text-sm text-muted-foreground">
              Страница {page} из {totalPages}
            </span>
            <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
              Далее
            </Button>
          </div>
        )}
      </div>
    </section>
  )
}

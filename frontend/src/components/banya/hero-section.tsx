"use client"

import { useState, useRef, useEffect } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Search, MapPin } from "lucide-react"
import { RUSSIAN_CITIES } from "@/components/banya/city-combobox"

const VENUES_CATALOG = "/venues#catalog"

function venuesSearchHref(params: URLSearchParams): string {
  const qs = params.toString()
  return qs ? `/venues?${qs}#catalog` : VENUES_CATALOG
}

export function HeroSection() {
  const router = useRouter()
  const [city, setCity] = useState("")
  const [name, setName] = useState("")
  const [suggestions, setSuggestions] = useState<string[]>([])
  const [showSuggestions, setShowSuggestions] = useState(false)
  const wrapperRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (city.length < 1) {
      setSuggestions([])
      return
    }
    const q = city.toLowerCase()
    setSuggestions(RUSSIAN_CITIES.filter((c) => c.toLowerCase().startsWith(q)).slice(0, 6))
  }, [city])

  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setShowSuggestions(false)
      }
    }
    document.addEventListener("mousedown", handleClickOutside)
    return () => document.removeEventListener("mousedown", handleClickOutside)
  }, [])

  const selectCity = (c: string) => {
    setCity(c)
    setShowSuggestions(false)
  }

  const handleSearch = () => {
    const params = new URLSearchParams()
    const qName = name.trim()
    const qCity = city.trim()
    if (qName) params.set("q", qName)
    if (qCity) params.set("city", qCity)
    router.push(venuesSearchHref(params))
  }

  return (
    <section className="relative min-h-[600px] w-full overflow-hidden">
      {/* Background with warm overlay */}
      <div
        className="absolute inset-0 bg-cover bg-center"
        style={{
          // Keep payload small for local/dev LCP; full-width hero rarely needs >1280px source.
          backgroundImage: `url('https://images.unsplash.com/photo-1584622650111-993a426fbf0a?q=75&w=1280&auto=format&fit=crop')`,
        }}
      >
        <div className="absolute inset-0 bg-gradient-to-b from-[#78350F]/70 via-[#92400E]/50 to-[#1C1917]/80" />
      </div>

      {/* Content */}
      <div className="relative flex min-h-[600px] flex-col items-center justify-center px-4 text-center">
        <h1 className="mb-4 max-w-4xl text-balance text-4xl font-bold tracking-tight text-white md:text-5xl lg:text-6xl">
          Найди идеальную баню или сауну рядом с тобой
        </h1>
        <p className="mb-8 max-w-2xl text-pretty text-lg text-white/90 md:text-xl">
          Более 500 проверенных заведений с отзывами, фото и онлайн-бронированием
        </p>

        {/* Search Bar */}
        <form
          onSubmit={(e) => { e.preventDefault(); handleSearch() }}
          className="flex w-full max-w-2xl flex-col gap-3 sm:flex-row"
        >
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 z-10 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              placeholder="Название бани, сауны..."
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="h-12 bg-white pl-10 text-base text-foreground placeholder:text-muted-foreground"
            />
          </div>
          <div ref={wrapperRef} className="relative sm:w-[200px]">
            <MapPin className="absolute left-3 top-1/2 z-10 h-5 w-5 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="text"
              placeholder="Город..."
              value={city}
              onChange={(e) => setCity(e.target.value)}
              onFocus={() => setShowSuggestions(true)}
              className="h-12 bg-white pl-10 text-base text-foreground placeholder:text-muted-foreground"
            />
            {showSuggestions && suggestions.length > 0 && (
              <div className="absolute top-full z-50 mt-1 w-full rounded-md border bg-popover p-1 shadow-md">
                {suggestions.map((c) => (
                  <button
                    key={c}
                    type="button"
                    className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground"
                    onMouseDown={(e) => {
                      e.preventDefault()
                      selectCity(c)
                    }}
                  >
                    <MapPin className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    {c}
                  </button>
                ))}
              </div>
            )}
          </div>
          <Button type="submit" size="lg" className="h-12 gap-2 px-8">
            <Search className="h-5 w-5" />
            Найти
          </Button>
        </form>

        {/* Popular cities */}
        <div className="mt-6 flex flex-wrap items-center justify-center gap-2">
          <span className="text-sm text-white/70">Популярные:</span>
          {["Москва", "Санкт-Петербург", "Казань", "Новосибирск"].map((c) => {
            const params = new URLSearchParams()
            params.set("city", c)
            return (
              <Button
                key={c}
                asChild
                variant="ghost"
                size="sm"
                className="h-7 border border-white/30 text-sm text-white hover:bg-white/20 hover:text-white"
              >
                <Link href={venuesSearchHref(params)}>{c}</Link>
              </Button>
            )
          })}
        </div>
      </div>
    </section>
  )
}

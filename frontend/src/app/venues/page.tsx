"use client";

import { Suspense, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { VenueCard } from "@/components/venue-card";
import { StarRating } from "@/components/star-rating";
import { getVenues } from "@/lib/api";
import { VENUE_TYPE_LABELS } from "@/lib/types";
import { Search, SlidersHorizontal, X } from "lucide-react";

export default function VenuesPage() {
  return (
    <Suspense
      fallback={
        <div className="mx-auto max-w-7xl px-4 py-6">
          <div className="h-8 w-48 animate-pulse rounded bg-muted" />
        </div>
      }
    >
      <VenuesContent />
    </Suspense>
  );
}

function VenuesContent() {
  const searchParams = useSearchParams();
  const initialQ = searchParams.get("q") ?? "";

  const [search, setSearch] = useState(initialQ);
  const [type, setType] = useState("");
  const [minPrice, setMinPrice] = useState("");
  const [maxPrice, setMaxPrice] = useState("");
  const [minRating, setMinRating] = useState(0);
  const [showFilters, setShowFilters] = useState(false);

  const { data: venues, isLoading } = useQuery({
    queryKey: ["venues", search, type, minPrice, maxPrice, minRating],
    queryFn: () =>
      getVenues({
        q: search || undefined,
        type: type || undefined,
        min_price: minPrice ? Number(minPrice) : undefined,
        max_price: maxPrice ? Number(maxPrice) : undefined,
        min_rating: minRating || undefined,
      }),
  });

  const clearFilters = () => {
    setType("");
    setMinPrice("");
    setMaxPrice("");
    setMinRating(0);
    setSearch("");
  };

  const hasFilters = type || minPrice || maxPrice || minRating > 0;

  return (
    <div className="mx-auto max-w-7xl px-4 py-6">
      <div className="mb-6">
        <h1 className="mb-4 text-2xl font-bold">Каталог заведений</h1>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Поиск по названию или городу..."
              className="pl-10"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </div>
          <Button
            variant="outline"
            onClick={() => setShowFilters(!showFilters)}
            className="gap-2 md:hidden"
          >
            <SlidersHorizontal className="h-4 w-4" />
            Фильтры
          </Button>
        </div>
      </div>

      <div className="flex gap-6">
        {/* Sidebar filters */}
        <aside
          className={`${
            showFilters ? "block" : "hidden"
          } w-full shrink-0 space-y-6 md:block md:w-56`}
        >
          <div>
            <Label className="mb-2 text-sm font-semibold">Тип заведения</Label>
            <div className="mt-2 space-y-1.5">
              {Object.entries(VENUE_TYPE_LABELS).map(([value, label]) => (
                <label
                  key={value}
                  className="flex cursor-pointer items-center gap-2 text-sm"
                >
                  <input
                    type="radio"
                    name="type"
                    value={value}
                    checked={type === value}
                    onChange={() => setType(type === value ? "" : value)}
                    className="accent-primary"
                  />
                  {label}
                </label>
              ))}
            </div>
          </div>

          <Separator />

          <div>
            <Label className="mb-2 text-sm font-semibold">Цена (₽/час)</Label>
            <div className="mt-2 flex items-center gap-2">
              <Input
                type="number"
                placeholder="от"
                value={minPrice}
                onChange={(e) => setMinPrice(e.target.value)}
                className="h-8"
              />
              <span className="text-muted-foreground">—</span>
              <Input
                type="number"
                placeholder="до"
                value={maxPrice}
                onChange={(e) => setMaxPrice(e.target.value)}
                className="h-8"
              />
            </div>
          </div>

          <Separator />

          <div>
            <Label className="mb-2 text-sm font-semibold">Минимальный рейтинг</Label>
            <div className="mt-2">
              <StarRating
                rating={minRating}
                size="lg"
                interactive
                onChange={setMinRating}
              />
              {minRating > 0 && (
                <p className="mt-1 text-xs text-muted-foreground">
                  от {minRating} звёзд
                </p>
              )}
            </div>
          </div>

          {hasFilters && (
            <>
              <Separator />
              <Button
                variant="ghost"
                size="sm"
                className="w-full gap-1"
                onClick={clearFilters}
              >
                <X className="h-3.5 w-3.5" />
                Сбросить фильтры
              </Button>
            </>
          )}
        </aside>

        {/* Results grid */}
        <div className="flex-1">
          {isLoading ? (
            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
              {Array.from({ length: 6 }).map((_, i) => (
                <div
                  key={i}
                  className="h-72 animate-pulse rounded-xl bg-muted"
                />
              ))}
            </div>
          ) : venues && venues.length > 0 ? (
            <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
              {venues.map((venue) => (
                <VenueCard key={venue.id} venue={venue} />
              ))}
            </div>
          ) : (
            <div className="flex flex-col items-center py-16 text-center">
              <Search className="mb-4 h-12 w-12 text-muted-foreground/50" />
              <h3 className="mb-2 text-lg font-semibold">Ничего не найдено</h3>
              <p className="text-sm text-muted-foreground">
                Попробуйте изменить параметры поиска
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

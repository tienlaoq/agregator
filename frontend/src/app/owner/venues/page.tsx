"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { StarRating } from "@/components/star-rating";
import { getOwnerVenues } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { VENUE_TYPE_LABELS } from "@/lib/types";
import { Plus, MapPin, Building2 } from "lucide-react";

export default function OwnerVenuesPage() {
  const router = useRouter();
  const { token, user, hydrated } = useAuthStore();

  useEffect(() => {
    if (hydrated && (!token || user?.role !== "owner")) {
      router.push("/auth/login");
    }
  }, [hydrated, token, user, router]);

  const { data: venues, isLoading } = useQuery({
    queryKey: ["owner-venues"],
    queryFn: getOwnerVenues,
    enabled: !!token && user?.role === "owner",
  });

  if (!hydrated || !token) return null;

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Мои заведения</h1>
        <Button onClick={() => router.push("/owner/venues/new")}>
          <Plus className="mr-1 h-4 w-4" />
          Добавить заведение
        </Button>
      </div>

      {isLoading ? (
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-28 animate-pulse rounded-xl bg-muted" />
          ))}
        </div>
      ) : venues && venues.length > 0 ? (
        <div className="space-y-4">
          {venues.map((venue) => (
            <Link key={venue.id} href={`/venues/${venue.slug}`}>
              <Card className="transition-shadow hover:shadow-md">
                <CardContent className="pt-4">
                  <div className="flex gap-4">
                    <div className="hidden h-20 w-20 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-amber-200 to-orange-300 sm:flex">
                      <span className="text-2xl">🏛️</span>
                    </div>
                    <div className="flex-1 space-y-1">
                      <div className="flex items-center gap-2">
                        <h3 className="font-semibold">{venue.name}</h3>
                        <Badge variant="secondary">
                          {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
                        </Badge>
                      </div>
                      <div className="flex items-center gap-2">
                        <StarRating rating={venue.rating} size="sm" />
                        <span className="text-xs text-muted-foreground">
                          ({venue.review_count} отзывов)
                        </span>
                      </div>
                      <div className="flex items-center gap-1 text-sm text-muted-foreground">
                        <MapPin className="h-3.5 w-3.5" />
                        {venue.address}
                      </div>
                      <p className="text-sm font-medium text-primary">
                        от {venue.price_from.toLocaleString("ru-RU")} ₽/час
                      </p>
                    </div>
                  </div>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center py-16 text-center">
          <Building2 className="mb-4 h-12 w-12 text-muted-foreground/50" />
          <h3 className="mb-2 text-lg font-semibold">Заведений пока нет</h3>
          <p className="mb-4 text-sm text-muted-foreground">
            Добавьте своё первое заведение на БаняГид
          </p>
          <Button onClick={() => router.push("/owner/venues/new")}>
            <Plus className="mr-1 h-4 w-4" />
            Добавить заведение
          </Button>
        </div>
      )}
    </div>
  );
}

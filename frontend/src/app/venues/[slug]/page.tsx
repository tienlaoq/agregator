"use client";

import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { StarRating } from "@/components/star-rating";
import { BookingForm } from "@/components/booking-form";
import { ReviewList } from "@/components/review-list";
import { getVenueBySlug, getVenueReviews } from "@/lib/api";
import { VENUE_TYPE_LABELS } from "@/lib/types";
import Image from "next/image";
import { MapPin, Phone, Clock, Droplets } from "lucide-react";

export default function VenueDetailPage() {
  const { slug } = useParams<{ slug: string }>();

  const {
    data: venue,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["venue", slug],
    queryFn: () => getVenueBySlug(slug),
    enabled: !!slug,
  });

  const {
    data: reviews,
    refetch: refetchReviews,
  } = useQuery({
    queryKey: ["venue-reviews", venue?.id],
    queryFn: () => getVenueReviews(venue!.id),
    enabled: !!venue?.id,
  });

  if (isLoading) {
    return (
      <div className="mx-auto max-w-7xl px-4 py-8">
        <div className="grid gap-8 lg:grid-cols-3">
          <div className="space-y-4 lg:col-span-2">
            <div className="h-80 animate-pulse rounded-xl bg-muted" />
            <div className="h-8 w-1/2 animate-pulse rounded bg-muted" />
            <div className="h-4 w-3/4 animate-pulse rounded bg-muted" />
          </div>
          <div className="h-96 animate-pulse rounded-xl bg-muted" />
        </div>
      </div>
    );
  }

  if (error || !venue) {
    return (
      <div className="flex flex-col items-center py-20 text-center">
        <h2 className="mb-2 text-xl font-semibold">Заведение не найдено</h2>
        <p className="text-muted-foreground">
          Проверьте адрес или вернитесь в каталог
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl px-4 py-8">
      {/* Photo gallery */}
      <div className="mb-8 grid gap-2 sm:grid-cols-4 sm:grid-rows-2">
        <div className="relative aspect-[16/10] overflow-hidden rounded-xl bg-gradient-to-br from-amber-200 to-orange-300 sm:col-span-2 sm:row-span-2">
          {venue.image_url ? (
            <Image
              src={venue.image_url}
              alt={venue.name}
              fill
              className="object-cover"
            />
          ) : (
            <div className="flex h-full items-center justify-center">
              <span className="text-6xl">🏛️</span>
            </div>
          )}
        </div>
        {[1, 2, 3, 4].map((i) => (
          <div
            key={i}
            className="hidden aspect-video overflow-hidden rounded-xl bg-gradient-to-br from-amber-100 to-orange-200 sm:block"
          >
            <div className="flex h-full items-center justify-center">
              <span className="text-2xl opacity-50">📷</span>
            </div>
          </div>
        ))}
      </div>

      <div className="grid gap-8 lg:grid-cols-3">
        {/* Main content */}
        <div className="space-y-6 lg:col-span-2">
          <div>
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <Badge variant="secondary">
                {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
              </Badge>
              <div className="flex items-center gap-1">
                <StarRating rating={venue.rating} size="sm" showValue />
                <span className="text-xs text-muted-foreground">
                  ({venue.review_count} отзывов)
                </span>
              </div>
            </div>
            <h1 className="mb-2 text-3xl font-bold">{venue.name}</h1>
            <div className="flex flex-col gap-2 text-sm text-muted-foreground">
              <div className="flex items-center gap-2">
                <MapPin className="h-4 w-4 shrink-0" />
                {venue.address}
              </div>
              {venue.phone && (
                <div className="flex items-center gap-2">
                  <Phone className="h-4 w-4 shrink-0" />
                  {venue.phone}
                </div>
              )}
            </div>
          </div>

          <Separator />

          <Tabs defaultValue="about">
            <TabsList>
              <TabsTrigger value="about">О заведении</TabsTrigger>
              <TabsTrigger value="services">Услуги</TabsTrigger>
              <TabsTrigger value="reviews">
                Отзывы ({reviews?.length ?? 0})
              </TabsTrigger>
            </TabsList>

            <TabsContent value="about" className="mt-4 space-y-4">
              <p className="text-sm leading-relaxed text-foreground/80">
                {venue.description}
              </p>
              {venue.amenities.length > 0 && (
                <div>
                  <h3 className="mb-3 font-semibold">Удобства</h3>
                  <div className="flex flex-wrap gap-2">
                    {venue.amenities.map((a) => (
                      <Badge key={a} variant="outline" className="gap-1">
                        <Droplets className="h-3 w-3" />
                        {a}
                      </Badge>
                    ))}
                  </div>
                </div>
              )}
            </TabsContent>

            <TabsContent value="services" className="mt-4">
              {venue.services.length > 0 ? (
                <div className="space-y-3">
                  {venue.services.map((service) => (
                    <div
                      key={service.id}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <div>
                        <p className="font-medium">{service.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {service.description}
                        </p>
                        {service.duration_minutes > 0 && (
                          <div className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
                            <Clock className="h-3 w-3" />
                            {service.duration_minutes} мин
                          </div>
                        )}
                      </div>
                      <span className="whitespace-nowrap font-semibold text-primary">
                        {service.price.toLocaleString("ru-RU")} ₽
                      </span>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="py-8 text-center text-muted-foreground">
                  Информация об услугах будет добавлена позже
                </p>
              )}
            </TabsContent>

            <TabsContent value="reviews" className="mt-4">
              <ReviewList
                venueId={venue.id}
                reviews={reviews ?? []}
                onReviewAdded={() => refetchReviews()}
              />
            </TabsContent>
          </Tabs>
        </div>

        {/* Booking sidebar */}
        <div className="lg:sticky lg:top-20">
          <BookingForm
            venueId={venue.id}
            venueName={venue.name}
            priceFrom={venue.price_from}
          />
        </div>
      </div>
    </div>
  );
}

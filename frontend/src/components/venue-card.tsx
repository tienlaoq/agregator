import Link from "next/link";
import Image from "next/image";
import { MapPin } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { StarRating } from "@/components/star-rating";
import { VENUE_TYPE_LABELS } from "@/lib/types";
import type { Venue } from "@/lib/types";

interface VenueCardProps {
  venue: Venue;
}

const typeColors: Record<string, string> = {
  banya: "bg-amber-100 text-amber-800",
  sauna: "bg-orange-100 text-orange-800",
  hammam: "bg-teal-100 text-teal-800",
};

export function VenueCard({ venue }: VenueCardProps) {
  return (
    <Link href={`/venues/${venue.slug}`}>
      <Card className="group h-full overflow-hidden transition-shadow hover:shadow-lg">
        <div className="relative aspect-[16/10] bg-gradient-to-br from-amber-200 to-orange-300">
          {venue.image_url ? (
            <Image
              src={venue.image_url}
              alt={venue.name}
              fill
              className="object-cover"
            />
          ) : (
            <div className="flex h-full items-center justify-center">
              <span className="text-4xl">🏛️</span>
            </div>
          )}
          <div className="absolute left-3 top-3">
            <span
              className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
                typeColors[venue.type] ?? "bg-gray-100 text-gray-800"
              }`}
            >
              {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
            </span>
          </div>
        </div>
        <CardContent className="space-y-2 pt-3">
          <h3 className="line-clamp-1 font-semibold transition-colors group-hover:text-primary">
            {venue.name}
          </h3>
          <div className="flex items-center gap-2">
            <StarRating rating={venue.rating} size="sm" />
            <span className="text-xs text-muted-foreground">
              ({venue.review_count})
            </span>
          </div>
          <div className="flex items-center gap-1 text-xs text-muted-foreground">
            <MapPin className="h-3 w-3 shrink-0" />
            <span className="line-clamp-1">{venue.address}</span>
          </div>
          <div className="pt-1">
            <Badge variant="secondary" className="text-xs font-semibold">
              от {venue.price_from.toLocaleString("ru-RU")} ₽/час
            </Badge>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

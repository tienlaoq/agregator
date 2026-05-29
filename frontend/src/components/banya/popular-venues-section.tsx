import { Button } from "@/components/ui/button"
import { ArrowRight } from "lucide-react"
import Link from "next/link"
import type { Venue } from "@/lib/types"
import { VenueCard } from "@/components/banya/venue-card"

/** Server Component — данные приходят через props из page.tsx (SSR). */
export function PopularVenuesSection({ venues }: { venues: Venue[] }) {
  if (venues.length === 0) return null

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
          {venues.map((venue) => (
            <VenueCard key={venue.id} venue={venue} />
          ))}
        </div>
      </div>
    </section>
  )
}

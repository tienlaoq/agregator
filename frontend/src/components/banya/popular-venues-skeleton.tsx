import { Button } from "@/components/ui/button"
import { ArrowRight } from "lucide-react"
import Link from "next/link"

/** Skeleton-заглушка для секции популярных заведений — показывается пока стримится данные. */
export function PopularVenuesSkeleton() {
  return (
    <section className="bg-secondary/30 py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="mb-10 flex flex-col items-center justify-between gap-4 md:flex-row">
          <div>
            <div className="h-9 w-64 animate-pulse rounded-md bg-muted" />
            <div className="mt-2 h-5 w-48 animate-pulse rounded-md bg-muted" />
          </div>
          <Button variant="outline" asChild className="gap-2" disabled>
            <Link href="/venues">
              Смотреть все
              <ArrowRight className="h-4 w-4" />
            </Link>
          </Button>
        </div>

        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="overflow-hidden rounded-xl border border-border bg-card">
              <div className="h-48 animate-pulse bg-muted" />
              <div className="p-4 space-y-3">
                <div className="h-5 w-3/4 animate-pulse rounded bg-muted" />
                <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
                <div className="h-4 w-2/3 animate-pulse rounded bg-muted" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

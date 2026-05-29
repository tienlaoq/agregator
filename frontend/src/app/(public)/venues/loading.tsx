export default function VenuesLoading() {
  return (
    <section className="bg-background py-16 md:py-24">
      <div className="container mx-auto px-4">
        <div className="mb-8 h-10 w-56 animate-pulse rounded-lg bg-muted" />
        <div className="mb-6 flex flex-col gap-4 lg:flex-row">
          <div className="h-11 flex-1 animate-pulse rounded-lg bg-muted" />
          <div className="h-11 w-full animate-pulse rounded-lg bg-muted lg:max-w-[240px]" />
          <div className="flex gap-3">
            {[140, 160, 160].map((w, i) => (
              <div key={i} style={{ width: w }} className="h-11 animate-pulse rounded-lg bg-muted" />
            ))}
          </div>
        </div>
        <div className="mb-10 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="h-80 animate-pulse rounded-xl bg-muted" />
          ))}
        </div>
      </div>
    </section>
  )
}

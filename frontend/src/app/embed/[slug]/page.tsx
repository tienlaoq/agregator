import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ApiError, getVenueBySlug, venueMediaUrl } from "@/lib/api";
import { siteUrl } from "@/lib/seo-site";
import { VENUE_TYPE_LABELS } from "@/lib/types";
import { CalendarCheck, Star } from "lucide-react";

// Виджет не индексируется — это встраиваемый фрагмент, а не самостоятельная
// страница; каноничная карточка живёт на /venues/[slug].
export const metadata: Metadata = {
  robots: { index: false, follow: false },
};

function formatMoney(v: number): string {
  return `${new Intl.NumberFormat("ru-RU").format(Math.round(v))} ₽`;
}

export default async function VenueEmbedPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  let venue: Awaited<ReturnType<typeof getVenueBySlug>>;
  try {
    venue = await getVenueBySlug(slug);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }

  const photos = venue.photos ?? [];
  const cover = photos.find((p) => p.is_cover) ?? photos[0];
  const coverUrl = cover ? venueMediaUrl(cover.url) : null;
  // target="_blank" — бронь/оплата открываются на полном сайте, а не внутри
  // чужого iframe, где логин и платёжка ломаются.
  const bookingUrl = `${siteUrl()}/venues/${slug}?utm_source=embed&utm_medium=widget`;

  return (
    <main className="mx-auto max-w-md p-3">
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        {coverUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={coverUrl}
            alt={venue.name}
            className="h-40 w-full object-cover"
          />
        ) : null}
        <div className="p-4">
          <div className="flex items-start justify-between gap-2">
            <div>
              <div className="text-lg font-semibold text-foreground">
                {venue.name}
              </div>
              <div className="mt-0.5 text-sm text-muted-foreground">
                {venue.city} · {VENUE_TYPE_LABELS[venue.type] ?? venue.type}
              </div>
            </div>
            {venue.review_count > 0 ? (
              <div className="flex shrink-0 items-center gap-1 text-sm font-medium text-foreground">
                <Star className="h-4 w-4 fill-amber-500 text-amber-500" />
                {venue.rating.toFixed(1)}
              </div>
            ) : null}
          </div>

          <div className="mt-3 text-sm text-muted-foreground">
            Парение · от{" "}
            <span className="font-medium text-foreground">
              {formatMoney(venue.price_from)}
            </span>
          </div>

          <a
            href={bookingUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="mt-4 flex w-full items-center justify-center gap-1.5 rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition hover:opacity-90"
          >
            <CalendarCheck className="h-4 w-4" />
            Забронировать онлайн
          </a>
          <p className="mt-2 text-center text-xs text-muted-foreground">
            Свободное время видно сразу · предоплата защищает бронь
          </p>

          <a
            href={siteUrl()}
            target="_blank"
            rel="noopener noreferrer"
            className="mt-3 block text-center text-[11px] text-muted-foreground/70 hover:text-muted-foreground"
          >
            Работает на БаняГид
          </a>
        </div>
      </div>
    </main>
  );
}

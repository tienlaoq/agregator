"use client"

import { Badge } from "@/components/ui/badge"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  VENUE_SOCIAL_PUBLIC_LABELS,
  parseVenueSocialLinks,
  type Venue,
  type VenueSocialLinkKey,
} from "@/lib/types"
import { ChevronDown, ImageIcon, Layers, Sparkles } from "lucide-react"

function formatMoneyRub(n: number): string {
  return `${n.toLocaleString("ru-RU")} ₽`
}

type Props = { venue: Venue }

/** Полная карточка заведения для экрана модерации (залы, фото, услуги, удобства). */
export function AdminVenueFullCard({ venue }: Props) {
  const photos = venue.photos ?? []
  const halls = venue.halls ?? []
  const services = venue.services ?? []
  const amenities = venue.amenities ?? []
  const social = parseVenueSocialLinks(venue.social_links)

  const hasExtra =
    photos.length > 0 ||
    halls.length > 0 ||
    services.length > 0 ||
    amenities.length > 0 ||
    (venue.working_hours && venue.working_hours.trim() !== "") ||
    venue.capacity != null ||
    venue.price_from != null ||
    venue.latitude != null ||
    venue.longitude != null ||
    Object.values(social).some((u) => u.trim() !== "")

  if (!hasExtra) {
    return (
      <p className="text-sm text-muted-foreground">
        Партнёр ещё не добавил фото, залы и услуги — только базовые поля и данные для проверки
        владельца.
      </p>
    )
  }

  return (
    <Collapsible defaultOpen className="group rounded-lg border border-border bg-muted/30">
      <CollapsibleTrigger className="flex w-full items-center justify-between gap-2 px-4 py-3 text-left text-sm font-medium hover:bg-muted/50">
        <span className="flex items-center gap-2">
          <Layers className="h-4 w-4 shrink-0 text-muted-foreground" />
          Полная карточка: залы, фото, услуги
        </span>
        <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="space-y-4 border-t border-border px-4 pb-4 pt-3 text-sm">
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-muted-foreground">
            {venue.working_hours ? (
              <span>
                <span className="font-medium text-foreground">Режим: </span>
                {venue.working_hours}
              </span>
            ) : null}
            {venue.capacity != null && venue.capacity > 0 ? (
              <span>
                <span className="font-medium text-foreground">Вместимость: </span>
                до {venue.capacity} чел.
              </span>
            ) : null}
            {venue.price_from != null && venue.price_from > 0 ? (
              <span>
                <span className="font-medium text-foreground">Цена от: </span>
                {formatMoneyRub(venue.price_from)}
              </span>
            ) : null}
            {venue.latitude != null && venue.longitude != null ? (
              <span className="font-mono text-xs">
                <span className="font-medium text-foreground">Коорд.: </span>
                {venue.latitude.toFixed(5)}, {venue.longitude.toFixed(5)}
              </span>
            ) : null}
          </div>

          {amenities.length > 0 ? (
            <div>
              <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
                <Sparkles className="h-4 w-4 text-muted-foreground" />
                Удобства заведения
              </div>
              <div className="flex flex-wrap gap-1.5">
                {amenities.map((a) => (
                  <Badge key={a} variant="secondary" className="font-normal">
                    {a}
                  </Badge>
                ))}
              </div>
            </div>
          ) : null}

          {(Object.keys(VENUE_SOCIAL_PUBLIC_LABELS) as VenueSocialLinkKey[]).some(
            (k) => social[k]?.trim(),
          ) ? (
            <div>
              <p className="mb-2 font-medium text-foreground">Ссылки</p>
              <ul className="space-y-1">
                {(Object.keys(VENUE_SOCIAL_PUBLIC_LABELS) as VenueSocialLinkKey[]).map((k) => {
                  const url = social[k]?.trim()
                  if (!url) return null
                  return (
                    <li key={k}>
                      <a
                        href={url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary hover:underline"
                      >
                        {VENUE_SOCIAL_PUBLIC_LABELS[k]}
                      </a>
                    </li>
                  )
                })}
              </ul>
            </div>
          ) : null}

          {photos.length > 0 ? (
            <div>
              <div className="mb-2 flex items-center gap-2 font-medium text-foreground">
                <ImageIcon className="h-4 w-4 text-muted-foreground" />
                Фото карточки ({photos.length})
              </div>
              <div className="flex flex-wrap gap-2">
                {photos.map((p) => (
                  <a
                    key={p.id}
                    href={p.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="block overflow-hidden rounded-md border border-border bg-background"
                  >
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={p.url}
                      alt=""
                      className="h-28 w-40 object-cover"
                    />
                  </a>
                ))}
              </div>
            </div>
          ) : null}

          {halls.length > 0 ? (
            <div>
              <p className="mb-2 font-medium text-foreground">Залы ({halls.length})</p>
              <div className="space-y-3">
                {halls.map((h) => (
                  <div
                    key={h.id}
                    className="rounded-md border border-border bg-background p-3"
                  >
                    <div className="flex flex-wrap items-baseline justify-between gap-2">
                      <span className="font-medium">{h.name || "Без названия"}</span>
                      <span className="text-muted-foreground">
                        от {formatMoneyRub(h.price_from ?? 0)} · до {h.capacity ?? "—"} чел.
                      </span>
                    </div>
                    {(h.amenities ?? []).length > 0 ? (
                      <div className="mt-2 flex flex-wrap gap-1">
                        {(h.amenities ?? []).map((a) => (
                          <Badge key={a} variant="outline" className="text-xs font-normal">
                            {a}
                          </Badge>
                        ))}
                      </div>
                    ) : null}
                    {(h.photos ?? []).length > 0 ? (
                      <div className="mt-2 flex flex-wrap gap-2">
                        {(h.photos ?? []).map((hp) => (
                          <a
                            key={hp.id}
                            href={hp.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="block overflow-hidden rounded border border-border"
                          >
                            {/* eslint-disable-next-line @next/next/no-img-element */}
                            <img
                              src={hp.url}
                              alt=""
                              className="h-20 w-28 object-cover"
                            />
                          </a>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {services.length > 0 ? (
            <div>
              <p className="mb-2 font-medium text-foreground">Услуги ({services.length})</p>
              <ul className="space-y-2 rounded-md border border-border bg-background p-3">
                {services.map((s) => (
                  <li key={s.id} className="border-b border-border pb-2 last:border-0 last:pb-0">
                    <div className="flex flex-wrap justify-between gap-2 font-medium">
                      <span>{s.name}</span>
                      <span className="text-muted-foreground">
                        {formatMoneyRub(Number(s.price) || 0)}
                        {s.duration_min != null || s.duration_minutes != null
                          ? ` · ${s.duration_min ?? s.duration_minutes} мин`
                          : ""}
                      </span>
                    </div>
                    {s.description ? (
                      <p className="mt-1 text-muted-foreground">{s.description}</p>
                    ) : null}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

import Link from "next/link"
import { Card, CardContent } from "@/components/ui/card"
import {
  MapPin,
  Award,
  Navigation,
  ImageIcon,
  Images,
  ShieldCheck,
  ArrowRight,
} from "lucide-react"
import { FramedImg } from "@/components/banya/framed-image"
import { masterCardImageSrc, masterCardPriceLabel } from "@/lib/api"
import type { MasterProfile } from "@/lib/types"

const WORK_FORMAT_LABELS: Record<string, string> = {
  mobile: "Выезд к клиенту",
  venue: "В бане / у заведения",
  both: "И выезд, и в бане",
}

interface MasterCardProps {
  master: MasterProfile
}

/**
 * Карточка пар-мастера для каталога. Дизайн совпадает с VenueCard:
 * фото с бейджами → имя → город → быстрые факты (опыт, выезд) →
 * краткое описание → специализации → цена + CTA.
 */
export function MasterCard({ master }: MasterCardProps) {
  const cardImg = masterCardImageSrc(master)
  const photoCount = master.photos?.length ?? 0
  const verified = master.status === "active"
  const priceLine = masterCardPriceLabel(master)
  const wfLabel = WORK_FORMAT_LABELS[master.work_format] ?? master.work_format
  const specs = master.specializations ?? []
  const mobile = master.work_format === "mobile" || master.work_format === "both"

  return (
    <Link href={`/masters/${master.slug}`} className="group block h-full">
      <Card className="flex h-full flex-col overflow-hidden border-border bg-card transition-all hover:shadow-xl">
        <div className="relative aspect-[4/3] overflow-hidden bg-muted flex items-center justify-center">
          {cardImg ? (
            <FramedImg
              src={cardImg}
              alt={master.display_name}
              className="transition-transform duration-300 group-hover:scale-105"
            />
          ) : (
            <div className="flex flex-col items-center gap-1 text-muted-foreground/40">
              <ImageIcon className="h-8 w-8" />
              <span className="text-xs">Нет фото</span>
            </div>
          )}

          <span className="absolute left-3 top-3 inline-flex items-center gap-1.5 rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary ring-1 ring-inset ring-primary/20 backdrop-blur-sm">
            {wfLabel}
          </span>

          {verified && (
            <span className="absolute right-3 top-3 inline-flex items-center gap-1.5 rounded-full bg-emerald-50/90 px-2.5 py-1 text-xs font-medium text-emerald-700 backdrop-blur-sm">
              <ShieldCheck className="h-3.5 w-3.5" />
              Проверено
            </span>
          )}

          {photoCount > 1 && (
            <span className="absolute left-3 bottom-3 inline-flex items-center gap-1.5 rounded-md bg-black/55 px-2 py-1 text-xs text-white">
              <Images className="h-3.5 w-3.5" />
              {photoCount}
            </span>
          )}
        </div>

        <CardContent className="flex flex-1 flex-col p-5">
          <h3 className="mb-1.5 line-clamp-1 text-lg font-semibold text-card-foreground">
            {master.display_name}
          </h3>

          <div className="mb-1 flex items-center gap-1.5 text-sm text-muted-foreground">
            <MapPin className="h-4 w-4 shrink-0" />
            <span className="line-clamp-1">{master.city}</span>
          </div>

          {(master.experience_years > 0 || (mobile && master.travel_radius_km > 0)) && (
            <div className="mb-2.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
              {master.experience_years > 0 && (
                <span className="inline-flex items-center gap-1.5">
                  <Award className="h-4 w-4 shrink-0" />
                  Опыт {master.experience_years} лет
                </span>
              )}
              {mobile && master.travel_radius_km > 0 && (
                <span className="inline-flex items-center gap-1.5">
                  <Navigation className="h-4 w-4 shrink-0" />
                  Выезд до {master.travel_radius_km} км
                </span>
              )}
            </div>
          )}

          {master.bio && (
            <p className="mb-3 line-clamp-2 text-sm text-muted-foreground">{master.bio}</p>
          )}

          {specs.length > 0 && (
            <div className="mb-4 flex flex-wrap gap-1.5">
              {specs.slice(0, 3).map((s) => (
                <span
                  key={s}
                  className="max-w-[10rem] truncate rounded-full bg-secondary px-2.5 py-1 text-xs text-secondary-foreground"
                >
                  {s}
                </span>
              ))}
              {specs.length > 3 && (
                <span className="rounded-full bg-muted px-2.5 py-1 text-xs text-muted-foreground">
                  +{specs.length - 3}
                </span>
              )}
            </div>
          )}

          <div className="mt-auto flex items-end justify-between gap-3 border-t border-border pt-4">
            <div className="leading-tight">
              {priceLine ? (
                <div className="text-lg font-bold text-primary">{priceLine}</div>
              ) : (
                <div className="text-sm font-medium text-muted-foreground">Цена по запросу</div>
              )}
            </div>
            <span className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-colors group-hover:bg-primary/90">
              Записаться
              <ArrowRight className="h-4 w-4" />
            </span>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}

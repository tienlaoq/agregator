import type { VenueGuest } from "@/lib/types";

/** Подписи сегментов гостя (Customer 360). */
export const GUEST_SEGMENT_LABELS: Record<string, string> = {
  new: "Новый",
  regular: "Постоянный",
  vip: "VIP",
  at_risk: "В зоне риска",
};

export function guestSegmentLabel(s: string): string {
  return GUEST_SEGMENT_LABELS[s] ?? s;
}

type BadgeVariant = "default" | "secondary" | "outline" | "destructive";

/** Цвет бейджа сегмента: VIP выделяем, «в зоне риска» — тревожно. */
export function guestSegmentVariant(s: string): BadgeVariant {
  switch (s) {
    case "vip":
      return "default";
    case "at_risk":
      return "destructive";
    case "regular":
      return "secondary";
    default:
      return "outline";
  }
}

/** Имя из профиля, иначе email, иначе обезличенная отметка. */
export function guestDisplayName(
  g: Pick<VenueGuest, "user_name" | "user_email">,
): string {
  return g.user_name?.trim() || g.user_email?.trim() || "Гость без профиля";
}

export function formatMoney(v: number | undefined): string {
  return `${(v ?? 0).toLocaleString("ru-RU")} ₽`;
}

/** ISO-дата → ДД.ММ.ГГГГ; пусто → «—». */
export function formatGuestDate(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  });
}

import type { MasterClient } from "@/lib/types";

/** Подписи сегментов клиента мастера (аналог гостевых, без VIP). */
export const CLIENT_SEGMENT_LABELS: Record<string, string> = {
  new: "Новый",
  regular: "Постоянный",
  at_risk: "Спящий",
};

export function clientSegmentLabel(s: string): string {
  return CLIENT_SEGMENT_LABELS[s] ?? s;
}

type BadgeVariant = "default" | "secondary" | "outline" | "destructive";

/** Цвет бейджа сегмента: спящий — тревожно, постоянный — акцент. */
export function clientSegmentVariant(s: string): BadgeVariant {
  switch (s) {
    case "at_risk":
      return "destructive";
    case "regular":
      return "secondary";
    default:
      return "outline";
  }
}

/** Имя из user-service, иначе обезличенная отметка. */
export function clientDisplayName(
  c: Pick<MasterClient, "user_name">,
): string {
  return c.user_name?.trim() || "Клиент без профиля";
}

/** Копейки → «1 234 ₽». LTV клиента хранится в копейках. */
export function formatKopecks(kopecks: number | undefined): string {
  return `${Math.round((kopecks ?? 0) / 100).toLocaleString("ru-RU")} ₽`;
}

/** ISO-дата → ДД.ММ.ГГГГ; пусто → «—». */
export function formatClientDate(iso: string | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  });
}

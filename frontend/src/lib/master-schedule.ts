import type { MasterSlotBlock } from "@/lib/types";

/** Часы работы одного дня недели. Индекс 0 = Пн … 6 = Вс. */
export interface WorkingHoursDay {
  on: boolean;
  from: string; // "HH:MM"
  to: string; // "HH:MM"
}

export const DOW_LABELS = ["Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"];

/** Дефолт: будни 10:00–21:00, выходные 12:00–20:00, воскресенье — выходной. */
export const DEFAULT_WORKING_HOURS: WorkingHoursDay[] = [
  { on: true, from: "10:00", to: "21:00" },
  { on: true, from: "10:00", to: "21:00" },
  { on: true, from: "10:00", to: "21:00" },
  { on: true, from: "10:00", to: "21:00" },
  { on: true, from: "10:00", to: "21:00" },
  { on: true, from: "12:00", to: "20:00" },
  { on: false, from: "12:00", to: "20:00" },
];

const HHMM = /^\d{2}:\d{2}$/;

function isValidDay(v: unknown): v is WorkingHoursDay {
  if (!v || typeof v !== "object") return false;
  const d = v as Record<string, unknown>;
  return (
    typeof d.on === "boolean" &&
    typeof d.from === "string" &&
    HHMM.test(d.from) &&
    typeof d.to === "string" &&
    HHMM.test(d.to)
  );
}

/** Читает часы работы из availability_json.hours; при отсутствии/невалидности — дефолт. */
export function parseWorkingHours(raw: string | undefined): WorkingHoursDay[] {
  if (!raw?.trim() || raw.trim() === "{}") return clone(DEFAULT_WORKING_HOURS);
  try {
    const o = JSON.parse(raw) as { hours?: unknown };
    if (
      o &&
      typeof o === "object" &&
      Array.isArray(o.hours) &&
      o.hours.length === 7 &&
      o.hours.every(isValidDay)
    ) {
      return o.hours as WorkingHoursDay[];
    }
  } catch {
    return clone(DEFAULT_WORKING_HOURS);
  }
  return clone(DEFAULT_WORKING_HOURS);
}

/**
 * Кладёт часы в availability_json под ключ `hours`, сохраняя прочие ключи
 * (в частности человекочитаемую заметку `note`, которую редактирует профиль).
 */
export function mergeHoursIntoAvailability(
  prevRaw: string | undefined,
  hours: WorkingHoursDay[],
): string {
  let base: Record<string, unknown> = {};
  const raw = prevRaw?.trim() ?? "";
  if (raw && raw !== "{}") {
    try {
      const o = JSON.parse(raw);
      if (o && typeof o === "object" && !Array.isArray(o)) {
        base = { ...(o as Record<string, unknown>) };
      }
    } catch {
      base = {};
    }
  }
  base.hours = hours;
  return JSON.stringify(base);
}

/** ISO YYYY-MM-DD в локальном времени (без UTC-сдвига toISOString). */
export function toLocalISODate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

/** Индекс дня недели Пн=0..Вс=6 из JS Date (getDay(): Вс=0..Сб=6). */
export function dowIndex(d: Date): number {
  return (d.getDay() + 6) % 7;
}

/** Есть ли блокировка на эту дату (любая — на весь день или интервал). */
export function hasBlockOn(blocks: MasterSlotBlock[], iso: string): boolean {
  return blocks.some((b) => b.date === iso);
}

/** Подпись интервала блокировки: «весь день» или «14:00–16:00». */
export function blockIntervalLabel(b: MasterSlotBlock): string {
  if (!b.time_from && !b.time_to) return "весь день";
  return `${b.time_from}–${b.time_to}`;
}

/** ISO-дата → «5 июля»; пусто → «—». */
export function formatBlockDate(iso: string): string {
  const d = new Date(iso + "T00:00:00");
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString("ru-RU", { day: "numeric", month: "long" });
}

function clone(days: WorkingHoursDay[]): WorkingHoursDay[] {
  return days.map((d) => ({ ...d }));
}

/** Утилиты для выбора интервала бронирования (как на карточке заведения). */

export function hhmmToMinutes(s: string): number | null {
  const m = s.match(/^(\d{1,2}):(\d{2})$/)
  if (!m) return null
  const h = parseInt(m[1], 10)
  const min = parseInt(m[2], 10)
  if (
    !Number.isFinite(h) ||
    !Number.isFinite(min) ||
    min < 0 ||
    min > 59 ||
    h < 0 ||
    h > 23
  ) {
    return null
  }
  return h * 60 + min
}

/** Сетка «время начала»: 10:00–22:00 с шагом 30 мин (совпадает с api-gateway). */
export function thirtyMinuteStartGridFrom10To22(): string[] {
  const out: string[] = []
  for (let m = 10 * 60; m <= 22 * 60; m += 30) {
    out.push(
      `${String(Math.floor(m / 60)).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`,
    )
  }
  return out
}

/** Окончание визита в тот же день: от +30 мин до +12 ч, шаг 30 минут. */
export function endTimeOptionsThirtyMinutes(startHHMM: string): string[] {
  const startTotal = hhmmToMinutes(startHHMM)
  if (startTotal == null) return []
  const out: string[] = []
  for (let delta = 30; delta <= 720; delta += 30) {
    const total = startTotal + delta
    if (total >= 24 * 60) break
    const eh = Math.floor(total / 60)
    const em = total % 60
    out.push(`${String(eh).padStart(2, "0")}:${String(em).padStart(2, "0")}`)
  }
  return out
}

export function defaultEndTimeForDuration(
  startHHMM: string,
  preferredDurMin: number,
): string {
  const opts = endTimeOptionsThirtyMinutes(startHHMM)
  if (opts.length === 0) return ""
  const startM = hhmmToMinutes(startHHMM)
  if (startM == null) return opts[0] ?? ""
  const need = Math.max(30, Math.min(720, preferredDurMin))
  for (const o of opts) {
    const om = hhmmToMinutes(o)
    if (om != null && om - startM >= need) return o
  }
  return opts[opts.length - 1] ?? ""
}

/** Длина интервала в минутах (тот же день, to > from). */
export function slotLengthMinutes(from: string, to: string): number | null {
  const [fh, fm] = from.split(":").map((x) => parseInt(x, 10))
  const [th, tm] = to.split(":").map((x) => parseInt(x, 10))
  if (![fh, fm, th, tm].every((n) => Number.isFinite(n))) return null
  const v = th * 60 + tm - (fh * 60 + fm)
  return v > 0 ? v : null
}

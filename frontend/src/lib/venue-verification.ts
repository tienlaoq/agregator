/** Цифры ИНН/ОГРН без форматирования */
export function digitsOnly(s: string): string {
  return s.replace(/\D/g, "");
}

export function isValidRUInn(s: string): boolean {
  const d = digitsOnly(s);
  return d.length === 10 || d.length === 12;
}

/** Пусто — ок; иначе 13 (ОГРН) или 15 (ОГРНИП) */
export function isValidOgrnOptional(s: string): boolean {
  const t = s.trim();
  if (!t) return true;
  const d = digitsOnly(t);
  return d.length === 13 || d.length === 15;
}

export function isValidListingURL(s: string): boolean {
  const t = s.trim();
  if (!t) return false;
  try {
    const u = new URL(t);
    if (u.protocol !== "http:" && u.protocol !== "https:") return false;
    const host = u.hostname.toLowerCase();
    if (!host || host === "localhost" || host.startsWith("127.")) return false;
    return true;
  } catch {
    return false;
  }
}

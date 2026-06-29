import type { ChatThread } from "@/lib/types";

/**
 * Заголовок собеседника в списке чатов и в шапке мини-чата.
 * Сервер заполняет peer_display_name для venue_booking и master_booking;
 * без него — короткий fallback (локальная подстраховка до ответа API).
 */
export function chatThreadDisplayLabel(t: ChatThread): string {
  const peer = t.peer_display_name?.trim();
  if (peer) return peer;
  const short = (t.ref_id || "").slice(0, 8);
  if (t.kind === "master_booking") {
    return short ? `Пар-мастер · ${short}` : "Пар-мастер";
  }
  return short ? `Заведение · ${short}` : "Заведение";
}

/** Подпись под именем собеседника: тип диалога. */
export function chatThreadKindLabel(t: ChatThread): string {
  return t.kind === "master_booking" ? "Пар-мастер" : "Заведение";
}

/** Первая буква имени собеседника для аватара-заглушки. */
export function chatThreadInitial(t: ChatThread): string {
  const label = chatThreadDisplayLabel(t).trim();
  const first = [...label].find((ch) => /\p{L}|\p{N}/u.test(ch));
  return (first ?? "?").toUpperCase();
}

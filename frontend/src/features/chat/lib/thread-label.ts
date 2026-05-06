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

/** Строка заголовка внутри карточки переписки («Чат: …»). */
export function chatThreadPanelTitle(t: ChatThread): string {
  return `Чат: ${chatThreadDisplayLabel(t)}`;
}

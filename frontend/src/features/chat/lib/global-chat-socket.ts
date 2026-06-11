"use client";

import { issueChatWsTicket } from "@/lib/api";
import { CHAT_V2_WS_PATH } from "@/lib/chat-paths";
import type { ChatEventEnvelope } from "@/features/chat/types/chat";

type Listener = (evt: ChatEventEnvelope) => void;

const listeners = new Set<Listener>();

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let pingTimer: ReturnType<typeof setInterval> | null = null;
let attempt = 0;
let activeToken: string | null = null;
/** Одноразовый билет из Redis. Обязателен — access_token в query не используется. */
let activeWsTicket: string | null = null;
let stopped = false;

/**
 * Строит URL для WebSocket-соединения.
 * Требует ws_ticket — передача access_token в query string намеренно убрана:
 * токен попадает в access-логи сервера и proxy-серверов.
 * Возвращает null, если билет ещё не получен.
 */
function buildWsUrl(): string | null {
  if (!activeWsTicket) return null;
  const base = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  const u = new URL(base);
  u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
  u.pathname = CHAT_V2_WS_PATH;
  u.searchParams.set("ws_ticket", activeWsTicket);
  return u.toString();
}

function dispatch(evt: ChatEventEnvelope): void {
  for (const l of listeners) {
    try {
      l(evt);
    } catch {
      // ignore subscriber errors
    }
  }
}

/** Подписка на все события чата (один общий WebSocket на пользователя). */
export function subscribeChatEvents(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * Отправляет сообщение через WebSocket (команда `send_message`).
 *
 * N12: если в будущем `useChatThread.send` переключится с HTTP на WS,
 * необходимо передавать `client_msg_id` — иначе сервер сгенерирует случайный
 * UUID и повторная отправка при реконнекте создаст дубликат.
 *
 * Используйте `crypto.randomUUID()` на стороне вызывающего кода и сохраняйте
 * его до получения подтверждения от сервера (событие `chat.message.created`
 * с тем же `id`), после чего UUID можно выбросить.
 *
 * @returns `true` — сообщение передано в буфер сокета, `false` — сокет не готов.
 */
export function sendWsChatMessage(params: {
  threadId: string;
  text: string;
  clientMsgId: string;
}): boolean {
  if (ws?.readyState !== WebSocket.OPEN) return false;
  ws.send(
    JSON.stringify({
      type: "send_message",
      thread_id: params.threadId,
      text: params.text,
      client_msg_id: params.clientMsgId,
    }),
  );
  return true;
}

function clearTimers(): void {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (pingTimer) {
    clearInterval(pingTimer);
    pingTimer = null;
  }
}

function connect(): void {
  // Не подключаемся без билета — access_token в query string не допускается.
  if (!activeToken || !activeWsTicket || stopped) return;
  const url = buildWsUrl();
  if (!url) return;
  ws = new WebSocket(url);
  ws.onopen = () => {
    attempt = 0;
    ws?.send(JSON.stringify({ type: "ping", event: "chat.ping" }));
    if (pingTimer) clearInterval(pingTimer);
    pingTimer = setInterval(() => {
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "ping", event: "chat.ping" }));
      }
    }, 30000);
  };
  ws.onmessage = (e) => {
    try {
      const data = JSON.parse(String(e.data)) as ChatEventEnvelope;
      dispatch(data);
    } catch {
      // ignore malformed frames
    }
  };
  ws.onclose = () => {
    clearTimers();
    pingTimer = null;
    ws = null;
    if (stopped || !activeToken) return;
    const backoffMs = Math.min(30000, 1000 * Math.pow(2, Math.min(attempt, 5)));
    attempt++;
    reconnectTimer = setTimeout(() => void reconnectWithFreshTicket(), backoffMs);
  };
  ws.onerror = () => ws?.close();
}

/**
 * Реконнект всегда идёт со свежим билетом: старый погашен сервером при
 * предыдущем апгрейде (Redis GetDel — билет одноразовый), поэтому повторное
 * подключение со старым значением гарантированно получает 401.
 */
async function reconnectWithFreshTicket(): Promise<void> {
  if (stopped || !activeToken) return;
  const res = await issueChatWsTicket();
  if (stopped || !activeToken) return;
  if (!res?.ticket) {
    const backoffMs = Math.min(30000, 1000 * Math.pow(2, Math.min(attempt, 5)));
    attempt++;
    reconnectTimer = setTimeout(() => void reconnectWithFreshTicket(), backoffMs);
    return;
  }
  activeWsTicket = res.ticket;
  connect();
}

/**
 * Подключить сокет при наличии токена и ws_ticket; `null` — отключить (выход из аккаунта).
 *
 * Соединение устанавливается только при наличии обоих аргументов:
 *   - `token` — признак аутентификации (пользователь залогинен)
 *   - `wsTicket` — одноразовый билет из POST `/v2/chat/ws-ticket` (хранится в Redis)
 *
 * Если `wsTicket` не передан или равен null, сокет ждёт следующего вызова с билетом.
 * access_token намеренно не передаётся в query string — он попадает в access-логи.
 */
export function setChatSocketToken(token: string | null, wsTicket?: string | null): void {
  const nextTicket = wsTicket ?? null;
  if (!token) {
    stopped = true;
    activeToken = null;
    activeWsTicket = null;
    clearTimers();
    ws?.close();
    ws = null;
    return;
  }
  if (
    token === activeToken &&
    nextTicket === activeWsTicket &&
    ws?.readyState === WebSocket.OPEN
  ) {
    return;
  }
  clearTimers();
  ws?.close();
  ws = null;
  activeToken = token;
  activeWsTicket = nextTicket;
  stopped = false;
  attempt = 0;
  connect();
}

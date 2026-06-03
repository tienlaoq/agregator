"use client";

import { NOTIFICATION_V2_WS_PATH } from "@/lib/notification-paths";
import type { NotificationEventEnvelope } from "@/features/notifications/types/notification";

type Listener = (evt: NotificationEventEnvelope) => void;

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
 * Строит URL для WebSocket уведомлений.
 * Требует ws_ticket — access_token в query string намеренно убран (он попадает
 * в access-логи сервера и proxy). Возвращает null, если билета ещё нет.
 */
function buildWsUrl(): string | null {
  if (!activeWsTicket) return null;
  const base = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  const u = new URL(base);
  u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
  u.pathname = NOTIFICATION_V2_WS_PATH;
  u.searchParams.set("ws_ticket", activeWsTicket);
  return u.toString();
}

function dispatch(evt: NotificationEventEnvelope): void {
  for (const l of listeners) {
    try {
      l(evt);
    } catch {
      // ignore subscriber errors
    }
  }
}

/** Подписка на события колокольчика (один общий WebSocket на пользователя). */
export function subscribeNotificationEvents(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
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
    ws?.send(JSON.stringify({ type: "ping", event: "notification.ping" }));
    if (pingTimer) clearInterval(pingTimer);
    pingTimer = setInterval(() => {
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "ping", event: "notification.ping" }));
      }
    }, 30000);
  };
  ws.onmessage = (e) => {
    try {
      const data = JSON.parse(String(e.data)) as NotificationEventEnvelope;
      dispatch(data);
    } catch {
      // ignore malformed frames
    }
  };
  ws.onclose = () => {
    clearTimers();
    pingTimer = null;
    ws = null;
    if (stopped || !activeToken || !activeWsTicket) return;
    const backoffMs = Math.min(30000, 1000 * Math.pow(2, Math.min(attempt, 5)));
    attempt++;
    reconnectTimer = setTimeout(connect, backoffMs);
  };
  ws.onerror = () => ws?.close();
}

/**
 * Подключить сокет при наличии токена и ws_ticket; `null` — отключить (выход из аккаунта).
 * Соединение устанавливается только при наличии обоих аргументов; access_token
 * намеренно не передаётся в query string.
 */
export function setNotificationSocketToken(
  token: string | null,
  wsTicket?: string | null,
): void {
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

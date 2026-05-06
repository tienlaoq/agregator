"use client";

import { CHAT_V2_WS_PATH } from "@/lib/chat-paths";
import type { ChatEventEnvelope } from "@/features/chat/types/chat";

type Listener = (evt: ChatEventEnvelope) => void;

const listeners = new Set<Listener>();

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let pingTimer: ReturnType<typeof setInterval> | null = null;
let attempt = 0;
let activeToken: string | null = null;
/** Одноразовый билет из Redis (предпочтительнее, чем долгий access_token в query). */
let activeWsTicket: string | null = null;
let stopped = false;

function buildWsUrl(token: string): string {
  const base = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  const u = new URL(base);
  u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
  u.pathname = CHAT_V2_WS_PATH;
  if (activeWsTicket) {
    u.searchParams.set("ws_ticket", activeWsTicket);
  } else {
    u.searchParams.set("access_token", token);
  }
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
  if (!activeToken || stopped) return;
  const tok = activeToken;
  ws = new WebSocket(buildWsUrl(tok));
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
    reconnectTimer = setTimeout(connect, backoffMs);
  };
  ws.onerror = () => ws?.close();
}

/**
 * Подключить сокет при наличии токена; `null` — отключить (выход из аккаунта).
 * Передайте `wsTicket` после POST `/v2/chat/ws-ticket`, чтобы не тащить access_token в query.
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

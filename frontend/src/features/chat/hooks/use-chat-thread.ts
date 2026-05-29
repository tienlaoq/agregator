"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ensureChatThreadV2,
  listChatMessagesV2,
  markChatThreadReadV2,
  sendChatMessageV2,
} from "@/features/chat/api/chat-api";
import type { ChatEventEnvelope, ChatKind } from "@/features/chat/types/chat";
import type { ChatMessage, ChatThread } from "@/lib/types";
import { formatApiErrorMessage } from "@/lib/api";
import { subscribeChatEvents } from "@/features/chat/lib/global-chat-socket";

const MARK_READ_DEBOUNCE_MS = 400;

export function useChatThread(kind: ChatKind, refId: string, enabled: boolean) {
  const [thread, setThread] = useState<ChatThread | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const markReadTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const scheduleMarkRead = useCallback((threadId: string) => {
    if (markReadTimerRef.current) clearTimeout(markReadTimerRef.current);
    markReadTimerRef.current = setTimeout(() => {
      markReadTimerRef.current = null;
      void markChatThreadReadV2(threadId).catch(() => {});
    }, MARK_READ_DEBOUNCE_MS);
  }, []);

  useEffect(() => {
    if (!enabled || !refId) return;
    let alive = true;
    (async () => {
      setBusy(true);
      setError("");
      try {
        const tr = await ensureChatThreadV2({ kind, ref_id: refId });
        if (!alive) return;
        setThread(tr.thread);
        const hist = await listChatMessagesV2(tr.thread.id, { limit: 200, offset: 0 });
        if (!alive) return;
        setMessages(hist.messages ?? []);
        const mr = await markChatThreadReadV2(tr.thread.id).catch(() => null);
        if (!alive) return;
        if (mr?.thread) setThread(mr.thread);
      } catch (e) {
        if (alive) setError(formatApiErrorMessage(e, "Не удалось открыть чат."));
      } finally {
        if (alive) setBusy(false);
      }
    })();
    return () => {
      alive = false;
      if (markReadTimerRef.current) {
        clearTimeout(markReadTimerRef.current);
        markReadTimerRef.current = null;
      }
    };
  }, [enabled, kind, refId]);

  const onEvent = useCallback(
    (evt: ChatEventEnvelope) => {
      const eventName = evt.event || evt.type || "";
      if (!thread?.id) return;
      if ((eventName === "chat.message.created" || eventName === "message_new") && evt.message?.thread_id === thread.id) {
        setMessages((prev) => (prev.some((x) => x.id === evt.message!.id) ? prev : [...prev, evt.message!]));
        scheduleMarkRead(thread.id);
      }
      if ((eventName === "chat.thread.read_updated" || eventName === "read_updated") && evt.thread?.id === thread.id) {
        setThread(evt.thread);
      }
    },
    [thread?.id, scheduleMarkRead],
  );

  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  useEffect(() => {
    if (!enabled || !refId) return;
    return subscribeChatEvents((evt) => onEventRef.current(evt));
  }, [enabled, refId]);

  // N12: отправка идёт через HTTP (sendChatMessageV2), а не через WS.
  // Это сделано намеренно: HTTP даёт явный ответ (ack) с финальным message.id,
  // тогда как WS-send требует сопоставления client_msg_id → server message.id
  // через входящие события, что усложняет optimistic-update логику.
  //
  // Если в будущем понадобится переключиться на WS-send (меньше latency,
  // меньше HTTP-соединений), используйте sendWsChatMessage из global-chat-socket.ts.
  // Он уже принимает client_msg_id — без него повторная отправка при реконнекте
  // создаст дубликат (бэкенд дедублицирует только по client_msg_id).
  const send = useCallback(
    async (text: string) => {
      if (!thread?.id || !text.trim()) return;
      setBusy(true);
      setError("");
      const clientMsgId = typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : "";
      try {
        const sent = await sendChatMessageV2(thread.id, text.trim(), clientMsgId);
        setMessages((prev) => (prev.some((x) => x.id === sent.message.id) ? prev : [...prev, sent.message]));
        if (sent.thread) setThread(sent.thread);
      } catch (e) {
        setError(formatApiErrorMessage(e, "Не удалось отправить сообщение."));
      } finally {
        setBusy(false);
      }
    },
    [thread?.id],
  );

  return useMemo(() => ({ thread, messages, error, busy, send }), [thread, messages, error, busy, send]);
}


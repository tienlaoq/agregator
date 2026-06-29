"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { SendHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth";
import type { ChatKind } from "@/features/chat/types/chat";
import { useChatThread } from "@/features/chat/hooks/use-chat-thread";
import {
  getOutgoingDeliveryState,
  sortMessagesForTimeline,
  type OutgoingDeliveryState,
} from "@/features/chat/lib/read-receipts";
import { dayKey, formatDaySeparator, formatMessageTime } from "@/features/chat/lib/format-time";

type Props = {
  kind: ChatKind;
  refId: string;
  /** Доп. классы корня — виджет задаёт высоту через flex-1/min-h-0. */
  className?: string;
};

function sameChatUser(a: string | undefined, b: string | undefined): boolean {
  if (!a?.trim() || !b?.trim()) return false;
  return a.trim().toLowerCase() === b.trim().toLowerCase();
}

function DeliveryTicks({ state }: { state: OutgoingDeliveryState }) {
  const isRead = state === "read";
  const marks = state === "sent" ? "✓" : "✓✓";
  const label = state === "read" ? "Прочитано" : state === "unread" ? "Не прочитано" : "Отправлено";
  return (
    <span
      className={cn(
        "text-[11px] leading-none tracking-[-0.18em]",
        isRead ? "text-sky-500" : "text-muted-foreground/55",
      )}
      aria-label={label}
    >
      {marks}
    </span>
  );
}

export function ChatPanel({ kind, refId, className }: Props) {
  const token = useAuthStore((s) => s.token);
  const userId = useAuthStore((s) => s.user?.id);
  const [text, setText] = useState("");
  const chat = useChatThread(kind, refId, Boolean(token));

  const scrollRef = useRef<HTMLDivElement>(null);
  const orderedMessages = useMemo(() => sortMessagesForTimeline(chat.messages), [chat.messages]);
  const sending = chat.busy;

  // Автопрокрутка к последнему сообщению при загрузке и появлении новых.
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [orderedMessages.length, refId]);

  const onSend = async () => {
    const value = text.trim();
    if (!value || sending) return;
    setText("");
    await chat.send(value);
  };

  return (
    <div className={cn("flex min-h-0 flex-col", className)}>
      {chat.error ? (
        <p className="shrink-0 border-b border-destructive/15 bg-destructive/5 px-3 py-2 text-[13px] text-destructive">
          {chat.error}
        </p>
      ) : null}

      <div
        ref={scrollRef}
        className="flex-1 min-h-0 space-y-1 overflow-y-auto bg-muted/30 px-3 py-3 dark:bg-muted/20"
      >
        {orderedMessages.length === 0 ? (
          <div className="flex h-full items-center justify-center px-6 text-center">
            <p className="text-[13px] text-muted-foreground">
              {chat.busy ? "Загрузка…" : "Сообщений пока нет. Напишите первым 👋"}
            </p>
          </div>
        ) : (
          orderedMessages.map((m, i) => {
            const mine = sameChatUser(userId, m.author_user_id);
            const state =
              mine && chat.thread && userId
                ? getOutgoingDeliveryState(m, chat.thread, userId, orderedMessages)
                : "sent";
            const prev = orderedMessages[i - 1];
            const showDay = dayKey(m.created_at) !== dayKey(prev?.created_at);
            return (
              <div key={m.id}>
                {showDay ? (
                  <div className="my-2 flex justify-center">
                    <span className="rounded-full bg-background/80 px-2.5 py-0.5 text-[11px] font-medium text-muted-foreground shadow-sm">
                      {formatDaySeparator(m.created_at)}
                    </span>
                  </div>
                ) : null}
                <div className={cn("flex", mine ? "justify-end" : "justify-start")}>
                  <div
                    className={cn(
                      "max-w-[78%] rounded-2xl px-3 py-1.5 text-[14px] leading-snug shadow-sm",
                      mine
                        ? "rounded-br-md bg-primary text-primary-foreground"
                        : "rounded-bl-md bg-background text-foreground",
                    )}
                  >
                    <div className="whitespace-pre-wrap break-words">{m.text}</div>
                    <div
                      className={cn(
                        "mt-0.5 flex items-center justify-end gap-1",
                        mine ? "text-primary-foreground/70" : "text-muted-foreground",
                      )}
                    >
                      <span className="text-[10px] leading-none">{formatMessageTime(m.created_at)}</span>
                      {mine && chat.thread ? <DeliveryTicks state={state} /> : null}
                    </div>
                  </div>
                </div>
              </div>
            );
          })
        )}
      </div>

      <div className="flex shrink-0 items-end gap-2 border-t border-border/70 bg-card p-2.5">
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void onSend();
            }
          }}
          rows={1}
          placeholder="Сообщение…"
          className="max-h-28 min-h-[40px] flex-1 resize-none rounded-2xl border border-border bg-background px-3.5 py-2.5 text-[14px] leading-snug outline-none transition-colors placeholder:text-muted-foreground/70 focus-visible:border-primary/60 focus-visible:ring-2 focus-visible:ring-primary/15"
        />
        <Button
          type="button"
          size="icon"
          className="h-10 w-10 shrink-0 rounded-full"
          onClick={onSend}
          disabled={sending || !text.trim()}
          aria-label="Отправить"
        >
          <SendHorizontal className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

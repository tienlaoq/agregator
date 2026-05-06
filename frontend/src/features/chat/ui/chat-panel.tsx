"use client";

import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth";
import type { ChatKind } from "@/features/chat/types/chat";
import { useChatThread } from "@/features/chat/hooks/use-chat-thread";
import {
  getOutgoingDeliveryState,
  sortMessagesForTimeline,
  type OutgoingDeliveryState,
} from "@/features/chat/lib/read-receipts";

type Props = {
  kind: ChatKind;
  refId: string;
  title?: string;
};

function sameChatUser(a: string | undefined, b: string | undefined): boolean {
  if (!a?.trim() || !b?.trim()) return false;
  return a.trim().toLowerCase() === b.trim().toLowerCase();
}

function TelegramStyleTicks({ state }: { state: OutgoingDeliveryState }) {
  const isRead = state === "read";
  const marks = state === "sent" ? "✓" : "✓✓";
  const label = state === "read" ? "Прочитано" : state === "unread" ? "Не прочитано" : "Отправлено";
  return (
    <span
      className={
        isRead
          ? "text-[11px] leading-none tracking-[-0.18em] text-sky-500"
          : "text-[11px] leading-none tracking-[-0.18em] text-muted-foreground/55"
      }
      aria-label={label}
    >
      {marks}
    </span>
  );
}

export function ChatPanel({ kind, refId, title = "Чат по заявке" }: Props) {
  const token = useAuthStore((s) => s.token);
  const userId = useAuthStore((s) => s.user?.id);
  const [text, setText] = useState("");
  const chat = useChatThread(kind, refId, Boolean(token));

  const orderedMessages = useMemo(() => sortMessagesForTimeline(chat.messages), [chat.messages]);

  const onSend = async () => {
    if (!text.trim()) return;
    await chat.send(text.trim());
    setText("");
  };

  return (
    <Card
      className={cn(
        "rounded-2xl border border-neutral-200/90 bg-white shadow-sm dark:border-border dark:bg-card",
      )}
    >
      <CardHeader className="border-b border-neutral-100 pb-3 pt-4 dark:border-border">
        <CardTitle className="text-[17px] font-semibold leading-tight text-neutral-900 dark:text-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 pt-4">
        {chat.error ? <p className="text-sm text-destructive">{chat.error}</p> : null}
        <div className="max-h-52 space-y-2 overflow-y-auto rounded-xl bg-[#ebebeb] p-2.5 dark:bg-muted/50">
          {chat.messages.length === 0 ? (
            <p className="px-1 text-sm text-[#757575] dark:text-muted-foreground">
              {chat.busy ? "Загрузка…" : "Сообщений пока нет."}
            </p>
          ) : (
            chat.messages.map((m) => {
              const mine = sameChatUser(userId, m.author_user_id);
              const state =
                mine && chat.thread && userId
                  ? getOutgoingDeliveryState(m, chat.thread, userId, orderedMessages)
                  : "sent";
              return (
                <div
                  key={m.id}
                  className={cn(
                    "rounded-2xl px-3 py-2 text-[15px] leading-snug shadow-sm",
                    mine
                      ? "ml-6 bg-[#cce4ff] text-neutral-900 dark:bg-primary/25 dark:text-foreground"
                      : "mr-6 bg-white text-neutral-900 dark:bg-background",
                  )}
                >
                  <div className="whitespace-pre-wrap break-words">{m.text}</div>
                  {mine && chat.thread ? (
                    <div className="mt-0.5 flex justify-end">
                      <TelegramStyleTicks state={state} />
                    </div>
                  ) : null}
                </div>
              );
            })
          )}
        </div>
        <div className="flex gap-2">
          <Input
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Сообщение…"
            className="rounded-xl border-neutral-200 bg-white dark:border-border"
          />
          <Button type="button" className="shrink-0 rounded-xl px-4" onClick={onSend} disabled={chat.busy || !text.trim()}>
            Отправить
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}


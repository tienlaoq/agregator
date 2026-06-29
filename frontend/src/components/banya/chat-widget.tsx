"use client";

import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, MessageCircle, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { issueChatWsTicket, listChatThreadsV2 } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { ChatPanel } from "@/features/chat/ui/chat-panel";
import type { ChatThread } from "@/lib/types";
import type { ChatKind } from "@/features/chat/types/chat";
import { setChatSocketToken } from "@/features/chat/lib/global-chat-socket";
import { useChatRealtimeBridge } from "@/features/chat/hooks/use-chat-realtime-bridge";
import {
  chatThreadDisplayLabel,
  chatThreadInitial,
  chatThreadKindLabel,
} from "@/features/chat/lib/thread-label";
import { formatThreadTime } from "@/features/chat/lib/format-time";
import { useChatWidgetStore } from "@/features/chat/store/chat-widget-store";
import { cn } from "@/lib/utils";

function toChatKind(kind: string): ChatKind {
  return kind === "master_booking" ? "master_booking" : "venue_booking";
}

/** Совпадение выбранного из CRM с нитью из списка (оба вида чата — один формат UI). */
function threadMatchesSelection(t: ChatThread, kind: ChatKind, refId: string): boolean {
  return t.ref_id === refId && toChatKind(t.kind) === kind;
}

export function ChatWidget() {
  const token = useAuthStore((s) => s.token);
  const userId = useAuthStore((s) => s.user?.id);
  const queryClient = useQueryClient();
  const isOpen = useChatWidgetStore((s) => s.isOpen);
  const selected = useChatWidgetStore((s) => s.selected);
  const toggle = useChatWidgetStore((s) => s.toggle);
  const close = useChatWidgetStore((s) => s.close);
  const setSelected = useChatWidgetStore((s) => s.setSelected);
  const openWithThread = useChatWidgetStore((s) => s.openWithThread);

  const threadsQuery = useQuery({
    queryKey: ["chat-widget-threads"],
    queryFn: () => listChatThreadsV2({ limit: 100, offset: 0 }),
    enabled: Boolean(token),
    refetchInterval: token ? 15000 : false,
  });

  const wsTicketQuery = useQuery({
    queryKey: ["chat-ws-ticket"],
    queryFn: issueChatWsTicket,
    enabled: Boolean(token),
    staleTime: 70_000,
    refetchInterval: 80_000,
    retry: false,
  });

  const threads = useMemo(() => threadsQuery.data?.threads ?? [], [threadsQuery.data?.threads]);

  /** Сколько диалогов с ненулевым непрочитанным (клиент / баня / пар-мастер). */
  const unreadChatsCount = useMemo(
    () => threads.filter((t) => (t.unread_count ?? 0) > 0).length,
    [threads],
  );

  /** Открытый диалог: либо явно выбранный, либо null (показываем список). */
  const activeThread = useMemo(() => {
    if (!selected) return null;
    return threads.find((t) => threadMatchesSelection(t, selected.kind, selected.refId)) ?? null;
  }, [selected, threads]);

  const activeThreadId = activeThread?.id ?? null;
  const inConversation = Boolean(selected);

  useEffect(() => {
    const ticket = wsTicketQuery.data?.ticket ?? null;
    setChatSocketToken(token ?? null, ticket);
    return () => setChatSocketToken(null);
  }, [token, wsTicketQuery.data?.ticket]);

  useChatRealtimeBridge({
    enabled: Boolean(token),
    queryClient,
    userId,
    isOpen,
    activeThreadId,
  });

  if (!token) return null;

  const conversationTitle = activeThread
    ? chatThreadDisplayLabel(activeThread)
    : selected?.title?.replace(/^Чат:\s*/i, "") ?? "Чат";
  const conversationSubtitle = activeThread ? chatThreadKindLabel(activeThread) : "Диалог";

  return (
    <div className="fixed bottom-4 right-4 z-50 md:bottom-6 md:right-6">
      {isOpen ? (
        <div className="mb-3 flex h-[min(34rem,75vh)] w-[min(24rem,calc(100vw-2rem))] flex-col overflow-hidden rounded-2xl border border-border/80 bg-card shadow-2xl">
          {/* Шапка: список ↔ диалог */}
          <div className="flex shrink-0 items-center gap-2 border-b border-border/70 bg-card px-2.5 py-2.5">
            {inConversation ? (
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className="h-8 w-8 shrink-0"
                onClick={() => setSelected(null)}
                aria-label="Назад к списку"
              >
                <ArrowLeft className="h-4 w-4" />
              </Button>
            ) : null}
            <div className="min-w-0 flex-1">
              <div className="truncate text-[15px] font-semibold leading-tight text-foreground">
                {inConversation ? conversationTitle : "Чаты"}
              </div>
              <div className="truncate text-[12px] leading-tight text-muted-foreground">
                {inConversation
                  ? conversationSubtitle
                  : unreadChatsCount > 0
                    ? `Новых диалогов: ${unreadChatsCount}`
                    : "Все сообщения"}
              </div>
            </div>
            <Button
              type="button"
              size="icon"
              variant="ghost"
              className="h-8 w-8 shrink-0"
              onClick={close}
              aria-label="Закрыть чаты"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>

          {/* Тело: список диалогов или открытая переписка */}
          {inConversation && selected ? (
            <ChatPanel kind={selected.kind} refId={selected.refId} className="flex-1" />
          ) : (
            <div className="min-h-0 flex-1 overflow-y-auto p-1.5">
              {threads.length === 0 ? (
                <p className="px-3 py-10 text-center text-[13px] text-muted-foreground">
                  {threadsQuery.isLoading ? "Загрузка чатов…" : "Доступных чатов пока нет."}
                </p>
              ) : (
                <ul className="space-y-0.5">
                  {threads.map((t) => {
                    const label = chatThreadDisplayLabel(t);
                    const unread = t.unread_count > 0;
                    return (
                      <li key={t.id}>
                        <button
                          type="button"
                          onClick={() =>
                            openWithThread({ kind: toChatKind(t.kind), refId: t.ref_id, title: label })
                          }
                          className="flex w-full items-center gap-2.5 rounded-xl px-2 py-2 text-left transition-colors hover:bg-muted/70"
                        >
                          <span
                            className={cn(
                              "flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-[14px] font-semibold",
                              unread
                                ? "bg-primary text-primary-foreground"
                                : "bg-muted text-muted-foreground",
                            )}
                          >
                            {chatThreadInitial(t)}
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="flex items-baseline justify-between gap-2">
                              <span
                                className={cn(
                                  "truncate text-[14px] leading-snug",
                                  unread ? "font-semibold text-foreground" : "font-medium text-foreground",
                                )}
                              >
                                {label}
                              </span>
                              <span className="shrink-0 text-[11px] text-muted-foreground">
                                {formatThreadTime(t.last_message_at)}
                              </span>
                            </span>
                            <span className="mt-0.5 flex items-center justify-between gap-2">
                              <span className="truncate text-[12px] text-muted-foreground">
                                {chatThreadKindLabel(t)}
                              </span>
                              {unread ? (
                                <span className="flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-primary px-1.5 text-[11px] font-semibold leading-none text-primary-foreground">
                                  {t.unread_count > 99 ? "99+" : t.unread_count}
                                </span>
                              ) : null}
                            </span>
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          )}
        </div>
      ) : null}

      <Button
        type="button"
        size="icon"
        onClick={toggle}
        className="relative h-12 w-12 rounded-full shadow-lg"
        title={
          unreadChatsCount > 0
            ? `Диалогов с новыми сообщениями: ${unreadChatsCount}`
            : "Открыть чаты"
        }
        aria-label={
          unreadChatsCount > 0
            ? `Открыть чаты. Диалогов с непрочитанным: ${unreadChatsCount}`
            : "Открыть чаты"
        }
      >
        <MessageCircle className="h-5 w-5" />
        {unreadChatsCount > 0 ? (
          <span
            className="pointer-events-none absolute -right-0.5 -top-0.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-destructive px-1 text-[11px] font-semibold leading-none text-destructive-foreground shadow-md ring-2 ring-background"
            aria-hidden
          >
            {unreadChatsCount > 99 ? "99+" : unreadChatsCount}
          </span>
        ) : null}
      </Button>
    </div>
  );
}

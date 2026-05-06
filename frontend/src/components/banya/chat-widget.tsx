"use client";

import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageCircle, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { issueChatWsTicket, listChatThreadsV2 } from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import { ChatPanel } from "@/features/chat/ui/chat-panel";
import type { ChatThread } from "@/lib/types";
import type { ChatKind } from "@/features/chat/types/chat";
import { setChatSocketToken } from "@/features/chat/lib/global-chat-socket";
import { useChatRealtimeBridge } from "@/features/chat/hooks/use-chat-realtime-bridge";
import { chatThreadDisplayLabel, chatThreadPanelTitle } from "@/features/chat/lib/thread-label";
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

  const activePanel = useMemo(() => {
    if (selected) {
      const match = threads.find((t) => threadMatchesSelection(t, selected.kind, selected.refId));
      const title = match ? chatThreadPanelTitle(match) : (selected.title ?? "Чат");
      return {
        kind: selected.kind,
        refId: selected.refId,
        title,
      };
    }
    if (threads.length === 0) return null;
    const first = threads[0];
    return {
      kind: toChatKind(first.kind),
      refId: first.ref_id,
      title: chatThreadPanelTitle(first),
    };
  }, [selected, threads]);

  const activeThreadId = useMemo(() => {
    if (!activePanel) return null;
    const t = threads.find(
      (x) => x.ref_id === activePanel.refId && toChatKind(x.kind) === activePanel.kind,
    );
    return t?.id ?? null;
  }, [threads, activePanel]);

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

  return (
    <div className="fixed bottom-4 right-4 z-50 md:bottom-6 md:right-6">
      {isOpen ? (
        <div
          className={cn(
            "mb-3 flex max-h-[75vh] w-[calc(100vw-2rem)] max-w-sm flex-col gap-3 overflow-hidden rounded-2xl border border-neutral-200/90 bg-[#f5f5f5] p-3 shadow-2xl dark:border-border dark:bg-muted/40",
          )}
        >
          <div className="flex items-center justify-between px-0.5">
            <span className="text-[17px] font-semibold leading-tight text-neutral-900 dark:text-foreground">
              Чаты
            </span>
            <Button type="button" size="icon" variant="ghost" className="h-8 w-8 shrink-0" onClick={close}>
              <X className="h-4 w-4" />
            </Button>
          </div>

          <div className="rounded-2xl border border-neutral-200/80 bg-white p-1.5 shadow-sm dark:border-border dark:bg-card">
            <div className="max-h-44 space-y-0.5 overflow-y-auto">
              {threads.length === 0 ? (
                <p className="px-3 py-4 text-center text-sm text-[#757575] dark:text-muted-foreground">
                  {threadsQuery.isLoading ? "Загрузка чатов…" : "Доступных чатов пока нет."}
                </p>
              ) : (
                threads.map((t) => {
                  const active =
                    activePanel?.refId === t.ref_id && activePanel?.kind === toChatKind(t.kind);
                  const label = chatThreadDisplayLabel(t);
                  return (
                    <button
                      key={t.id}
                      type="button"
                      onClick={() =>
                        openWithThread({
                          kind: toChatKind(t.kind),
                          refId: t.ref_id,
                          title: chatThreadPanelTitle(t),
                        })
                      }
                      className={cn(
                        "w-full rounded-xl px-3 py-2.5 text-left transition-colors",
                        active
                          ? "bg-[#F5F0ED] dark:bg-muted"
                          : "hover:bg-neutral-50 dark:hover:bg-muted/60",
                      )}
                    >
                      <div className="text-[15px] font-semibold leading-snug text-neutral-900 dark:text-foreground">
                        {label}
                      </div>
                      <div className="mt-0.5 text-[13px] leading-snug text-[#757575] dark:text-muted-foreground">
                        {t.unread_count > 0 ? `Непрочитанных: ${t.unread_count}` : "Без непрочитанных"}
                      </div>
                    </button>
                  );
                })
              )}
            </div>
          </div>

          {activePanel ? (
            <ChatPanel kind={activePanel.kind} refId={activePanel.refId} title={activePanel.title} />
          ) : null}
        </div>
      ) : null}

      <Button
        type="button"
        size="icon"
        onClick={toggle}
        className="relative h-12 w-12 rounded-full shadow-lg"
        title={
          unreadChatsCount > 0
            ? `Диалогов с новыми сообщениями: ${unreadChatsCount} (цифра на бейдже — число таких диалогов; в списке ниже — непрочитанные сообщения по каждому)`
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

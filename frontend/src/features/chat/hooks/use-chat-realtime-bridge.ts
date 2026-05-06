"use client";

import { useEffect } from "react";
import type { QueryClient } from "@tanstack/react-query";
import { playChatNotificationSound } from "@/features/chat/lib/chat-notification-sound";
import {
  shouldInvalidateChatThreadsList,
  shouldPlayChatNotificationSound,
} from "@/features/chat/lib/notification-policy";
import { subscribeChatEvents } from "@/features/chat/lib/global-chat-socket";

const THREADS_QUERY_KEY = ["chat-widget-threads"] as const;

type Params = {
  enabled: boolean;
  queryClient: QueryClient;
  userId: string | undefined;
  isOpen: boolean;
  activeThreadId: string | null | undefined;
};

/**
 * Инвалидация списка чатов и звук для входящих (когда открыт не тот тред или виджет закрыт).
 */
export function useChatRealtimeBridge({
  enabled,
  queryClient,
  userId,
  isOpen,
  activeThreadId,
}: Params): void {
  useEffect(() => {
    if (!enabled) return;
    return subscribeChatEvents((evt) => {
      const name = evt.event || evt.type || "";

      if (shouldInvalidateChatThreadsList(name)) {
        void queryClient.invalidateQueries({ queryKey: [...THREADS_QUERY_KEY] });
      }

      if (
        shouldPlayChatNotificationSound({
          eventName: name,
          authorUserId: evt.message?.author_user_id,
          currentUserId: userId,
          messageThreadId: evt.message?.thread_id,
          widgetOpen: isOpen,
          activeThreadId,
        })
      ) {
        playChatNotificationSound();
      }
    });
  }, [enabled, queryClient, userId, isOpen, activeThreadId]);
}

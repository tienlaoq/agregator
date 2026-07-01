"use client";

import { useEffect } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  issueNotificationWsTicket,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
} from "@/lib/api";
import { useAuthStore } from "@/store/auth";
import {
  setNotificationSocketToken,
  subscribeNotificationEvents,
} from "@/features/notifications/lib/global-notification-socket";
import { shouldPlayNotificationSound } from "@/features/notifications/lib/notification-sound-policy";
import {
  playNotificationSound,
  unlockNotificationAudio,
} from "@/lib/notification-sound";
import type { NotificationListResponse } from "@/features/notifications/types/notification";

const LIST_KEY = ["notifications", "list"] as const;

/**
 * Состояние и действия колокольчика: список уведомлений, счётчик непрочитанных,
 * пометка прочитанным и realtime-доставка новых уведомлений по WebSocket.
 *
 * Список — единственный источник правды: `unread_count` приходит в том же ответе,
 * поэтому отдельный запрос счётчика не нужен. `enabled` управляет всеми сетевыми
 * эффектами — передавайте Boolean(token).
 */
export function useNotifications(enabled: boolean) {
  const token = useAuthStore((s) => s.token);
  const queryClient = useQueryClient();

  const listQuery = useQuery({
    queryKey: LIST_KEY,
    queryFn: () => listNotifications({ limit: 30, offset: 0 }),
    enabled,
    refetchInterval: enabled ? 60_000 : false,
  });

  const wsTicketQuery = useQuery({
    queryKey: ["notification-ws-ticket"],
    queryFn: issueNotificationWsTicket,
    enabled,
    staleTime: 70_000,
    refetchInterval: enabled ? 80_000 : false,
    retry: false,
  });

  const refreshList = () => queryClient.invalidateQueries({ queryKey: LIST_KEY });

  const markReadMutation = useMutation({
    mutationFn: (notificationId: string) => markNotificationRead(notificationId),
    onSuccess: () => void refreshList(),
  });

  const markAllReadMutation = useMutation({
    mutationFn: () => markAllNotificationsRead(),
    onSuccess: () => void refreshList(),
  });

  // (Ре)подключаем WebSocket при смене токена/билета; отключаем при логауте.
  useEffect(() => {
    if (!enabled) {
      setNotificationSocketToken(null);
      return;
    }
    const ticket = wsTicketQuery.data?.ticket ?? null;
    setNotificationSocketToken(token ?? null, ticket);
    return () => setNotificationSocketToken(null);
  }, [enabled, token, wsTicketQuery.data?.ticket]);

  // Web Audio блокируется до первого жеста пользователя. Разблокируем один раз
  // на первом клике/тапе по странице — общий AudioContext включает звук и для
  // колокольчика, и для чата, даже если виджет чата ни разу не открывали.
  useEffect(() => {
    if (!enabled) return;
    const unlock = () => void unlockNotificationAudio();
    window.addEventListener("pointerdown", unlock, { once: true });
    return () => window.removeEventListener("pointerdown", unlock);
  }, [enabled]);

  // Новое уведомление прилетело по сокету — обновляем список (и счётчик в нём)
  // и проигрываем короткий сигнал.
  useEffect(() => {
    if (!enabled) return;
    return subscribeNotificationEvents((evt) => {
      if (evt.event !== "notification.created") return;
      void queryClient.invalidateQueries({ queryKey: LIST_KEY });
      if (
        shouldPlayNotificationSound({
          eventName: evt.event,
          hasNotification: Boolean(evt.notification),
        })
      ) {
        playNotificationSound();
      }
    });
  }, [enabled, queryClient]);

  const list: NotificationListResponse["notifications"] =
    listQuery.data?.notifications ?? [];
  const unreadCount = listQuery.data?.unread_count ?? 0;

  return {
    list,
    unreadCount,
    isLoading: listQuery.isLoading,
    markRead: markReadMutation.mutate,
    markAllRead: markAllReadMutation.mutate,
    isMarkingAll: markAllReadMutation.isPending,
  };
}

/** Префикс HTTP API уведомлений ("колокольчик"); gateway монтирует v2 под `/api/v2`. */
export const NOTIFICATION_V2_PREFIX = "/api/v2/notifications" as const;

/** Путь WebSocket уведомлений (относительно origin gateway). */
export const NOTIFICATION_V2_WS_PATH = `${NOTIFICATION_V2_PREFIX}/ws` as const;

export const notificationV2Paths = {
  list: NOTIFICATION_V2_PREFIX,
  unreadCount: `${NOTIFICATION_V2_PREFIX}/unread-count`,
  readAll: `${NOTIFICATION_V2_PREFIX}/read-all`,
  read: (notificationId: string) =>
    `${NOTIFICATION_V2_PREFIX}/${encodeURIComponent(notificationId)}/read`,
  wsTicket: `${NOTIFICATION_V2_PREFIX}/ws-ticket`,
} as const;

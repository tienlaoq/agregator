/** Одно уведомление из колокольчика (форма ответа gateway). */
export type Notification = {
  id: string;
  user_id: string;
  type: string;
  title: string;
  body: string;
  read: boolean;
  /** Произвольный JSON-payload строкой (например, `{"kind":"crm_staff_invited",…}`). */
  data?: string;
  /** RFC3339. */
  created_at?: string;
  /** RFC3339, присутствует только у прочитанных. */
  read_at?: string;
};

export type NotificationListResponse = {
  notifications: Notification[];
  total: number;
  unread_count: number;
};

export type UnreadCountResponse = {
  unread_count: number;
};

/** Кадр, приходящий по WebSocket уведомлений. */
export type NotificationEventEnvelope = {
  event?: string; // "notification.created" | "notification.connected" | "notification.pong"
  notification?: Notification;
};

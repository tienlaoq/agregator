"use client";

/**
 * Звук чата теперь живёт в общем модуле @/lib/notification-sound (он же
 * используется колокольчиком). Алиасы сохранены, чтобы не трогать call-sites.
 */

export {
  unlockNotificationAudio as unlockChatNotificationAudio,
  playNotificationSound as playChatNotificationSound,
} from "@/lib/notification-sound";

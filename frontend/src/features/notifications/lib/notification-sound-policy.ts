/**
 * Чистая логика: проигрывать ли звук при входящем кадре колокольчика по WS.
 *
 * Уведомления — персональный инбокс: сервер шлёт `notification.created` только
 * владельцу, поэтому «свой/чужой автор» здесь не проверяется (в отличие от чата).
 * Пикаем только на создание нового уведомления с полезной нагрузкой.
 */

export type NotificationSoundInput = {
  eventName: string;
  hasNotification: boolean;
};

export function isNewNotificationEvent(eventName: string): boolean {
  return eventName === "notification.created";
}

export function shouldPlayNotificationSound(i: NotificationSoundInput): boolean {
  return isNewNotificationEvent(i.eventName) && i.hasNotification;
}

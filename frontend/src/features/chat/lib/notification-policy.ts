/**
 * Чистая логика: проигрывать ли звук при входящем сообщении по WS.
 */

export type ChatNotificationSoundInput = {
  eventName: string;
  authorUserId: string | undefined;
  currentUserId: string | undefined;
  messageThreadId: string | undefined;
  widgetOpen: boolean;
  activeThreadId: string | null | undefined;
};

export function isIncomingChatMessageEvent(eventName: string): boolean {
  return eventName === "chat.message.created" || eventName === "message_new";
}

/** Инвалидировать список тредов при этих событиях. */
export function shouldInvalidateChatThreadsList(eventName: string): boolean {
  return (
    eventName === "chat.thread.read_updated" ||
    eventName === "read_updated" ||
    eventName === "chat.message.created" ||
    eventName === "message_new"
  );
}

export function shouldPlayChatNotificationSound(i: ChatNotificationSoundInput): boolean {
  if (!isIncomingChatMessageEvent(i.eventName)) return false;
  if (!i.authorUserId || !i.currentUserId) return false;
  if (i.authorUserId === i.currentUserId) return false;
  if (!i.messageThreadId) return false;

  const viewingThatThread =
    i.widgetOpen &&
    Boolean(i.activeThreadId) &&
    i.activeThreadId === i.messageThreadId;
  if (viewingThatThread) return false;

  return true;
}

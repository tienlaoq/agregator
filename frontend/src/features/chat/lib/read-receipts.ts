import type { ChatMessage, ChatThread } from "@/lib/types";

function normId(id: string): string {
  return id.trim().toLowerCase();
}

export function sortMessagesForTimeline(messages: ChatMessage[]): ChatMessage[] {
  return [...messages].sort((a, b) => {
    const ta = a.created_at ?? "";
    const tb = b.created_at ?? "";
    if (ta !== tb) return ta.localeCompare(tb);
    return a.id.localeCompare(b.id);
  });
}

export function messageTimelineIndex(msgId: string, ordered: ChatMessage[]): number {
  return ordered.findIndex((m) => m.id === msgId);
}

/** True when every other participant has marked read at or past this message (chronological order). */
export function isOutgoingReadByEveryone(
  msg: ChatMessage,
  thread: ChatThread,
  viewerUserId: string,
  ordered: ChatMessage[],
): boolean {
  const vid = normId(viewerUserId);
  const msgIdx = messageTimelineIndex(msg.id, ordered);
  if (msgIdx < 0) return false;

  const participants = thread.participant_user_ids ?? [];
  const recipients = participants.map(normId).filter((id) => id && id !== vid);
  if (recipients.length === 0) return false;

  const reads = thread.participant_reads ?? [];
  for (const rid of recipients) {
    const row = reads.find((r) => normId(r.user_id) === rid);
    const watermark = row?.last_read_message_id;
    if (!watermark) return false;
    const wIdx = messageTimelineIndex(watermark, ordered);
    if (wIdx < 0) return false;
    if (wIdx < msgIdx) return false;
  }
  return true;
}

export type OutgoingDeliveryState = "sent" | "unread" | "read";

export function getOutgoingDeliveryState(
  msg: ChatMessage,
  thread: ChatThread,
  viewerUserId: string,
  ordered: ChatMessage[],
): OutgoingDeliveryState {
  if (isOutgoingReadByEveryone(msg, thread, viewerUserId, ordered)) return "read";

  const vid = normId(viewerUserId);
  const recipients = (thread.participant_user_ids ?? []).map(normId).filter((id) => id && id !== vid);
  if (recipients.length === 0) return "sent";

  const reads = thread.participant_reads ?? [];
  const hasAnyRecipientReadMark = recipients.some((rid) => {
    const row = reads.find((r) => normId(r.user_id) === rid);
    return Boolean(row?.last_read_message_id);
  });
  return hasAnyRecipientReadMark ? "unread" : "sent";
}

import type { ChatMessage, ChatThread } from "@/lib/types";

export type ChatKind = "venue_booking" | "master_booking";
export type ChatConnectionState = "connecting" | "online" | "reconnecting" | "offline";
export type ChatDeliveryState = "sending" | "sent" | "failed";

export type ChatEventEnvelope = {
  type?: string; // v1 compatibility
  event?: string; // v2
  message?: ChatMessage;
  thread?: ChatThread;
  error?: string;
};


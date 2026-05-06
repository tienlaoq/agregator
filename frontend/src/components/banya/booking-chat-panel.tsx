"use client";
import { useEffect } from "react";
import { useChatWidgetStore } from "@/features/chat/store/chat-widget-store";

type Props = {
  kind: "venue_booking" | "master_booking";
  refId: string;
  title?: string;
};

export function BookingChatPanel({ kind, refId, title = "Чат по заявке" }: Props) {
  const openWithThread = useChatWidgetStore((s) => s.openWithThread);

  useEffect(() => {
    openWithThread({ kind, refId, title });
  }, [kind, openWithThread, refId, title]);

  return null;
}


"use client";

import { create } from "zustand";
import type { ChatKind } from "@/features/chat/types/chat";
import { unlockChatNotificationAudio } from "@/features/chat/lib/chat-notification-sound";

type ChatWidgetStore = {
  isOpen: boolean;
  selected: { kind: ChatKind; refId: string; title?: string } | null;
  toggle: () => void;
  close: () => void;
  setSelected: (thread: { kind: ChatKind; refId: string; title?: string } | null) => void;
  openWithThread: (thread: { kind: ChatKind; refId: string; title?: string }) => void;
};

export const useChatWidgetStore = create<ChatWidgetStore>((set) => ({
  isOpen: false,
  selected: null,
  toggle: () => {
    void unlockChatNotificationAudio();
    set((s) => ({ isOpen: !s.isOpen }));
  },
  close: () => set({ isOpen: false }),
  setSelected: (thread) => set({ selected: thread }),
  openWithThread: (thread) => {
    void unlockChatNotificationAudio();
    set({ isOpen: true, selected: thread });
  },
}));


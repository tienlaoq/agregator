import { describe, expect, it } from "vitest";
import {
  shouldInvalidateChatThreadsList,
  shouldPlayChatNotificationSound,
} from "../notification-policy";

describe("shouldPlayChatNotificationSound", () => {
  const base = {
    eventName: "chat.message.created",
    authorUserId: "other",
    currentUserId: "me",
    messageThreadId: "t1",
    widgetOpen: false,
    activeThreadId: null as string | null,
  };

  it("plays when widget closed and message from other", () => {
    expect(shouldPlayChatNotificationSound(base)).toBe(true);
  });

  it("does not play for own message", () => {
    expect(
      shouldPlayChatNotificationSound({
        ...base,
        authorUserId: "me",
      }),
    ).toBe(false);
  });

  it("does not play when viewing that thread", () => {
    expect(
      shouldPlayChatNotificationSound({
        ...base,
        widgetOpen: true,
        activeThreadId: "t1",
      }),
    ).toBe(false);
  });

  it("plays when widget open but different thread", () => {
    expect(
      shouldPlayChatNotificationSound({
        ...base,
        widgetOpen: true,
        activeThreadId: "t2",
      }),
    ).toBe(true);
  });

  it("does not play for non-message events", () => {
    expect(
      shouldPlayChatNotificationSound({
        ...base,
        eventName: "chat.pong",
      }),
    ).toBe(false);
  });
});

describe("shouldInvalidateChatThreadsList", () => {
  it("matches message and read events", () => {
    expect(shouldInvalidateChatThreadsList("chat.message.created")).toBe(true);
    expect(shouldInvalidateChatThreadsList("message_new")).toBe(true);
    expect(shouldInvalidateChatThreadsList("chat.thread.read_updated")).toBe(true);
    expect(shouldInvalidateChatThreadsList("read_updated")).toBe(true);
    expect(shouldInvalidateChatThreadsList("chat.pong")).toBe(false);
  });
});

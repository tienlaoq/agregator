import { describe, expect, it } from "vitest";
import { shouldPlayNotificationSound } from "@/features/notifications/lib/notification-sound-policy";

describe("shouldPlayNotificationSound", () => {
  it("plays on a created notification with payload", () => {
    expect(
      shouldPlayNotificationSound({ eventName: "notification.created", hasNotification: true }),
    ).toBe(true);
  });

  it("ignores non-created events", () => {
    for (const eventName of ["notification.connected", "notification.pong", "read_updated", ""]) {
      expect(shouldPlayNotificationSound({ eventName, hasNotification: true })).toBe(false);
    }
  });

  it("ignores a created event without a notification body", () => {
    expect(
      shouldPlayNotificationSound({ eventName: "notification.created", hasNotification: false }),
    ).toBe(false);
  });
});

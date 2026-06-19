import { describe, expect, it } from "vitest";
import { safeNextPath } from "./safe-redirect";

describe("safeNextPath", () => {
  it.each([
    ["/support", "/support"],
    ["/", "/"],
    ["/venues?city=moskva", "/venues?city=moskva"],
    ["/my/bookings", "/my/bookings"],
  ])("accepts same-site path %s", (input, expected) => {
    expect(safeNextPath(input)).toBe(expected);
  });

  it.each([
    ["//evil.com", "protocol-relative"],
    ["/\\evil.com", "backslash"],
    ["https://evil.com", "absolute url"],
    ["evil.com", "no leading slash"],
    ["support", "bare word"],
    ["", "empty"],
    [null, "null"],
    [undefined, "undefined"],
  ])("rejects %s (%s)", (input) => {
    expect(safeNextPath(input)).toBeNull();
  });
});

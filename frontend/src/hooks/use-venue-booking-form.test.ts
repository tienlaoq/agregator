import { describe, expect, it } from "vitest";
import { effectiveHourlyRateRub, isWeekendDate } from "./use-venue-booking-form";
import type { Venue } from "@/lib/types";

const venue = {
  price_from: 3000,
  price_weekend: 4000,
  halls: [
    { id: "h1", price_from: 5000, price_weekend: 6000 },
    { id: "h2", price_from: 2000, price_weekend: 0 }, // weekend unset → weekday
  ],
} as unknown as Venue;

describe("isWeekendDate", () => {
  it("undefined date is treated as weekday", () => {
    expect(isWeekendDate(undefined)).toBe(false);
  });
  it("Saturday and Sunday are weekend", () => {
    expect(isWeekendDate(new Date("2026-07-18"))).toBe(true); // Sat
    expect(isWeekendDate(new Date("2026-07-19"))).toBe(true); // Sun
    expect(isWeekendDate(new Date("2026-07-17"))).toBe(false); // Fri
  });
});

describe("effectiveHourlyRateRub", () => {
  it("weekday: venue base with no halls", () => {
    expect(effectiveHourlyRateRub(venue, [], false)).toBe(3000);
  });
  it("weekday: hall raises base", () => {
    expect(effectiveHourlyRateRub(venue, ["h1"], false)).toBe(5000);
  });
  it("weekend: venue weekend rate", () => {
    expect(effectiveHourlyRateRub(venue, [], true)).toBe(4000);
  });
  it("weekend: hall weekend rate wins", () => {
    expect(effectiveHourlyRateRub(venue, ["h1"], true)).toBe(6000);
  });
  it("weekend: hall without weekend rate falls back to weekday, venue weekend wins", () => {
    // h2 weekend unset → 2000; venue weekend 4000 is higher.
    expect(effectiveHourlyRateRub(venue, ["h2"], true)).toBe(4000);
  });
});

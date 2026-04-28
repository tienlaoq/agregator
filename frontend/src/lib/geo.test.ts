import { describe, expect, it } from "vitest";
import {
  excludeZoneContainedInTravelRadius,
  haversineKm,
  TRAVEL_ZONE_CONTAINMENT_EPS_KM,
} from "./geo";

describe("haversineKm", () => {
  it("returns 0 for identical points", () => {
    expect(haversineKm(55.75, 37.62, 55.75, 37.62)).toBe(0);
  });

  it("is symmetric", () => {
    const a = haversineKm(59.93, 30.33, 55.76, 37.62);
    const b = haversineKm(55.76, 37.62, 59.93, 30.33);
    expect(Math.abs(a - b)).toBeLessThan(1e-9);
  });
});

describe("excludeZoneContainedInTravelRadius", () => {
  it("allows zone fully inside travel disk", () => {
    const pinLat = 55.75;
    const pinLon = 37.62;
    const r = 10;
    const dSmall = 2;
    const zoneLat = pinLat + dSmall / 111;
    const zoneLon = pinLon;
    const zoneR = 5;
    expect(
      excludeZoneContainedInTravelRadius(pinLat, pinLon, r, zoneLat, zoneLon, zoneR),
    ).toBe(true);
  });

  it("rejects zone extending past travel radius", () => {
    expect(
      excludeZoneContainedInTravelRadius(55, 37, 5, 56, 37, 10, TRAVEL_ZONE_CONTAINMENT_EPS_KM),
    ).toBe(false);
  });

  it("returns false when travel radius invalid", () => {
    expect(excludeZoneContainedInTravelRadius(0, 0, 0, 1, 1, 1)).toBe(false);
    expect(excludeZoneContainedInTravelRadius(0, 0, -1, 1, 1, 1)).toBe(false);
  });
});

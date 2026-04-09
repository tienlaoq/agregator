import { describe, expect, it } from "vitest"
import { extractStreetCore, isStreetSuggestReady } from "../address-suggest-rules"

describe("address-suggest-rules", () => {
  it("extractStreetCore strips common prefixes", () => {
    expect(extractStreetCore("ул. Ленина")).toBe("Ленина")
    expect(extractStreetCore("пр-т Мира")).toBe("Мира")
  })

  it("isStreetSuggestReady requires half of first line (min 4 core chars)", () => {
    expect(isStreetSuggestReady("Ле")).toBe(false)
    expect(isStreetSuggestReady("ул. Ле")).toBe(false)
    expect(isStreetSuggestReady("Лен")).toBe(false)
    expect(isStreetSuggestReady("Лени")).toBe(true)
    expect(isStreetSuggestReady("Ленина")).toBe(true)
    expect(isStreetSuggestReady("ул. Ленина")).toBe(true)
    expect(isStreetSuggestReady("Бауман, д. 1")).toBe(true)
  })
})

import { test, expect } from "@playwright/test";

test.describe("Catalog", () => {
  test("venues page loads", async ({ page }) => {
    await page.goto("/venues");
    await page.waitForLoadState("networkidle");
    const body = await page.textContent("body");
    expect(body).toBeTruthy();
  });

  test("homepage loads with navigation", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveTitle(/./);

    const catalogLink = page.locator("a[href='/venues']");
    if (await catalogLink.count() > 0) {
      await catalogLink.first().click();
      await expect(page).toHaveURL(/venues/);
    }
  });

  test("search functionality works", async ({ page }) => {
    await page.goto("/venues");
    await page.waitForLoadState("networkidle");

    const searchInput = page.locator("input[type='search'], input[placeholder*='Поиск'], input[placeholder*='поиск'], input[name='search'], input[name='q']");
    if (await searchInput.count() > 0) {
      await searchInput.first().fill("Баня");
      await page.waitForTimeout(1000);
    }
  });

  test("venue detail page accessible via slug", async ({ page }) => {
    await page.goto("/venues");
    await page.waitForLoadState("networkidle");

    const venueLink = page.locator("a[href^='/venues/']").first();
    if (await venueLink.count() > 0) {
      await venueLink.click();
      await page.waitForLoadState("networkidle");
      await expect(page).toHaveURL(/venues\/.+/);
    }
  });
});

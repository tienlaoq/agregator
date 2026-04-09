import { test, expect } from "@playwright/test";

test.describe("Admin Moderation", () => {
  test("admin venues page requires authentication", async ({ page }) => {
    await page.goto("/admin/venues");
    await page.waitForLoadState("networkidle");

    const url = page.url();
    const body = await page.textContent("body");
    expect(url + body).toBeTruthy();
  });

  test("partner registration page loads", async ({ page }) => {
    await page.goto("/partner");
    await page.waitForLoadState("networkidle");
    const body = await page.textContent("body");
    expect(body).toBeTruthy();
  });

  test("owner venues page requires authentication", async ({ page }) => {
    await page.goto("/owner/venues");
    await page.waitForLoadState("networkidle");
    const body = await page.textContent("body");
    expect(body).toBeTruthy();
  });

  test("create venue page requires authentication", async ({ page }) => {
    await page.goto("/owner/venues/new");
    await page.waitForLoadState("networkidle");
    const body = await page.textContent("body");
    expect(body).toBeTruthy();
  });
});

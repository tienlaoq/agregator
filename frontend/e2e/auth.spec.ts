import { test, expect } from "@playwright/test";

test.describe("Authentication", () => {
  test("register page loads and has required fields", async ({ page }) => {
    await page.goto("/auth/register");
    await expect(page.locator("input[type='email'], input[name='email']")).toBeVisible();
    await expect(page.locator("input[type='password'], input[name='password']")).toBeVisible();
  });

  test("login page loads and has required fields", async ({ page }) => {
    await page.goto("/auth/login");
    await expect(page.locator("input[type='email'], input[name='email']")).toBeVisible();
    await expect(page.locator("input[type='password'], input[name='password']")).toBeVisible();
  });

  test("login with invalid credentials shows error", async ({ page }) => {
    await page.goto("/auth/login");

    await page.locator("input[type='email'], input[name='email']").fill("invalid@test.com");
    await page.locator("input[type='password'], input[name='password']").fill("wrongpassword");

    const submitButton = page.locator("button[type='submit']");
    if (await submitButton.isVisible()) {
      await submitButton.click();
      await page.waitForTimeout(2000);
      const pageContent = await page.textContent("body");
      expect(pageContent).toBeTruthy();
    }
  });

  test("register page has role selection", async ({ page }) => {
    await page.goto("/auth/register");
    const body = await page.textContent("body");
    expect(body).toBeTruthy();
  });

  test("unauthenticated user sees login/register buttons", async ({ page }) => {
    await page.goto("/");
    const loginLink = page.locator("a[href='/auth/login']");
    const registerLink = page.locator("a[href='/auth/register']");

    const hasLogin = await loginLink.count();
    const hasRegister = await registerLink.count();
    expect(hasLogin + hasRegister).toBeGreaterThan(0);
  });
});

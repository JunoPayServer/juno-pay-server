import { test, expect } from "@playwright/test";

const adminPassword = process.env.E2E_ADMIN_PASSWORD ?? "test-admin-password";

test("redirects to /login when not authenticated", async ({ page }) => {
  await page.goto("/status");
  await expect(page).toHaveURL(/\/login$/);
});

test("can login and view status", async ({ page }) => {
  await page.goto("/login");
  await page.locator("#password").fill(adminPassword);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page).toHaveURL(/\/status$/);
  await expect(page.getByRole("heading", { name: "Status" })).toBeVisible();
  await expect(page.getByText("Best Height")).toBeVisible();
  await expect(page.getByText("42")).toBeVisible();
});

test("can create a merchant", async ({ page }) => {
  await page.goto("/login");
  await page.locator("#password").fill(adminPassword);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page).toHaveURL(/\/status$/);

  await page.goto("/merchants");
  await expect(page.getByRole("heading", { name: "Merchants", level: 1 })).toBeVisible();

  const name = `acme-${Date.now()}`;
  await page.locator("#name").fill(name);
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.getByRole("link", { name })).toBeVisible();
});

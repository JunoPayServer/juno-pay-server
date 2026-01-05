import { test, expect } from "@playwright/test";

test("registers, buys air, and auto-tracks checkout status", async ({ page, request }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Register" })).toBeVisible();

  await page.locator("#email").fill("demo@example.com");
  await page.getByRole("button", { name: "Register" }).click();
  await expect(page.getByRole("heading", { name: "Buy Air" })).toBeVisible();

  await page.getByRole("button", { name: "Buy" }).click();
  await expect(page.getByText("Checkout")).toBeVisible();
  await expect(page.getByText("Awaiting payment")).toBeVisible();

  const invoiceID = await page.evaluate(() => {
    const raw = localStorage.getItem("juno_demo_orders_v1") ?? "[]";
    const v = JSON.parse(raw);
    return v[0]?.invoice_id ?? "";
  });
  expect(invoiceID).toMatch(/^inv_/);

  await request.post(`http://127.0.0.1:39180/_test/pay/${invoiceID}`);
  await expect(page.getByText("Pending confirmations")).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText(/Confirmations:/)).toBeVisible();
  await expect(page.getByText("Payment complete")).toBeVisible({ timeout: 20_000 });

  await page.goto("/orders");
  const row = page.locator("tr", { hasText: invoiceID });
  await expect(row).toContainText("confirmed", { timeout: 20_000 });
});

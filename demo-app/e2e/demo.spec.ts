import { test, expect } from "@playwright/test";

test("registers, buys air, and tracks order status", async ({ page, request }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Register" })).toBeVisible();

  await page.locator("#email").fill("demo@example.com");
  await page.getByRole("button", { name: "Register" }).click();
  await expect(page.getByRole("heading", { name: "Buy Air" })).toBeVisible();

  await page.getByRole("button", { name: "Buy" }).click();
  await expect(page.getByText("Latest order")).toBeVisible();

  const invoiceID = await page.evaluate(() => {
    const raw = localStorage.getItem("juno_demo_orders_v1") ?? "[]";
    const v = JSON.parse(raw);
    return v[0]?.invoice_id ?? "";
  });
  expect(invoiceID).toMatch(/^inv_/);

  await page.goto("/orders");
  const row = page.locator("tr", { hasText: invoiceID });
  await expect(row).toContainText("open");

  await request.post(`http://127.0.0.1:39180/_test/pay/${invoiceID}`);
  await page.getByRole("button", { name: "Refresh all" }).click();
  await expect(row).toContainText("paid");

  await row.getByRole("button", { name: "View" }).click();
  await page.getByRole("button", { name: "Fetch events" }).click();
  await expect(page.getByText("invoice.created")).toBeVisible();
  await expect(page.getByText("invoice.paid")).toBeVisible();
});


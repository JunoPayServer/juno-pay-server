import { afterEach, describe, expect, it, vi } from "vitest";
import { createAirInvoice } from "@/app/actions";

afterEach(() => {
  vi.unstubAllGlobals();
  delete process.env.JUNO_PAY_BASE_URL;
  delete process.env.JUNO_PAY_MERCHANT_API_KEY;
});

describe("demo actions", () => {
  it("createAirInvoice returns config error when merchant key is missing", async () => {
    process.env.JUNO_PAY_BASE_URL = "http://example.test";
    const res = await createAirInvoice({ external_order_id: "o1", demo_user_id: "u1", email: "a@b.com" });
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.code).toBe("config");
      expect(res.error).toMatch(/missing merchant api key/i);
    }
  });

  it("createAirInvoice returns API error details without throwing", async () => {
    process.env.JUNO_PAY_BASE_URL = "http://example.test";
    process.env.JUNO_PAY_MERCHANT_API_KEY = "k_test";

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        const body = { status: "error", error: { code: "unauthorized", message: "unauthorized" } };
        return new Response(JSON.stringify(body), { status: 401, headers: { "Content-Type": "application/json" } });
      }),
    );

    const res = await createAirInvoice({ external_order_id: "o1", demo_user_id: "u1", email: "a@b.com" });
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.code).toBe("unauthorized");
      expect(res.error).toBe("unauthorized");
    }
  });

  it("createAirInvoice returns ok response", async () => {
    process.env.JUNO_PAY_BASE_URL = "http://example.test";
    process.env.JUNO_PAY_MERCHANT_API_KEY = "k_test";

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        const body = {
          status: "ok",
          data: {
            invoice: {
              invoice_id: "inv_1",
              merchant_id: "m_1",
              external_order_id: "o1",
              status: "open",
              address: "j1...",
              amount_zat: 100_000_000,
              required_confirmations: 100,
              received_zat_pending: 0,
              received_zat_confirmed: 0,
              expires_at: null,
              created_at: "2026-01-01T00:00:00Z",
              updated_at: "2026-01-01T00:00:00Z",
            },
            invoice_token: "inv_tok_1",
          },
        };
        return new Response(JSON.stringify(body), { status: 201, headers: { "Content-Type": "application/json" } });
      }),
    );

    const res = await createAirInvoice({ external_order_id: "o1", demo_user_id: "u1", email: "a@b.com" });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.data.invoice.invoice_id).toBe("inv_1");
      expect(res.data.invoice_token).toBe("inv_tok_1");
    }
  });
});


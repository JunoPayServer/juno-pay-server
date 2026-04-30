import { beforeEach, describe, expect, it, vi } from "vitest";
import { APIError, adminLogin, adminLogout, listMerchants } from "@/lib/api";

describe("api", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("listMerchants parses ok response", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input, init) => {
        expect(input).toBe("/v1/admin/merchants");
        expect((init?.headers as Record<string, string>)["Content-Type"]).toBe("application/json");

        return new Response(
          JSON.stringify({
            status: "ok",
            data: [
              {
                merchant_id: "m_1",
                name: "Demo",
                status: "active",
                settings: {
                  invoice_ttl_seconds: 3600,
                  required_confirmations: 100,
                  policies: {
                    late_payment_policy: "reject",
                    partial_payment_policy: "reject",
                    overpayment_policy: "accept",
                  },
                },
                created_at: "2025-01-01T00:00:00Z",
                updated_at: "2025-01-01T00:00:00Z",
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );

    const out = await listMerchants();
    expect(out).toHaveLength(1);
    expect(out[0]?.merchant_id).toBe("m_1");
  });

  it("throws APIError with server error body", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        return new Response(
          JSON.stringify({
            status: "error",
            error: { code: "bad_request", message: "nope" },
          }),
          { status: 400, headers: { "Content-Type": "application/json" } },
        );
      }),
    );

    await expect(listMerchants()).rejects.toMatchObject({ name: "APIError", status: 400, code: "bad_request", message: "nope" });
  });

  it("throws APIError for invalid ok response envelope", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        return new Response(JSON.stringify({ ok: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      }),
    );

    await expect(listMerchants()).rejects.toMatchObject({ name: "APIError", status: 200, message: "invalid response" });
  });

  it("adminLogin succeeds on 204", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(input).toBe("/admin/login");
      expect(init?.method).toBe("POST");
      expect(init?.headers).toMatchObject({ "Content-Type": "application/json" });
      expect(init?.body).toBe(JSON.stringify({ password: "secret" }));
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(adminLogin("secret")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("adminLogin surfaces error message if present", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        return new Response(
          JSON.stringify({ status: "error", error: { code: "unauthorized", message: "wrong password" } }),
          { status: 401, headers: { "Content-Type": "application/json" } },
        );
      }),
    );

    let err: unknown;
    try {
      await adminLogin("bad");
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(APIError);
    expect(err).toMatchObject({ status: 401, message: "wrong password" });
  });

  it("adminLogout succeeds on 204", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(input).toBe("/admin/logout");
      expect(init?.method).toBe("POST");
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal("fetch", fetchMock);
    await expect(adminLogout()).resolves.toBeUndefined();
  });
});

import { describe, expect, it } from "vitest";
import { confirmationsCount, depositHeightForConfirmations, formatCountdown, invoicePhase, secondsUntilExpiry } from "@/lib/invoice";

describe("invoice helpers", () => {
  it("secondsUntilExpiry returns null for missing/invalid", () => {
    expect(secondsUntilExpiry(null, 0)).toBe(null);
    expect(secondsUntilExpiry("", 0)).toBe(null);
    expect(secondsUntilExpiry("not-a-date", 0)).toBe(null);
  });

  it("secondsUntilExpiry clamps to 0 when expired", () => {
    expect(secondsUntilExpiry("2026-01-01T00:00:00Z", Date.parse("2026-01-01T00:00:01Z"))).toBe(0);
  });

  it("formatCountdown formats mm:ss and h:mm:ss", () => {
    expect(formatCountdown(0)).toBe("0:00");
    expect(formatCountdown(5)).toBe("0:05");
    expect(formatCountdown(65)).toBe("1:05");
    expect(formatCountdown(3600)).toBe("1:00:00");
    expect(formatCountdown(3661)).toBe("1:01:01");
  });

  it("invoicePhase advances based on amounts and expiry", () => {
    const base = {
      invoice_id: "inv_1",
      merchant_id: "m_1",
      external_order_id: "o1",
      status: "open",
      address: "j1...",
      amount_zat: 100,
      required_confirmations: 100,
      received_zat_pending: 0,
      received_zat_confirmed: 0,
      expires_at: "2026-01-02T00:00:00Z",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };

    expect(invoicePhase(base, Date.parse("2026-01-01T00:00:00Z"))).toBe("awaiting_payment");
    expect(invoicePhase({ ...base, received_zat_pending: 100 }, Date.parse("2026-01-01T00:00:00Z"))).toBe("pending_confirmations");
    expect(invoicePhase({ ...base, received_zat_confirmed: 100 }, Date.parse("2026-01-01T00:00:00Z"))).toBe("payment_complete");
    expect(invoicePhase(base, Date.parse("2026-01-03T00:00:00Z"))).toBe("expired");
  });

  it("depositHeightForConfirmations returns max height across deposits", () => {
    const events = [
      { event_id: "1", type: "invoice.created", occurred_at: "2026-01-01T00:00:00Z", invoice_id: "inv_1", deposit: null, refund: null },
      {
        event_id: "2",
        type: "deposit.detected",
        occurred_at: "2026-01-01T00:00:00Z",
        invoice_id: "inv_1",
        deposit: { wallet_id: "w1", txid: "t1", action_index: 0, amount_zat: 100, height: 100 },
        refund: null,
      },
      {
        event_id: "3",
        type: "deposit.detected",
        occurred_at: "2026-01-01T00:00:00Z",
        invoice_id: "inv_1",
        deposit: { wallet_id: "w1", txid: "t2", action_index: 0, amount_zat: 50, height: 120 },
        refund: null,
      },
    ];
    expect(depositHeightForConfirmations(events)).toBe(120);
  });

  it("confirmationsCount computes bestHeight - depositHeight + 1", () => {
    expect(confirmationsCount(null, 100)).toBe(null);
    expect(confirmationsCount(100, null)).toBe(null);
    expect(confirmationsCount(99, 100)).toBe(0);
    expect(confirmationsCount(100, 100)).toBe(1);
    expect(confirmationsCount(104, 100)).toBe(5);
  });
});


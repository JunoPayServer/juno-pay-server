import type { JunoPayInvoice, JunoPayInvoicePhase } from "./types.js";

export function receivedTotalZat(inv: JunoPayInvoice): number {
  return (inv.received_zat_pending ?? 0) + (inv.received_zat_confirmed ?? 0);
}

export function isPaymentComplete(inv: JunoPayInvoice): boolean {
  return (inv.received_zat_confirmed ?? 0) >= inv.amount_zat;
}

export function isFullyPaid(inv: JunoPayInvoice): boolean {
  return receivedTotalZat(inv) >= inv.amount_zat;
}

export function secondsUntilExpiry(expiresAt: string | null | undefined, nowMs: number): number | null {
  if (!expiresAt) return null;
  const t = Date.parse(expiresAt);
  if (!Number.isFinite(t)) return null;
  const diff = Math.floor((t - nowMs) / 1000);
  return diff < 0 ? 0 : diff;
}

export function invoicePhase(inv: JunoPayInvoice, nowMs: number): JunoPayInvoicePhase {
  const exp = secondsUntilExpiry(inv.expires_at, nowMs);
  if (exp === 0 && !isFullyPaid(inv)) return "expired";
  if (isPaymentComplete(inv)) return "payment_complete";
  if (isFullyPaid(inv)) return "pending_confirmations";
  return "awaiting_payment";
}

import type { Invoice, InvoiceEvent } from "@/app/actions";

export type InvoicePhase = "awaiting_payment" | "pending_confirmations" | "payment_complete" | "expired";

export function receivedTotalZat(inv: Invoice): number {
  return (inv.received_zat_pending ?? 0) + (inv.received_zat_confirmed ?? 0);
}

export function isPaymentComplete(inv: Invoice): boolean {
  return (inv.received_zat_confirmed ?? 0) >= inv.amount_zat;
}

export function isFullyPaid(inv: Invoice): boolean {
  return receivedTotalZat(inv) >= inv.amount_zat;
}

export function secondsUntilExpiry(expiresAt: string | null | undefined, nowMs: number): number | null {
  if (!expiresAt) return null;
  const t = Date.parse(expiresAt);
  if (!Number.isFinite(t)) return null;
  const diff = Math.floor((t - nowMs) / 1000);
  return diff < 0 ? 0 : diff;
}

export function formatCountdown(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${String(h)}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return `${String(m)}:${String(sec).padStart(2, "0")}`;
}

export function invoicePhase(inv: Invoice, nowMs: number): InvoicePhase {
  const exp = secondsUntilExpiry(inv.expires_at, nowMs);
  if (exp === 0 && !isFullyPaid(inv)) return "expired";
  if (isPaymentComplete(inv)) return "payment_complete";
  if (isFullyPaid(inv)) return "pending_confirmations";
  return "awaiting_payment";
}

export function depositHeightForConfirmations(events: InvoiceEvent[]): number | null {
  let h: number | null = null;
  for (const e of events) {
    const dh = e.deposit?.height;
    if (typeof dh !== "number") continue;
    if (!Number.isFinite(dh)) continue;
    if (h === null || dh > h) h = dh;
  }
  return h;
}

export function confirmationsCount(bestHeight: number | null, depositHeight: number | null): number | null {
  if (bestHeight === null || depositHeight === null) return null;
  if (!Number.isFinite(bestHeight) || !Number.isFinite(depositHeight)) return null;
  const confs = Math.floor(bestHeight) - Math.floor(depositHeight) + 1;
  return confs < 0 ? 0 : confs;
}


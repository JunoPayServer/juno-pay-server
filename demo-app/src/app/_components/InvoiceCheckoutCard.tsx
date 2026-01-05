"use client";

import { useEffect, useMemo, useState } from "react";
import QRCode from "react-qr-code";
import type { Invoice } from "@/app/actions";
import type { DemoOrder } from "@/lib/storage";
import { formatCountdown, invoicePhase, isFullyPaid, isPaymentComplete, receivedTotalZat, secondsUntilExpiry } from "@/lib/invoice";
import { useLiveInvoice } from "@/lib/useLiveInvoice";

function formatJUNO(zat: number): string {
  const whole = Math.floor(zat / 100_000_000);
  const frac = String(zat % 100_000_000).padStart(8, "0").replace(/0+$/, "");
  return frac ? `${whole}.${frac}` : String(whole);
}

function PhasePill({ phase }: { phase: "awaiting_payment" | "pending_confirmations" | "payment_complete" | "expired" }) {
  const cfg = {
    awaiting_payment: { label: "Awaiting payment", cls: "border-zinc-200 bg-white text-zinc-950" },
    pending_confirmations: { label: "Pending confirmations", cls: "border-amber-200 bg-amber-50 text-amber-900" },
    payment_complete: { label: "Payment complete", cls: "border-emerald-200 bg-emerald-50 text-emerald-900" },
    expired: { label: "Expired", cls: "border-red-200 bg-red-50 text-red-900" },
  } as const;
  const v = cfg[phase];
  return <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${v.cls}`}>{v.label}</span>;
}

function OrderSummary({ inv }: { inv: Invoice }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <div>
        <div className="text-xs text-zinc-500">Amount</div>
        <div className="mt-1 text-sm font-medium text-zinc-950">{formatJUNO(inv.amount_zat)} JUNO</div>
      </div>
      <div>
        <div className="text-xs text-zinc-500">Invoice status</div>
        <div className="mt-1 text-sm text-zinc-700">{inv.status}</div>
      </div>
      <div className="sm:col-span-2">
        <div className="text-xs text-zinc-500">Deposit address</div>
        <div className="mt-1 font-mono text-xs break-all text-zinc-950">{inv.address}</div>
      </div>
    </div>
  );
}

export function InvoiceCheckoutCard({
  order,
  onOrderUpdate,
}: {
  order: DemoOrder;
  onOrderUpdate?: (next: DemoOrder) => void;
}) {
  const live = useLiveInvoice({ invoice_id: order.invoice_id, invoice_token: order.invoice_token, cursor: order.events_cursor }, { pollMs: 1000 });
  const inv = live.invoice ?? {
    invoice_id: order.invoice_id,
    merchant_id: "unknown",
    external_order_id: order.external_order_id,
    status: order.status,
    address: order.address,
    amount_zat: order.amount_zat,
    required_confirmations: 100,
    received_zat_pending: order.received_zat_pending,
    received_zat_confirmed: order.received_zat_confirmed,
    expires_at: null,
    created_at: order.created_at,
    updated_at: order.updated_at,
  };

  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const phase = useMemo(() => invoicePhase(inv, nowMs), [inv, nowMs]);
  const expSec = useMemo(() => secondsUntilExpiry(inv.expires_at ?? null, nowMs), [inv.expires_at, nowMs]);
  const totalReceived = useMemo(() => receivedTotalZat(inv), [inv]);

  const confirmationsText = useMemo(() => {
    const required = inv.required_confirmations;
    if (!isFullyPaid(inv) || isPaymentComplete(inv)) return null;
    if (live.confirmations === null) return `Confirmations: —/${required}`;
    const cur = Math.min(live.confirmations, required);
    return `Confirmations: ${cur}/${required}`;
  }, [inv, live.confirmations]);

  const progress = useMemo(() => {
    const required = inv.required_confirmations;
    if (!isFullyPaid(inv) || isPaymentComplete(inv) || live.confirmations === null) return null;
    const cur = Math.min(live.confirmations, required);
    if (required <= 0) return null;
    return Math.min(1, Math.max(0, cur / required));
  }, [inv, live.confirmations]);

  const [copied, setCopied] = useState(false);
  async function copyAddress() {
    try {
      await navigator.clipboard.writeText(inv.address);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      setCopied(false);
    }
  }

  useEffect(() => {
    if (!live.invoice || !onOrderUpdate) return;
    const next: DemoOrder = {
      ...order,
      address: live.invoice.address,
      amount_zat: live.invoice.amount_zat,
      status: live.invoice.status,
      received_zat_pending: live.invoice.received_zat_pending,
      received_zat_confirmed: live.invoice.received_zat_confirmed,
      updated_at: live.invoice.updated_at,
      events_cursor: live.nextCursor,
    };
    if (
      next.status !== order.status ||
      next.received_zat_pending !== order.received_zat_pending ||
      next.received_zat_confirmed !== order.received_zat_confirmed ||
      next.updated_at !== order.updated_at ||
      next.events_cursor !== order.events_cursor
    ) {
      onOrderUpdate(next);
    }
  }, [live.invoice, live.nextCursor, onOrderUpdate, order]);

  return (
    <div className="rounded-md border border-zinc-200 bg-white p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm font-semibold text-zinc-950">Checkout</div>
        <div className="flex items-center gap-2">
          <PhasePill phase={phase} />
          {live.loading ? <div className="text-xs text-zinc-500">Updating…</div> : null}
        </div>
      </div>

      {live.error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-800">{live.error}</div>
      ) : null}

      <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-[180px_1fr]">
        <div className="flex items-center justify-center rounded-md border border-zinc-200 bg-white p-3">
          <QRCode value={inv.address} size={160} />
        </div>

        <div className="space-y-3">
          <OrderSummary inv={inv} />

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => void copyAddress()}
              className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50"
            >
              {copied ? "Copied" : "Copy address"}
            </button>

            {phase === "awaiting_payment" ? (
              <div className="text-xs text-zinc-600">
                {expSec === null ? "No expiry set" : expSec === 0 ? "Expired" : `Expires in ${formatCountdown(expSec)}`}
              </div>
            ) : null}

            {phase === "pending_confirmations" && confirmationsText ? <div className="text-xs text-zinc-600">{confirmationsText}</div> : null}
          </div>

          {phase === "pending_confirmations" && progress !== null ? (
            <div className="h-2 w-full overflow-hidden rounded-full bg-zinc-100">
              <div className="h-full bg-amber-500" style={{ width: `${Math.round(progress * 100)}%` }} />
            </div>
          ) : null}

          {!isFullyPaid(inv) && phase !== "expired" ? (
            <div className="text-xs text-zinc-600">
              Received: <span className="font-mono">{formatJUNO(totalReceived)} JUNO</span> /{" "}
              <span className="font-mono">{formatJUNO(inv.amount_zat)} JUNO</span>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}


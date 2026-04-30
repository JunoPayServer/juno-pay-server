"use client";

import { useEffect, useMemo, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { formatCountdown, formatJUNO } from "./format.js";
import { invoicePhase, isFullyPaid, isPaymentComplete, receivedTotalZat, secondsUntilExpiry } from "./invoice.js";
import type { JunoPayInvoice, JunoPayInvoicePhase } from "./types.js";

function PhasePill({ phase }: { phase: JunoPayInvoicePhase }) {
  const cfg = {
    awaiting_payment: { label: "Awaiting payment", cls: "th-border th-input th-muted" },
    pending_confirmations: { label: "Pending confirmations", cls: "border-amber-500/30 bg-amber-500/8 text-amber-400" },
    payment_complete: { label: "Payment complete", cls: "border-emerald-500/30 bg-emerald-500/8 text-emerald-400" },
    expired: { label: "Expired", cls: "border-red-500/30 bg-red-500/8 text-red-400" },
  } as const;
  const v = cfg[phase];
  return <span className={`inline-flex items-center justify-center rounded-full border w-40 py-0.5 text-xs font-medium ${v.cls}`}>{v.label}</span>;
}

function CopyToast({ visible }: { visible: boolean }) {
  return (
    <div className={`fixed bottom-6 left-1/2 -translate-x-1/2 z-100 flex items-center gap-2 px-4 py-2 rounded-lg bg-white border border-black/10 text-sm text-black shadow-xl transition-all duration-300 ${visible ? "opacity-100 translate-y-0" : "opacity-0 translate-y-2 pointer-events-none"}`}>
      Address Copied
      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" className="shrink-0 text-emerald-500"><path d="M3 8l4 4 6-7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
    </div>
  );
}

function CopyableAddress({ address }: { address: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(address);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }
  return (
    <>
      <CopyToast visible={copied} />
      <div className="flex items-center gap-1.5 mt-1">
        <span className="font-mono text-xs th-muted">{address.slice(0, 10)}...{address.slice(-10)}</span>
        <button type="button" onClick={() => void copy()} title="Copy address" className="shrink-0 th-faint hover:th-muted transition-colors">
          {copied ? (
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none"><path d="M3 8l4 4 6-7" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"/></svg>
          ) : (
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none"><rect x="5" y="1" width="9" height="11" rx="1.5" stroke="currentColor" strokeWidth="1.4"/><path d="M3 4H2a1 1 0 00-1 1v9a1 1 0 001 1h9a1 1 0 001-1v-1" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round"/></svg>
          )}
        </button>
      </div>
    </>
  );
}

function OrderSummary({ invoice }: { invoice: JunoPayInvoice }) {
  return (
    <div className="grid grid-cols-1 gap-3">
      <div>
        <div className="text-xs th-dim">Amount</div>
        <div className="mt-1 text-sm font-medium th-text">{formatJUNO(invoice.amount_zat)} JUNO</div>
      </div>
      <div>
        <div className="text-xs th-dim">Deposit address</div>
        <CopyableAddress address={invoice.address} />
      </div>
    </div>
  );
}

function QRModal({
  address,
  expSec,
  logoSrc,
  onClose,
}: {
  address: string;
  expSec: number | null;
  logoSrc?: string;
  onClose: () => void;
}) {
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm" onClick={onClose}>
      <div className="relative rounded-2xl th-modal border th-border p-8 shadow-2xl flex flex-col items-center" onClick={(e) => e.stopPropagation()}>
        <button
          type="button"
          onClick={onClose}
          className="absolute top-4 right-4 w-7 h-7 flex items-center justify-center rounded-full th-input th-muted hover:th-text transition-colors text-sm"
        >
          x
        </button>

        <div className="mb-1 text-xl font-semibold th-text">Scan to Pay</div>
        <div className="mb-6 text-sm th-muted">Use your JUNO wallet to scan</div>

        <div className="rounded-xl overflow-hidden bg-white p-5">
          <QRCodeSVG
            value={address}
            size={260}
            level="H"
            imageSettings={logoSrc ? { src: logoSrc, width: 56, height: 56, excavate: true } : undefined}
          />
        </div>

        {expSec !== null && (
          <div className="mt-4 text-xs th-muted">
            {expSec === 0 ? "Expired" : `Expires in ${formatCountdown(expSec)}`}
          </div>
        )}

        <div className="mt-4 w-full max-w-[330px] text-center">
          <div className="text-[10px] font-semibold uppercase tracking-wider th-faint mb-1.5">Deposit Address</div>
          <div className="flex items-center justify-center">
            <CopyableAddress address={address} />
          </div>
        </div>
      </div>
    </div>
  );
}

export function JunoPayCheckout({
  invoice,
  confirmations = null,
  error = null,
  hidePhasePill = false,
  logoSrc,
}: {
  invoice: JunoPayInvoice;
  confirmations?: number | null;
  error?: string | null;
  hidePhasePill?: boolean;
  logoSrc?: string;
}) {
  const [nowMs, setNowMs] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);

  const phase = useMemo(() => invoicePhase(invoice, nowMs), [invoice, nowMs]);
  const expSec = useMemo(() => secondsUntilExpiry(invoice.expires_at ?? null, nowMs), [invoice.expires_at, nowMs]);
  const totalReceived = useMemo(() => receivedTotalZat(invoice), [invoice]);

  const confirmationsText = useMemo(() => {
    const required = invoice.required_confirmations;
    if (!isFullyPaid(invoice) || isPaymentComplete(invoice)) return null;
    if (confirmations === null) return `Confirmations: -/${required}`;
    const cur = Math.min(confirmations, required);
    return `Confirmations: ${cur}/${required}`;
  }, [confirmations, invoice]);

  const progress = useMemo(() => {
    const required = invoice.required_confirmations;
    if (!isFullyPaid(invoice) || isPaymentComplete(invoice) || confirmations === null) return null;
    const cur = Math.min(confirmations, required);
    if (required <= 0) return null;
    return Math.min(1, Math.max(0, cur / required));
  }, [confirmations, invoice]);

  const [qrOpen, setQrOpen] = useState(false);

  return (
    <>
      {qrOpen && <QRModal address={invoice.address} expSec={phase === "awaiting_payment" ? expSec : null} logoSrc={logoSrc} onClose={() => setQrOpen(false)} />}

      <div className="rounded-2xl border th-border th-surface p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="text-sm font-semibold th-text">Checkout</div>
          {!hidePhasePill && <PhasePill phase={phase} />}
        </div>

        {error ? (
          <div className="mt-3 rounded-lg border border-red-500/30 bg-red-500/8 px-3 py-2 text-xs text-red-400">{error}</div>
        ) : null}

        <div className="mt-4 space-y-4">
          <OrderSummary invoice={invoice} />

          <button
            type="button"
            onClick={() => setQrOpen(true)}
            className="w-full flex items-center justify-center gap-2 rounded-xl border th-border th-input hover:border-[#dc8548]/40 hover:text-[#dc8548] th-muted transition-colors py-3 text-sm font-medium"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" className="shrink-0">
              <rect x="1" y="1" width="5" height="5" rx="0.5" stroke="currentColor" strokeWidth="1.2" fill="none"/>
              <rect x="2.5" y="2.5" width="2" height="2" fill="currentColor"/>
              <rect x="10" y="1" width="5" height="5" rx="0.5" stroke="currentColor" strokeWidth="1.2" fill="none"/>
              <rect x="11.5" y="2.5" width="2" height="2" fill="currentColor"/>
              <rect x="1" y="10" width="5" height="5" rx="0.5" stroke="currentColor" strokeWidth="1.2" fill="none"/>
              <rect x="2.5" y="11.5" width="2" height="2" fill="currentColor"/>
              <rect x="10" y="10" width="2" height="2" fill="currentColor"/>
              <rect x="13" y="10" width="2" height="2" fill="currentColor"/>
              <rect x="10" y="13" width="2" height="2" fill="currentColor"/>
              <rect x="13" y="13" width="2" height="2" fill="currentColor"/>
            </svg>
            Show QR Code
          </button>

          <div className="flex flex-wrap items-center gap-2">
            {phase === "awaiting_payment" && expSec !== null && expSec > 0 ? (
              <div className="text-xs th-dim">Expires in {formatCountdown(expSec)}</div>
            ) : null}

            {phase === "pending_confirmations" && confirmationsText ? (
              <div className="text-xs th-dim">{confirmationsText}</div>
            ) : null}
          </div>

          {phase === "pending_confirmations" && progress !== null ? (
            <div className="h-1.5 w-full overflow-hidden rounded-full th-input">
              <div className="h-full bg-amber-500 transition-all" style={{ width: `${Math.round(progress * 100)}%` }} />
            </div>
          ) : null}

          {!isFullyPaid(invoice) && phase !== "expired" ? (
            <div className="text-xs th-dim">
              Received: <span className="font-mono th-muted">{formatJUNO(totalReceived)} JUNO</span> /{" "}
              <span className="font-mono th-muted">{formatJUNO(invoice.amount_zat)} JUNO</span>
            </div>
          ) : null}
        </div>
      </div>
    </>
  );
}

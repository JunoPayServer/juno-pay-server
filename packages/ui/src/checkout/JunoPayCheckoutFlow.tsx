"use client";

import { useEffect, useMemo, useState } from "react";
import { JunoPayCheckout } from "./JunoPayCheckout.js";
import { useJunoPayInvoice } from "./useJunoPayInvoice.js";
import type { JunoPayClient } from "./api.js";
import type { JunoPayInvoice, JunoPayPublicInvoice } from "./types.js";

export type JunoPayCheckoutFlowState = {
  invoice: JunoPayInvoice;
  invoice_token: string;
  next_cursor: string;
};

export function JunoPayCreateInvoiceButton({
  createInvoice,
  buttonLabel = "Pay with JunoPay",
  creatingLabel = "Creating checkout...",
  className = "",
  onInvoiceCreated,
  onError,
}: {
  createInvoice: () => Promise<JunoPayPublicInvoice>;
  buttonLabel?: string;
  creatingLabel?: string;
  className?: string;
  onInvoiceCreated?: (invoice: JunoPayPublicInvoice) => void;
  onError?: (error: string) => void;
}) {
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

  async function startCheckout() {
    setCreateError(null);
    setCreating(true);
    try {
      const next = await createInvoice();
      onInvoiceCreated?.(next);
    } catch (e) {
      const message = e instanceof Error ? e.message : "create invoice failed";
      setCreateError(message);
      onError?.(message);
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className={className}>
      {createError ? (
        <div className="mb-3 rounded-lg border border-red-500/30 bg-red-500/8 px-3 py-2 text-xs text-red-400">{createError}</div>
      ) : null}
      <button type="button" disabled={creating} onClick={() => void startCheckout()} className="btn-gold text-black font-semibold px-5 py-2 rounded-lg text-sm disabled:opacity-60">
        {creating ? creatingLabel : buttonLabel}
      </button>
    </div>
  );
}

export function JunoPayCheckoutFlow({
  createInvoice,
  client,
  initialInvoice = null,
  buttonLabel = "Pay with JunoPay",
  creatingLabel = "Creating checkout...",
  className = "",
  pollMs = 1000,
  hidePhasePill = false,
  logoSrc,
  onInvoiceCreated,
  onInvoiceUpdated,
}: {
  createInvoice: () => Promise<JunoPayPublicInvoice>;
  client: Pick<JunoPayClient, "getPublicInvoice" | "getStatus" | "listPublicInvoiceEvents">;
  initialInvoice?: JunoPayCheckoutFlowState | null;
  buttonLabel?: string;
  creatingLabel?: string;
  className?: string;
  pollMs?: number;
  hidePhasePill?: boolean;
  logoSrc?: string;
  onInvoiceCreated?: (invoice: JunoPayPublicInvoice) => void;
  onInvoiceUpdated?: (state: JunoPayCheckoutFlowState) => void;
}) {
  const [checkout, setCheckout] = useState<JunoPayCheckoutFlowState | null>(initialInvoice);

  const input = useMemo(() => {
    if (!checkout) return null;
    return {
      invoice_id: checkout.invoice.invoice_id,
      invoice_token: checkout.invoice_token,
      cursor: checkout.next_cursor,
    };
  }, [checkout]);

  const live = useJunoPayInvoice(client, input, { pollMs });
  const invoice = live.invoice ?? checkout?.invoice ?? null;

  useEffect(() => {
    if (!live.invoice || !checkout) return;
    if (
      live.invoice.status === checkout.invoice.status &&
      live.invoice.received_zat_pending === checkout.invoice.received_zat_pending &&
      live.invoice.received_zat_confirmed === checkout.invoice.received_zat_confirmed &&
      live.invoice.updated_at === checkout.invoice.updated_at &&
      live.nextCursor === checkout.next_cursor
    ) {
      return;
    }

    const next = { invoice: live.invoice, invoice_token: checkout.invoice_token, next_cursor: live.nextCursor };
    setCheckout(next);
    onInvoiceUpdated?.(next);
  }, [checkout, live.invoice, live.nextCursor, onInvoiceUpdated]);

  if (!invoice) {
    return (
      <JunoPayCreateInvoiceButton
        className={className}
        createInvoice={createInvoice}
        buttonLabel={buttonLabel}
        creatingLabel={creatingLabel}
        onInvoiceCreated={(next) => {
          const state = { invoice: next.invoice, invoice_token: next.invoice_token, next_cursor: "0" };
          setCheckout(state);
          onInvoiceCreated?.(next);
          onInvoiceUpdated?.(state);
        }}
      />
    );
  }

  return (
    <div className={className}>
      <JunoPayCheckout
        invoice={invoice}
        confirmations={live.confirmations}
        error={live.error}
        hidePhasePill={hidePhasePill}
        logoSrc={logoSrc}
      />
    </div>
  );
}

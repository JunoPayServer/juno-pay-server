"use client";

import { useEffect, useMemo } from "react";
import { JunoPayCheckout, type JunoPayInvoice } from "@junopayserver/widgets";
import type { DemoOrder } from "@/lib/storage";
import { useLiveInvoice } from "@/lib/useLiveInvoice";

export function InvoiceCheckoutCard({
  order,
  onOrderUpdate,
  hidePhasePill = false,
}: {
  order: DemoOrder;
  onOrderUpdate?: (next: DemoOrder) => void;
  hidePhasePill?: boolean;
}) {
  const live = useLiveInvoice({ invoice_id: order.invoice_id, invoice_token: order.invoice_token, cursor: order.events_cursor }, { pollMs: 1000 });
  const invoice = useMemo<JunoPayInvoice>(() => live.invoice ?? {
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
  }, [live.invoice, order]);

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
    <JunoPayCheckout
      invoice={invoice}
      confirmations={live.confirmations}
      error={live.error}
      hidePhasePill={hidePhasePill}
      logoSrc="/juno-pay-server-logo.svg"
    />
  );
}

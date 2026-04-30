"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import {
  JunoPayCheckoutFlow,
  JunoPayCreateInvoiceButton,
  type JunoPayCheckoutFlowState,
  type JunoPayClient,
  type JunoPayInvoice,
} from "@junopayserver/widgets";
import { createAirInvoice, getPublicInvoice, getPublicStatus, listPublicInvoiceEvents } from "@/app/actions";
import { Sidebar } from "@/app/_components/Sidebar";
import { clearUser, loadOrders, loadOrCreateDemoUser, saveOrders, type DemoOrder, type DemoUser } from "@/lib/storage";
import { uuidv4 } from "@/lib/uuid";
import { formatJUNO } from "@/lib/format";

const junoPayClient: Pick<JunoPayClient, "getPublicInvoice" | "getStatus" | "listPublicInvoiceEvents"> = {
  getPublicInvoice,
  getStatus: getPublicStatus,
  listPublicInvoiceEvents,
};

function orderIDFromExternalOrderID(externalOrderID: string | undefined, fallback: string): string {
  const value = externalOrderID?.trim();
  if (!value) return fallback;
  const parts = value.split(":");
  return parts[parts.length - 1] || fallback;
}

function orderToInvoice(order: DemoOrder): JunoPayInvoice {
  return {
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
}

export default function HomePage() {
  const [user, setUser] = useState<DemoUser | null>(null);
  const [orders, setOrders] = useState<DemoOrder[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setUser(loadOrCreateDemoUser());
    setOrders(loadOrders());
  }, []);

  const lastOrder = useMemo(() => orders[0] ?? null, [orders]);
  const initialCheckout = useMemo<JunoPayCheckoutFlowState | null>(() => {
    if (!lastOrder) return null;
    return {
      invoice: orderToInvoice(lastOrder),
      invoice_token: lastOrder.invoice_token,
      next_cursor: lastOrder.events_cursor,
    };
  }, [lastOrder]);

  function handleReset() {
    clearUser();
    setOrders([]);
    setUser(loadOrCreateDemoUser());
  }

  function upsertCheckoutState(state: JunoPayCheckoutFlowState) {
    const invoice = state.invoice;
    const orderID = orderIDFromExternalOrderID(invoice.external_order_id, invoice.invoice_id);
    const nextOrder: DemoOrder = {
      order_id: orderID,
      external_order_id: invoice.external_order_id ?? orderID,
      invoice_id: invoice.invoice_id,
      invoice_token: state.invoice_token,
      address: invoice.address,
      amount_zat: invoice.amount_zat,
      status: invoice.status,
      received_zat_pending: invoice.received_zat_pending,
      received_zat_confirmed: invoice.received_zat_confirmed,
      created_at: invoice.created_at ?? new Date().toISOString(),
      updated_at: invoice.updated_at ?? new Date().toISOString(),
      events_cursor: state.next_cursor,
    };

    setOrders((prev) => {
      const next = [nextOrder, ...prev.filter((o) => o.order_id !== nextOrder.order_id)];
      saveOrders(next);
      return next;
    });
  }

  if (!user) {
    return (
      <div className="min-h-screen th-page flex items-center justify-center px-4">
        <div className="text-sm th-dim">Loading demo...</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen th-page flex">
      <Sidebar username={user.username ?? user.email} email={user.email} onReset={handleReset} />

      {/* Main content */}
      <div className="flex-1 ml-64 flex flex-col min-h-screen th-content">
        {/* Top bar */}
        <header className="flex items-center justify-between px-8 py-4 border-b th-border th-page-alpha backdrop-blur-md sticky top-0 z-30">
          <div>
            <h1 className="text-base font-semibold th-text">Dashboard</h1>
            <p className="text-xs th-dim mt-0.5">Buy a gallon of air for 1 JUNO</p>
          </div>
          <div className="text-xs th-faint">
            {orders.length > 0 && `${orders.length} order${orders.length > 1 ? "s" : ""}`}
          </div>
        </header>

        {/* Content */}
        <main className="flex-1 px-8 py-8 max-w-2xl">
          {error && (
            <div className="mb-6 rounded-lg border border-red-500/30 bg-red-500/8 px-4 py-3 text-sm text-red-400">
              {error}
            </div>
          )}

          {/* Buy card */}
          <div className="rounded-2xl border th-border th-surface p-6 mb-6">
            <h2 className="text-sm font-semibold th-muted uppercase tracking-wider mb-4">Store</h2>

            <div className="flex items-center justify-between rounded-xl border th-border th-input px-5 py-4">
              <div>
                <div className="text-sm font-medium th-text">1 gallon of air</div>
                <div className="text-xs th-dim mt-0.5">Price: 1 JUNO</div>
              </div>
              <JunoPayCreateInvoiceButton
                createInvoice={async () => {
                  setError(null);
                  const orderID = uuidv4();
                  const externalOrderID = `demo-air:${user.user_id}:${orderID}`;
                  const out = await createAirInvoice({ external_order_id: externalOrderID, demo_user_id: user.user_id, email: user.email });
                  if (!out.ok) throw new Error(out.error);
                  return out.data;
                }}
                buttonLabel="Buy"
                creatingLabel="Creating..."
                onInvoiceCreated={(invoice) => upsertCheckoutState({ invoice: invoice.invoice, invoice_token: invoice.invoice_token, next_cursor: "0" })}
                onError={setError}
              />
            </div>
          </div>

          {/* Latest invoice */}
          {lastOrder && initialCheckout ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between mb-2">
                <h2 className="text-sm font-semibold th-muted uppercase tracking-wider">Latest Invoice</h2>
                <Link href="/orders" className="text-xs text-[#dc8548] hover:text-[#e89a68] transition-colors">
                  View all ({orders.length}) →
                </Link>
              </div>
              <JunoPayCheckoutFlow
                key={lastOrder.order_id}
                client={junoPayClient}
                initialInvoice={initialCheckout}
                createInvoice={async () => {
                  setError(null);
                  const orderID = uuidv4();
                  const externalOrderID = `demo-air:${user.user_id}:${orderID}`;
                  const out = await createAirInvoice({ external_order_id: externalOrderID, demo_user_id: user.user_id, email: user.email });
                  if (!out.ok) throw new Error(out.error);
                  return out.data;
                }}
                onInvoiceCreated={(invoice) => upsertCheckoutState({ invoice: invoice.invoice, invoice_token: invoice.invoice_token, next_cursor: "0" })}
                onInvoiceUpdated={upsertCheckoutState}
                logoSrc="/juno-pay-server-logo.svg"
              />
            </div>
          ) : (
            <div className="rounded-2xl border th-border th-surface p-8 text-center">
              <div className="th-dim text-sm">No invoices yet. Buy something to get started.</div>
            </div>
          )}
        </main>

        {/* Footer */}
        <footer className="px-8 py-4 border-t th-border text-xs th-faint">
          Amounts in JUNO · stored as zatoshis
          {orders.length > 0 && (
            <span> · latest: <span className="font-mono">{formatJUNO(orders[0]!.amount_zat)} JUNO</span></span>
          )}
        </footer>
      </div>
    </div>
  );
}

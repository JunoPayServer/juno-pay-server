"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { createAirInvoice } from "@/app/actions";
import { InvoiceCheckoutCard } from "@/app/_components/InvoiceCheckoutCard";
import { Sidebar } from "@/app/_components/Sidebar";
import { clearUser, loadOrders, loadOrCreateDemoUser, saveOrders, type DemoOrder, type DemoUser } from "@/lib/storage";
import { uuidv4 } from "@/lib/uuid";
import { formatJUNO } from "@/lib/format";

export default function HomePage() {
  const [user, setUser] = useState<DemoUser | null>(null);
  const [orders, setOrders] = useState<DemoOrder[]>([]);
  const [buying, setBuying] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setUser(loadOrCreateDemoUser());
    setOrders(loadOrders());
  }, []);

  const lastOrder = useMemo(() => orders[0] ?? null, [orders]);

  function handleReset() {
    clearUser();
    setOrders([]);
    setUser(loadOrCreateDemoUser());
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
              <button
                type="button"
                disabled={buying}
                onClick={async () => {
                  setError(null);
                  setBuying(true);
                  try {
                    const orderID = uuidv4();
                    const externalOrderID = `demo-air:${user.user_id}:${orderID}`;
                    const out = await createAirInvoice({ external_order_id: externalOrderID, demo_user_id: user.user_id, email: user.email });
                    if (!out.ok) { setError(out.error); return; }
                    const o: DemoOrder = {
                      order_id: orderID,
                      external_order_id: externalOrderID,
                      invoice_id: out.data.invoice.invoice_id,
                      invoice_token: out.data.invoice_token,
                      address: out.data.invoice.address,
                      amount_zat: out.data.invoice.amount_zat,
                      status: out.data.invoice.status,
                      received_zat_pending: out.data.invoice.received_zat_pending,
                      received_zat_confirmed: out.data.invoice.received_zat_confirmed,
                      created_at: out.data.invoice.created_at,
                      updated_at: out.data.invoice.updated_at,
                      events_cursor: "0",
                    };
                    const next = [o, ...orders];
                    saveOrders(next);
                    setOrders(next);
                  } catch (e) {
                    setError(e instanceof Error ? e.message : "create invoice failed");
                  } finally {
                    setBuying(false);
                  }
                }}
                className="btn-gold text-black font-semibold px-5 py-2 rounded-lg text-sm"
              >
                {buying ? "Creating…" : "Buy"}
              </button>
            </div>
          </div>

          {/* Latest invoice */}
          {lastOrder ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between mb-2">
                <h2 className="text-sm font-semibold th-muted uppercase tracking-wider">Latest Invoice</h2>
                <Link href="/orders" className="text-xs text-[#dc8548] hover:text-[#e89a68] transition-colors">
                  View all ({orders.length}) →
                </Link>
              </div>
              <InvoiceCheckoutCard
                order={lastOrder}
                onOrderUpdate={(next) => {
                  setOrders((prev) => {
                    const updated = prev.map((o) => (o.order_id === next.order_id ? next : o));
                    saveOrders(updated);
                    return updated;
                  });
                }}
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

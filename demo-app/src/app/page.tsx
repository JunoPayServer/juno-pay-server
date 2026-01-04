"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { createAirInvoice, getPublicInvoice } from "@/app/actions";
import { clearUser, loadOrders, loadUser, saveOrders, saveUser, type DemoOrder, type DemoUser } from "@/lib/storage";
import { uuidv4 } from "@/lib/uuid";

function formatJUNO(zat: number): string {
  const whole = Math.floor(zat / 100_000_000);
  const frac = String(zat % 100_000_000).padStart(8, "0").replace(/0+$/, "");
  return frac ? `${whole}.${frac}` : String(whole);
}

export default function HomePage() {
  const [user, setUser] = useState<DemoUser | null>(null);
  const [orders, setOrders] = useState<DemoOrder[]>([]);

  const [email, setEmail] = useState("");
  const [buying, setBuying] = useState(false);
  const [refreshingLatest, setRefreshingLatest] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setUser(loadUser());
    setOrders(loadOrders());
  }, []);

  const lastOrder = useMemo(() => orders[0] ?? null, [orders]);

  async function refreshOne(o: DemoOrder) {
    const invRes = await getPublicInvoice({ invoice_id: o.invoice_id, invoice_token: o.invoice_token });
    if (!invRes.ok) {
      throw new Error(invRes.error);
    }
    const inv = invRes.data;
    return {
      ...o,
      status: inv.status,
      received_zat_pending: inv.received_zat_pending,
      received_zat_confirmed: inv.received_zat_confirmed,
      updated_at: inv.updated_at,
    } satisfies DemoOrder;
  }

  return (
    <div className="mx-auto max-w-3xl p-6">
      <header className="flex items-center justify-between">
        <div>
          <div className="text-sm font-semibold tracking-tight">Juno Pay Demo</div>
          <div className="text-xs text-zinc-600">Buy a gallon of air for 1 JUNO</div>
        </div>
        <nav className="flex items-center gap-3 text-sm">
          <Link className="text-zinc-700 hover:text-zinc-950" href="/">
            Home
          </Link>
          <Link className="text-zinc-700 hover:text-zinc-950" href="/orders">
            Orders
          </Link>
        </nav>
      </header>

      {error ? (
        <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div>
      ) : null}

      <section className="mt-8 rounded-lg border border-zinc-200 bg-white p-6">
        <h1 className="text-xl font-semibold tracking-tight">Juno Pay Server</h1>
        <p className="mt-2 text-sm text-zinc-600">
          A self-hosted payment server for the Juno network. Create invoices, track deposits, and (optionally) deliver events to webhooks.
        </p>
        <div className="mt-4 flex flex-wrap items-center gap-3 text-sm">
          <Link className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 font-medium text-zinc-950 hover:bg-zinc-50" href="https://github.com/Abdullah1738/juno-pay-server" target="_blank" rel="noreferrer">
            GitHub →
          </Link>
          <Link className="text-zinc-700 hover:text-zinc-950" href="#demo">
            See demo ↓
          </Link>
          <Link className="text-zinc-700 hover:text-zinc-950" href="/admin/">
            Admin →
          </Link>
        </div>
      </section>

      {!user ? (
        <section id="demo" className="mt-8 scroll-mt-6 rounded-lg border border-zinc-200 bg-white p-6">
          <h2 className="text-lg font-semibold tracking-tight">Register</h2>
          <p className="mt-1 text-sm text-zinc-600">Local-only registration (stored in your browser).</p>

          <form
            className="mt-6 space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              const v = email.trim();
              if (!v) return;
              const u: DemoUser = { user_id: uuidv4(), email: v };
              saveUser(u);
              setUser(u);
            }}
          >
            <div>
              <label className="block text-sm font-medium text-zinc-700" htmlFor="email">
                Email
              </label>
              <input
                id="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
                placeholder="you@example.com"
                required
              />
            </div>
            <button type="submit" className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800">
              Register
            </button>
          </form>
        </section>
      ) : (
        <section id="demo" className="mt-8 scroll-mt-6 rounded-lg border border-zinc-200 bg-white p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h2 className="text-lg font-semibold tracking-tight">Buy Air</h2>
              <p className="mt-1 text-sm text-zinc-600">
                Signed-in as <span className="font-mono text-xs">{user.email}</span>
              </p>
            </div>
            <button
              type="button"
              onClick={() => {
                clearUser();
                setUser(null);
              }}
              className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50"
            >
              Reset
            </button>
          </div>

          <div className="mt-6 flex items-center justify-between rounded-md border border-zinc-200 bg-zinc-50 px-4 py-3">
            <div>
              <div className="text-sm font-medium text-zinc-950">1 gallon of air</div>
              <div className="text-xs text-zinc-600">Price: 1 JUNO</div>
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
                  if (!out.ok) {
                    setError(out.error);
                    return;
                  }

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
              className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-60"
            >
              {buying ? "Creating invoice..." : "Buy"}
            </button>
          </div>

          {lastOrder ? (
            <div className="mt-6 rounded-md border border-zinc-200 bg-white p-4">
              <div className="text-sm font-semibold text-zinc-950">Latest order</div>
              <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <div className="text-xs text-zinc-500">Invoice ID</div>
                  <div className="mt-1 font-mono text-xs">{lastOrder.invoice_id}</div>
                </div>
                <div>
                  <div className="text-xs text-zinc-500">Status</div>
                  <div className="mt-1 text-sm">{lastOrder.status}</div>
                </div>
                <div className="sm:col-span-2">
                  <div className="text-xs text-zinc-500">Deposit address</div>
                  <div className="mt-1 font-mono text-xs break-all">{lastOrder.address}</div>
                </div>
              </div>

              <div className="mt-4 flex items-center gap-2">
                <button
                  type="button"
                  disabled={refreshingLatest}
                  onClick={async () => {
                    setError(null);
                    setRefreshingLatest(true);
                    try {
                      const updated = await refreshOne(lastOrder);
                      const next = [updated, ...orders.slice(1)];
                      saveOrders(next);
                      setOrders(next);
                    } catch (e) {
                      setError(e instanceof Error ? e.message : "refresh failed");
                    } finally {
                      setRefreshingLatest(false);
                    }
                  }}
                  className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50 disabled:opacity-60"
                >
                  {refreshingLatest ? "Refreshing..." : "Refresh status"}
                </button>
                <Link href="/orders" className="text-xs font-medium text-zinc-700 hover:text-zinc-950">
                  View all orders →
                </Link>
              </div>
            </div>
          ) : (
            <div className="mt-6 text-sm text-zinc-600">No orders yet.</div>
          )}
        </section>
      )}

      <section className="mt-8 text-xs text-zinc-600">
        Amounts are displayed in JUNO, stored/processed as zatoshis (zat).
        {orders.length ? (
          <>
            {" "}
            Total orders: <span className="font-mono">{orders.length}</span> (latest amount{" "}
            <span className="font-mono">{formatJUNO(orders[0]!.amount_zat)} JUNO</span>).
          </>
        ) : null}
      </section>
    </div>
  );
}

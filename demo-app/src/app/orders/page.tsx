"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { getPublicInvoice, listPublicInvoiceEvents, type InvoiceEvent } from "@/app/actions";
import { loadOrders, loadUser, saveOrders, type DemoOrder } from "@/lib/storage";

function formatJUNO(zat: number): string {
  const whole = Math.floor(zat / 100_000_000);
  const frac = String(zat % 100_000_000).padStart(8, "0").replace(/0+$/, "");
  return frac ? `${whole}.${frac}` : String(whole);
}

function EventRow({ e }: { e: InvoiceEvent }) {
  return (
    <div className="rounded-md border border-zinc-200 bg-white p-3 text-sm">
      <div className="flex items-center justify-between">
        <div className="font-mono text-xs text-zinc-600">{e.event_id}</div>
        <div className="text-xs text-zinc-600">{e.occurred_at}</div>
      </div>
      <div className="mt-1 font-medium text-zinc-950">{e.type}</div>
      {e.deposit ? (
        <div className="mt-1 text-xs text-zinc-700">
          deposit={e.deposit.txid}:{e.deposit.action_index} amount={e.deposit.amount_zat} height={e.deposit.height}
        </div>
      ) : null}
    </div>
  );
}

export default function OrdersPage() {
  const [orders, setOrders] = useState<DemoOrder[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [events, setEvents] = useState<InvoiceEvent[]>([]);
  const [eventsCursor, setEventsCursor] = useState("0");
  const [refreshingAll, setRefreshingAll] = useState(false);
  const [loadingEvents, setLoadingEvents] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setOrders(loadOrders());
  }, []);

  const selectedOrder = useMemo(() => orders.find((o) => o.order_id === selected) ?? null, [orders, selected]);

  async function refreshAll() {
    setError(null);
    setRefreshingAll(true);
    try {
      const next: DemoOrder[] = [];
      for (const o of orders) {
        const inv = await getPublicInvoice({ invoice_id: o.invoice_id, invoice_token: o.invoice_token });
        next.push({
          ...o,
          status: inv.status,
          received_zat_pending: inv.received_zat_pending,
          received_zat_confirmed: inv.received_zat_confirmed,
          updated_at: inv.updated_at,
        });
      }
      saveOrders(next);
      setOrders(next);
    } catch (e) {
      setError(e instanceof Error ? e.message : "refresh failed");
    } finally {
      setRefreshingAll(false);
    }
  }

  async function refreshEvents(o: DemoOrder) {
    setError(null);
    setLoadingEvents(true);
    try {
      const out = await listPublicInvoiceEvents({
        invoice_id: o.invoice_id,
        invoice_token: o.invoice_token,
        cursor: eventsCursor,
      });
      if (out.events.length) {
        setEvents((prev) => [...prev, ...out.events]);
        const nextCursor = out.next_cursor === "0" ? eventsCursor : out.next_cursor;
        setEventsCursor(nextCursor);

        const nextOrders = orders.map((x) => (x.order_id === o.order_id ? { ...x, events_cursor: nextCursor } : x));
        saveOrders(nextOrders);
        setOrders(nextOrders);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "events failed");
    } finally {
      setLoadingEvents(false);
    }
  }

  return (
    <div className="mx-auto max-w-5xl p-6">
      <header className="flex items-center justify-between">
        <div>
          <div className="text-sm font-semibold tracking-tight">Orders</div>
          <div className="text-xs text-zinc-600">Track invoice status and events</div>
        </div>
        <nav className="flex items-center gap-3 text-sm">
          <Link className="text-zinc-700 hover:text-zinc-950" href="/">
            Home
          </Link>
        </nav>
      </header>

      {error ? (
        <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">{error}</div>
      ) : null}

      {!loadUser() ? (
        <div className="mt-8 rounded-lg border border-zinc-200 bg-white p-6">
          <div className="text-sm text-zinc-700">Register first.</div>
          <div className="mt-2">
            <Link href="/" className="text-sm font-medium text-zinc-700 hover:text-zinc-950">
              Go to home →
            </Link>
          </div>
        </div>
      ) : null}

      <section className="mt-8 rounded-lg border border-zinc-200 bg-white p-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-zinc-950">Orders</h2>
          <button
            type="button"
            onClick={() => refreshAll()}
            disabled={refreshingAll}
            className="rounded-md bg-zinc-950 px-3 py-1.5 text-xs font-medium text-white hover:bg-zinc-800 disabled:opacity-60"
          >
            {refreshingAll ? "Refreshing..." : "Refresh all"}
          </button>
        </div>

        {orders.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No orders yet.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Order</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Invoice</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Amount</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Pending</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Confirmed</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {orders.map((o) => (
                  <tr key={o.order_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{o.order_id}</div>
                      <div className="text-xs text-zinc-500">{o.created_at}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{o.invoice_id}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{formatJUNO(o.amount_zat)} JUNO</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{formatJUNO(o.received_zat_pending)} JUNO</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{formatJUNO(o.received_zat_confirmed)} JUNO</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{o.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <button
                        type="button"
                        onClick={() => {
                          setSelected(o.order_id);
                          setEvents([]);
                          setEventsCursor(o.events_cursor || "0");
                        }}
                        className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50"
                      >
                        View
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {selectedOrder ? (
        <section className="mt-8 rounded-lg border border-zinc-200 bg-white p-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-zinc-950">Order detail</h2>
            <button
              type="button"
              onClick={() => setSelected(null)}
              className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50"
            >
              Close
            </button>
          </div>

          <div className="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <div className="text-xs text-zinc-500">Invoice ID</div>
              <div className="mt-1 font-mono text-xs">{selectedOrder.invoice_id}</div>
            </div>
            <div>
              <div className="text-xs text-zinc-500">Status</div>
              <div className="mt-1 text-sm">{selectedOrder.status}</div>
            </div>
            <div className="sm:col-span-2">
              <div className="text-xs text-zinc-500">Deposit address</div>
              <div className="mt-1 font-mono text-xs break-all">{selectedOrder.address}</div>
            </div>
          </div>

          <div className="mt-4 flex items-center gap-2">
            <button
              type="button"
              disabled={loadingEvents}
              onClick={() => refreshEvents(selectedOrder)}
              className="rounded-md bg-zinc-950 px-3 py-1.5 text-xs font-medium text-white hover:bg-zinc-800 disabled:opacity-60"
            >
              {loadingEvents ? "Loading..." : "Fetch events"}
            </button>
            <div className="text-xs text-zinc-500">cursor={eventsCursor}</div>
          </div>

          {events.length ? (
            <div className="mt-4 space-y-2">
              {events.map((e) => (
                <EventRow key={e.event_id} e={e} />
              ))}
            </div>
          ) : (
            <div className="mt-4 text-sm text-zinc-600">No events loaded.</div>
          )}
        </section>
      ) : null}
    </div>
  );
}

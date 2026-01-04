"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type EventDelivery, listEventDeliveries } from "@/lib/api";

export default function EventDeliveriesPage() {
  const [merchantID, setMerchantID] = useState("");
  const [sinkID, setSinkID] = useState("");
  const [status, setStatus] = useState("");

  const [deliveries, setDeliveries] = useState<EventDelivery[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    try {
      setRefreshing(true);
      setError(null);
      const out = await listEventDeliveries({
        merchant_id: merchantID.trim() || undefined,
        sink_id: sinkID.trim() || undefined,
        status: status.trim() || undefined,
        limit: "200",
      });
      setDeliveries(out);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Event Deliveries</h1>
        <p className="mt-1 text-sm text-zinc-600">Delivery attempts + retry state (debug view).</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-zinc-700">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="m_..."
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-zinc-700">Sink ID</label>
            <input
              value={sinkID}
              onChange={(e) => setSinkID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="sink_..."
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-zinc-700">Status</label>
            <input
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="pending|delivered|failed|..."
            />
          </div>
        </div>

        <div className="mt-4">
          <button
            type="button"
            onClick={() => refresh()}
            disabled={refreshing}
            className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-60"
          >
            {refreshing ? "Loading..." : "Apply Filters"}
          </button>
        </div>

        {error ? (
          <div className="mt-4">
            <ErrorBanner message={error} />
          </div>
        ) : null}
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Deliveries</h2>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : deliveries.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No deliveries.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Delivery</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Attempt</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Next Retry</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Last Error</th>
                </tr>
              </thead>
              <tbody>
                {deliveries.map((d) => (
                  <tr key={d.delivery_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{d.delivery_id}</div>
                      <div className="text-xs text-zinc-500">sink={d.sink_id}</div>
                      <div className="text-xs text-zinc-500">event={d.event_id}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.attempt}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.next_retry_at ?? <span className="text-xs text-zinc-500">—</span>}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      {d.last_error ? (
                        <pre className="max-w-[560px] overflow-x-auto whitespace-pre-wrap break-words rounded-md bg-zinc-50 p-2 text-xs text-zinc-800">
                          {d.last_error}
                        </pre>
                      ) : (
                        <span className="text-xs text-zinc-500">—</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

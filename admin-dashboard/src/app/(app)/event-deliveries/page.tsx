"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type EventDelivery, listEventDeliveries } from "@/lib/api";
import { inputCls, selectCls } from "@/lib/styles";
import { useListPage, urlParam } from "@/lib/useListPage";

export default function EventDeliveriesPage() {
  const [merchantID, setMerchantID] = useState(() => urlParam("merchant_id"));
  const [sinkID, setSinkID] = useState(() => urlParam("sink_id"));
  const [status, setStatus] = useState(() => urlParam("status"));

  const [deliveries, setDeliveries] = useState<EventDelivery[]>([]);
  const { loading, refreshing, error, syncURL, run } = useListPage("/event-deliveries");

  async function refresh(override?: { merchantID?: string; sinkID?: string; status?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      sinkID: override?.sinkID ?? sinkID,
      status: override?.status ?? status,
    };
    await run(async () => {
      syncURL({ merchant_id: next.merchantID, sink_id: next.sinkID, status: next.status });
      const out = await listEventDeliveries({
        merchant_id: next.merchantID.trim() || undefined,
        sink_id: next.sinkID.trim() || undefined,
        status: next.status.trim() || undefined,
        limit: "200",
      });
      setDeliveries(out);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Event Deliveries</h1>
        <p className="mt-1 text-xs th-dim">Delivery attempts + retry state (debug view).</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
          <div className="sm:col-span-2">
            <label className="block text-xs th-muted mb-1">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className={inputCls}
              placeholder="m_..."
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs th-muted mb-1">Sink ID</label>
            <input
              value={sinkID}
              onChange={(e) => setSinkID(e.target.value)}
              className={inputCls}
              placeholder="sink_..."
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs th-muted mb-1">Status</label>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className={selectCls}
            >
              <option value="">(any)</option>
              <option value="pending">pending</option>
              <option value="delivered">delivered</option>
              <option value="failed">failed</option>
            </select>
          </div>
        </div>

        <div className="mt-4">
          <button
            type="button"
            onClick={() => refresh()}
            disabled={refreshing}
            className="btn-gold rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
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

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Deliveries</h2>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : deliveries.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No deliveries.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Delivery</th>
                  <th className="border-b th-border px-3 py-2">Status</th>
                  <th className="border-b th-border px-3 py-2">Attempt</th>
                  <th className="border-b th-border px-3 py-2">Next Retry</th>
                  <th className="border-b th-border px-3 py-2">Last Error</th>
                </tr>
              </thead>
              <tbody>
                {deliveries.map((d) => (
                  <tr key={d.delivery_id} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-mono text-xs th-dim">{d.delivery_id}</div>
                      <div className="text-xs th-faint">sink={d.sink_id}</div>
                      <div className="text-xs th-faint">event={d.event_id}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.status}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.attempt}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.next_retry_at ?? <span className="text-xs th-faint">—</span>}</td>
                    <td className="border-b th-border px-3 py-2">
                      {d.last_error ? (
                        <pre className="max-w-[560px] overflow-x-auto whitespace-pre-wrap wrap-break-word rounded-lg th-input p-2 text-xs th-dim">
                          {d.last_error}
                        </pre>
                      ) : (
                        <span className="text-xs th-faint">—</span>
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

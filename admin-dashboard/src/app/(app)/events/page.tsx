"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type CloudEvent, listOutboundEvents } from "@/lib/api";
import { inputCls } from "@/lib/styles";
import { useListPage, urlParam } from "@/lib/useListPage";

export default function OutboundEventsPage() {
  const [merchantID, setMerchantID] = useState(() => urlParam("merchant_id"));
  const [events, setEvents] = useState<CloudEvent[]>([]);
  const { loading, refreshing, error, syncURL, run } = useListPage("/events");

  async function refresh(override?: { merchantID?: string }) {
    const next = { merchantID: override?.merchantID ?? merchantID };
    await run(async () => {
      syncURL({ merchant_id: next.merchantID });
      const out = await listOutboundEvents({ merchant_id: next.merchantID.trim() || undefined, limit: "100" });
      setEvents(out.events);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Outbound Events</h1>
        <p className="mt-1 text-xs th-dim">Durable outbox events emitted to sinks (debug view).</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1">
            <label className="block text-xs th-muted mb-1">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className={inputCls}
              placeholder="m_..."
            />
          </div>
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
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Events</h2>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : events.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No events.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Time</th>
                  <th className="border-b th-border px-3 py-2">Type</th>
                  <th className="border-b th-border px-3 py-2">Subject</th>
                  <th className="border-b th-border px-3 py-2">ID</th>
                  <th className="border-b th-border px-3 py-2">Data</th>
                </tr>
              </thead>
              <tbody>
                {events.map((ev) => (
                  <tr key={ev.id} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2 th-dim">{ev.time}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{ev.type}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{ev.subject}</td>
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-mono text-xs th-faint">{ev.id}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2">
                      <pre className="max-w-sm overflow-x-auto text-xs th-faint">{JSON.stringify(ev.data, null, 2)}</pre>
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

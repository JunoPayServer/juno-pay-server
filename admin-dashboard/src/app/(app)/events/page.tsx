"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type CloudEvent, listOutboundEvents } from "@/lib/api";

export default function OutboundEventsPage() {
  const router = useRouter();
  const [merchantID, setMerchantID] = useState("");
  const [events, setEvents] = useState<CloudEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function syncURL(next: { merchantID: string }) {
    const p = new URLSearchParams();
    if (next.merchantID.trim()) p.set("merchant_id", next.merchantID.trim());
    const q = p.toString();
    router.replace(`/events${q ? `?${q}` : ""}`);
  }

  async function refresh(override?: { merchantID?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
    };
    try {
      setRefreshing(true);
      setError(null);
      syncURL(next);
      const out = await listOutboundEvents({ merchant_id: next.merchantID.trim() || undefined, limit: "100" });
      setEvents(out.events);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    const sp = new URLSearchParams(window.location.search);
    const m = sp.get("merchant_id") ?? "";
    setMerchantID(m);
    void refresh({ merchantID: m });
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Outbound Events</h1>
        <p className="mt-1 text-sm text-zinc-600">Durable outbox events emitted to sinks (debug view).</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1">
            <label className="block text-sm font-medium text-zinc-700">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="m_..."
            />
          </div>
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
        <h2 className="text-sm font-semibold text-zinc-950">Events</h2>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : events.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No events.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Time</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Type</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Subject</th>
                  <th className="border-b border-zinc-200 px-3 py-2">ID</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Data</th>
                </tr>
              </thead>
              <tbody>
                {events.map((ev) => (
                  <tr key={ev.id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="text-xs">{ev.time}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{ev.type}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{ev.subject ?? <span className="text-xs text-zinc-500">—</span>}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{ev.id}</div>
                      <div className="text-xs text-zinc-500">{ev.source}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <pre className="max-w-[560px] overflow-x-auto whitespace-pre-wrap break-words rounded-md bg-zinc-50 p-2 text-xs text-zinc-800">
                        {JSON.stringify(ev.data, null, 2)}
                      </pre>
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

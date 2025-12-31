"use client";

import { useAdmin } from "@/components/AdminProvider";
import { ErrorBanner } from "@/components/ErrorBanner";

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-zinc-100 py-2">
      <div className="text-sm text-zinc-600">{label}</div>
      <div className="text-sm font-medium text-zinc-950">{value}</div>
    </div>
  );
}

export default function StatusPage() {
  const { status, loading, error } = useAdmin();

  if (loading && !status) {
    return <div className="text-sm text-zinc-600">Loading...</div>;
  }
  if (error) {
    return <ErrorBanner message={error} />;
  }
  if (!status) {
    return <div className="text-sm text-zinc-600">No status available.</div>;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Status</h1>
        <p className="mt-1 text-sm text-zinc-600">Backend sync, scanner health, and delivery backlog.</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Chain</h2>
        <div className="mt-3">
          <Row label="Best Height" value={status.chain.best_height} />
          <Row label="Best Hash" value={<span className="font-mono text-xs">{status.chain.best_hash}</span>} />
          <Row label="Uptime (s)" value={status.chain.uptime_seconds} />
        </div>
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Scanner</h2>
        <div className="mt-3">
          <Row label="Connected" value={status.scanner.connected ? "Yes" : "No"} />
          <Row label="Last Cursor Applied" value={status.scanner.last_cursor_applied} />
          <Row label="Last Event At" value={status.scanner.last_event_at ?? "—"} />
        </div>
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Event Delivery</h2>
        <div className="mt-3">
          <Row label="Pending Deliveries" value={status.event_delivery.pending_deliveries} />
        </div>
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Restarts</h2>
        <div className="mt-3">
          <Row label="junocashd Restarts Detected" value={status.restarts.junocashd_restarts_detected} />
          <Row label="Last Restart At" value={status.restarts.last_restart_at ?? "—"} />
        </div>
      </section>
    </div>
  );
}


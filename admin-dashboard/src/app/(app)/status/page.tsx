"use client";

import { useAdmin } from "@/components/AdminProvider";
import { ErrorBanner } from "@/components/ErrorBanner";
import { Row } from "@/components/Row";

export default function StatusPage() {
  const { status, loading, error } = useAdmin();

  if (loading && !status) {
    return <div className="text-sm th-dim">Loading...</div>;
  }
  if (error) {
    return <ErrorBanner message={error} />;
  }
  if (!status) {
    return <div className="text-sm th-dim">No status available.</div>;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Status</h1>
        <p className="mt-1 text-xs th-dim">Backend sync, scanner health, and delivery backlog.</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Chain</h2>
        <div className="mt-3">
          <Row label="Best Height" value={status.chain.best_height} />
          <Row label="Best Hash" value={<span className="font-mono text-xs">{status.chain.best_hash}</span>} />
          <Row label="Uptime (s)" value={status.chain.uptime_seconds ?? "—"} />
        </div>
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Scanner</h2>
        <div className="mt-3">
          <Row label="Connected" value={status.scanner.connected ? "Yes" : "No"} />
          <Row label="Last Cursor Applied" value={status.scanner.last_cursor_applied} />
          <Row label="Last Event At" value={status.scanner.last_event_at ?? "—"} />
        </div>
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Event Delivery</h2>
        <div className="mt-3">
          <Row label="Pending Deliveries" value={status.event_delivery.pending_deliveries} />
        </div>
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Restarts</h2>
        <div className="mt-3">
          <Row label="junocashd Restarts Detected" value={status.restarts.junocashd_restarts_detected} />
          <Row label="Last Restart At" value={status.restarts.last_restart_at ?? "—"} />
        </div>
      </section>
    </div>
  );
}

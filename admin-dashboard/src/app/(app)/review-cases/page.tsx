"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type ReviewCase, listReviewCases, rejectReviewCase, resolveReviewCase } from "@/lib/api";

export default function ReviewCasesPage() {
  const [merchantID, setMerchantID] = useState("");
  const [status, setStatus] = useState("open");

  const [cases, setCases] = useState<ReviewCase[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [acting, setActing] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    try {
      setRefreshing(true);
      setError(null);
      const out = await listReviewCases({
        merchant_id: merchantID.trim() || undefined,
        status: status.trim() || undefined,
      });
      setCases(out);
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
        <h1 className="text-lg font-semibold tracking-tight">Review Cases</h1>
        <p className="mt-1 text-sm text-zinc-600">Manual review queue (late, overpay, partial, unknown).</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label className="block text-sm font-medium text-zinc-700">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="m_..."
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Status</label>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            >
              <option value="">(any)</option>
              <option value="open">open</option>
              <option value="resolved">resolved</option>
              <option value="rejected">rejected</option>
            </select>
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
        <h2 className="text-sm font-semibold text-zinc-950">Cases</h2>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : cases.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No cases.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Review</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Reason</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Invoice</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Notes</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {cases.map((c) => (
                  <tr key={c.review_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{c.review_id}</div>
                      <div className="text-xs text-zinc-500">{c.created_at}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{c.reason}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{c.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      {c.invoice_id ? <span className="font-mono text-xs">{c.invoice_id}</span> : "—"}
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="max-w-sm truncate text-xs text-zinc-700" title={c.notes}>
                        {c.notes}
                      </div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      {c.status === "open" ? (
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            disabled={acting === `${c.review_id}:resolve` || acting === `${c.review_id}:reject`}
                            onClick={async () => {
                              const notes = window.prompt("Resolve notes:");
                              if (!notes) return;
                              setError(null);
                              setActing(`${c.review_id}:resolve`);
                              try {
                                await resolveReviewCase(c.review_id, notes);
                                await refresh();
                              } catch (e) {
                                setError(e instanceof Error ? e.message : "resolve failed");
                              } finally {
                                setActing(null);
                              }
                            }}
                            className="rounded-md bg-zinc-950 px-3 py-1.5 text-xs font-medium text-white hover:bg-zinc-800 disabled:opacity-60"
                          >
                            {acting === `${c.review_id}:resolve` ? "Resolving..." : "Resolve"}
                          </button>
                          <button
                            type="button"
                            disabled={acting === `${c.review_id}:resolve` || acting === `${c.review_id}:reject`}
                            onClick={async () => {
                              const notes = window.prompt("Reject notes:");
                              if (!notes) return;
                              setError(null);
                              setActing(`${c.review_id}:reject`);
                              try {
                                await rejectReviewCase(c.review_id, notes);
                                await refresh();
                              } catch (e) {
                                setError(e instanceof Error ? e.message : "reject failed");
                              } finally {
                                setActing(null);
                              }
                            }}
                            className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-950 hover:bg-zinc-50 disabled:opacity-60"
                          >
                            {acting === `${c.review_id}:reject` ? "Rejecting..." : "Reject"}
                          </button>
                        </div>
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

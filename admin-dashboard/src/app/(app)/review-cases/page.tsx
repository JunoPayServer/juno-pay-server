"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type ReviewCase, listReviewCases, rejectReviewCase, resolveReviewCase } from "@/lib/api";
import { hydrateInvoiceLabels } from "@/lib/hydrateInvoiceLabels";
import { inputCls, selectCls } from "@/lib/styles";
import { useListPage, urlParam } from "@/lib/useListPage";

export default function ReviewCasesPage() {
  const [merchantID, setMerchantID] = useState(() => urlParam("merchant_id"));
  const [status, setStatus] = useState(() => urlParam("status", "open"));

  const [cases, setCases] = useState<ReviewCase[]>([]);
  const [invoiceLabels, setInvoiceLabels] = useState<Record<string, string>>({});
  const [acting, setActing] = useState<string | null>(null);
  const { loading, refreshing, error, setError, syncURL, run } = useListPage("/review-cases");

  async function refresh(override?: { merchantID?: string; status?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      status: override?.status ?? status,
    };
    await run(async () => {
      syncURL({ merchant_id: next.merchantID, status: next.status });
      const out = await listReviewCases({
        merchant_id: next.merchantID.trim() || undefined,
        status: next.status.trim() || undefined,
      });
      setCases(out);
      void hydrateInvoiceLabels(out.map((c) => c.invoice_id), invoiceLabels, setInvoiceLabels);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Review Cases</h1>
        <p className="mt-1 text-xs th-dim">Manual review queue (late, overpay, partial, unknown).</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label className="block text-xs th-muted mb-1">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className={inputCls}
              placeholder="m_..."
            />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Status</label>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className={selectCls}
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
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Cases</h2>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : cases.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No cases.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Review</th>
                  <th className="border-b th-border px-3 py-2">Reason</th>
                  <th className="border-b th-border px-3 py-2">Status</th>
                  <th className="border-b th-border px-3 py-2">Invoice</th>
                  <th className="border-b th-border px-3 py-2">Notes</th>
                  <th className="border-b th-border px-3 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {cases.map((c) => (
                  <tr key={c.review_id} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-mono text-xs th-dim">{c.review_id}</div>
                      <div className="text-xs th-faint">{c.created_at}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{c.reason}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{c.status}</td>
                    <td className="border-b th-border px-3 py-2">
                      {c.invoice_id ? (
                        <div>
                          <Link
                            href={`/invoice?invoice_id=${encodeURIComponent(c.invoice_id)}`}
                            className="th-text hover:text-[#dc8548] transition-colors font-medium"
                          >
                            {invoiceLabels[c.invoice_id] ?? "Invoice"}
                          </Link>
                          <div className="font-mono text-xs th-faint">{c.invoice_id}</div>
                        </div>
                      ) : "—"}
                    </td>
                    <td className="border-b th-border px-3 py-2">
                      <div className="max-w-sm truncate text-xs th-dim" title={c.notes}>{c.notes}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2">
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
                            className="btn-gold rounded-lg px-3 py-1.5 text-xs font-medium text-white disabled:opacity-60"
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
                            className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-xs transition-colors disabled:opacity-60"
                          >
                            {acting === `${c.review_id}:reject` ? "Rejecting..." : "Reject"}
                          </button>
                        </div>
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

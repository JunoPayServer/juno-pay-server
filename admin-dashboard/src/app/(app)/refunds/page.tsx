"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type Refund, createRefund, listRefunds } from "@/lib/api";
import { hydrateInvoiceLabels } from "@/lib/hydrateInvoiceLabels";
import { inputCls, selectCls } from "@/lib/styles";
import { useListPage, urlParam } from "@/lib/useListPage";

export default function RefundsPage() {
  const [merchantID, setMerchantID] = useState(() => urlParam("merchant_id"));
  const [invoiceID, setInvoiceID] = useState(() => urlParam("invoice_id"));
  const [status, setStatus] = useState(() => urlParam("status"));

  const [refunds, setRefunds] = useState<Refund[]>([]);
  const [invoiceLabels, setInvoiceLabels] = useState<Record<string, string>>({});
  const { loading, refreshing, error, setError, syncURL, run } = useListPage("/refunds");

  const [toAddress, setToAddress] = useState("");
  const [amountZat, setAmountZat] = useState("0");
  const [notes, setNotes] = useState("");
  const [externalRefundID, setExternalRefundID] = useState("");
  const [sentTxID, setSentTxID] = useState("");
  const [creating, setCreating] = useState(false);

  async function refresh(override?: { merchantID?: string; invoiceID?: string; status?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      invoiceID: override?.invoiceID ?? invoiceID,
      status: override?.status ?? status,
    };
    await run(async () => {
      syncURL({ merchant_id: next.merchantID, invoice_id: next.invoiceID, status: next.status });
      const out = await listRefunds({
        merchant_id: next.merchantID.trim() || undefined,
        invoice_id: next.invoiceID.trim() || undefined,
        status: next.status.trim() || undefined,
        limit: "100",
      });
      setRefunds(out.refunds);
      void hydrateInvoiceLabels(out.refunds.map((r) => r.invoice_id), invoiceLabels, setInvoiceLabels);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Refunds</h1>
        <p className="mt-1 text-xs th-dim">Manual refund recordkeeping (no signing/broadcast).</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Create Refund</h2>
        <form
          className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
          onSubmit={async (e) => {
            e.preventDefault();
            setError(null);
            try {
              setCreating(true);
              const amt = Number.parseInt(amountZat, 10);
              await createRefund({
                merchant_id: merchantID.trim(),
                invoice_id: invoiceID.trim() || undefined,
                external_refund_id: externalRefundID.trim() || undefined,
                to_address: toAddress.trim(),
                amount_zat: Number.isFinite(amt) ? amt : 0,
                sent_txid: sentTxID.trim() || undefined,
                notes: notes.trim() || undefined,
              });
              setToAddress("");
              setAmountZat("0");
              setNotes("");
              setExternalRefundID("");
              setSentTxID("");
              await refresh();
            } catch (e) {
              setError(e instanceof Error ? e.message : "create failed");
            } finally {
              setCreating(false);
            }
          }}
        >
          <div>
            <label className="block text-xs th-muted mb-1">Merchant ID</label>
            <input value={merchantID} onChange={(e) => setMerchantID(e.target.value)} className={inputCls} placeholder="m_..." required />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Invoice ID (optional)</label>
            <input value={invoiceID} onChange={(e) => setInvoiceID(e.target.value)} className={inputCls} placeholder="inv_..." />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs th-muted mb-1">To Address</label>
            <input value={toAddress} onChange={(e) => setToAddress(e.target.value)} className={inputCls} placeholder="j1..." required />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Amount (zat)</label>
            <input type="number" min={1} value={amountZat} onChange={(e) => setAmountZat(e.target.value)} className={inputCls} required />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Status Filter</label>
            <select value={status} onChange={(e) => setStatus(e.target.value)} className={selectCls}>
              <option value="">(any)</option>
              <option value="requested">requested</option>
              <option value="sent">sent</option>
              <option value="canceled">canceled</option>
            </select>
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">External Refund ID (optional)</label>
            <input value={externalRefundID} onChange={(e) => setExternalRefundID(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">Sent TxID (optional)</label>
            <input value={sentTxID} onChange={(e) => setSentTxID(e.target.value)} className={inputCls} />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs th-muted mb-1">Notes (optional)</label>
            <textarea value={notes} onChange={(e) => setNotes(e.target.value)} className="h-24 w-full rounded-lg border th-border th-input th-text px-3 py-2 text-sm focus:outline-none" />
          </div>
          <div className="sm:col-span-2 flex items-center gap-2">
            <button type="submit" disabled={creating} className="btn-gold rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-60">
              {creating ? "Creating..." : "Create Refund"}
            </button>
            <button type="button" onClick={() => refresh()} disabled={refreshing} className="rounded-lg border th-border th-hover th-muted px-4 py-2 text-sm transition-colors disabled:opacity-60">
              {refreshing ? "Refreshing..." : "Refresh List"}
            </button>
          </div>
        </form>

        {error ? (
          <div className="mt-4">
            <ErrorBanner message={error} />
          </div>
        ) : null}
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Refunds</h2>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : refunds.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No refunds.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Refund</th>
                  <th className="border-b th-border px-3 py-2">Merchant</th>
                  <th className="border-b th-border px-3 py-2">Invoice</th>
                  <th className="border-b th-border px-3 py-2">Amount</th>
                  <th className="border-b th-border px-3 py-2">Status</th>
                  <th className="border-b th-border px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {refunds.map((r) => (
                  <tr key={r.refund_id} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-mono text-xs th-dim">{r.refund_id}</div>
                      <div className="text-xs th-faint truncate" title={r.to_address}>{r.to_address}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2">
                      <span className="font-mono text-xs th-dim">{r.merchant_id}</span>
                    </td>
                    <td className="border-b th-border px-3 py-2">
                      {r.invoice_id ? (
                        <div>
                          <Link href={`/invoice?invoice_id=${encodeURIComponent(r.invoice_id)}`} className="th-text hover:text-[#dc8548] transition-colors font-medium">
                            {invoiceLabels[r.invoice_id] ?? "Invoice"}
                          </Link>
                          <div className="font-mono text-xs th-faint">{r.invoice_id}</div>
                        </div>
                      ) : "—"}
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{r.amount_zat}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{r.status}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{r.updated_at}</td>
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

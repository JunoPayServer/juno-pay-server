"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type Refund, createRefund, listRefunds } from "@/lib/api";

export default function RefundsPage() {
  const [merchantID, setMerchantID] = useState("");
  const [invoiceID, setInvoiceID] = useState("");
  const [status, setStatus] = useState("");

  const [refunds, setRefunds] = useState<Refund[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [toAddress, setToAddress] = useState("");
  const [amountZat, setAmountZat] = useState("0");
  const [notes, setNotes] = useState("");
  const [externalRefundID, setExternalRefundID] = useState("");
  const [sentTxID, setSentTxID] = useState("");

  async function refresh() {
    try {
      setError(null);
      const out = await listRefunds({
        merchant_id: merchantID.trim() || undefined,
        invoice_id: invoiceID.trim() || undefined,
        status: status.trim() || undefined,
        limit: "100",
      });
      setRefunds(out.refunds);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Refunds</h1>
        <p className="mt-1 text-sm text-zinc-600">Manual refund recordkeeping (no signing/broadcast).</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Create Refund</h2>
        <form
          className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2"
          onSubmit={async (e) => {
            e.preventDefault();
            setError(null);
            try {
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
            }
          }}
        >
          <div>
            <label className="block text-sm font-medium text-zinc-700">Merchant ID</label>
            <input
              value={merchantID}
              onChange={(e) => setMerchantID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="m_..."
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Invoice ID (optional)</label>
            <input
              value={invoiceID}
              onChange={(e) => setInvoiceID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="inv_..."
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-zinc-700">To Address</label>
            <input
              value={toAddress}
              onChange={(e) => setToAddress(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="j1..."
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Amount (zat)</label>
            <input
              type="number"
              min={1}
              value={amountZat}
              onChange={(e) => setAmountZat(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              required
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Status Filter</label>
            <select
              value={status}
              onChange={(e) => setStatus(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            >
              <option value="">(any)</option>
              <option value="requested">requested</option>
              <option value="sent">sent</option>
              <option value="canceled">canceled</option>
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">External Refund ID (optional)</label>
            <input
              value={externalRefundID}
              onChange={(e) => setExternalRefundID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">Sent TxID (optional)</label>
            <input
              value={sentTxID}
              onChange={(e) => setSentTxID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            />
          </div>
          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-zinc-700">Notes (optional)</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="mt-1 h-24 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
            />
          </div>
          <div className="sm:col-span-2">
            <button
              type="submit"
              className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800"
            >
              Create Refund
            </button>
            <button
              type="button"
              onClick={() => refresh()}
              className="ml-2 rounded-md border border-zinc-200 bg-white px-4 py-2 text-sm font-medium text-zinc-950 hover:bg-zinc-50"
            >
              Refresh List
            </button>
          </div>
        </form>

        {error ? (
          <div className="mt-4">
            <ErrorBanner message={error} />
          </div>
        ) : null}
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Refunds</h2>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : refunds.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No refunds.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Refund</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Merchant</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Invoice</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Amount</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {refunds.map((r) => (
                  <tr key={r.refund_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{r.refund_id}</div>
                      <div className="text-xs text-zinc-500 truncate" title={r.to_address}>
                        {r.to_address}
                      </div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <span className="font-mono text-xs">{r.merchant_id}</span>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      {r.invoice_id ? <span className="font-mono text-xs">{r.invoice_id}</span> : "—"}
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{r.amount_zat}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{r.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{r.updated_at}</td>
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


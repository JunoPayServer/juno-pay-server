"use client";

import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type Deposit, listDeposits } from "@/lib/api";

export default function DepositsPage() {
  const [merchantID, setMerchantID] = useState("");
  const [invoiceID, setInvoiceID] = useState("");
  const [txid, setTxID] = useState("");

  const [deposits, setDeposits] = useState<Deposit[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  async function refresh(override?: { merchantID?: string; invoiceID?: string; txid?: string }) {
    try {
      setError(null);
      const out = await listDeposits({
        merchant_id: (override?.merchantID ?? merchantID).trim() || undefined,
        invoice_id: (override?.invoiceID ?? invoiceID).trim() || undefined,
        txid: (override?.txid ?? txid).trim() || undefined,
        limit: "100",
      });
      setDeposits(out.deposits);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    const inv = new URLSearchParams(window.location.search).get("invoice_id") ?? "";
    setInvoiceID(inv);
    void refresh({ invoiceID: inv });
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Deposits</h1>
        <p className="mt-1 text-sm text-zinc-600">Detected deposits (attributed and unattributed).</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
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
            <label className="block text-sm font-medium text-zinc-700">Invoice ID</label>
            <input
              value={invoiceID}
              onChange={(e) => setInvoiceID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="inv_..."
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-zinc-700">TxID</label>
            <input
              value={txid}
              onChange={(e) => setTxID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="..."
            />
          </div>
        </div>

        <div className="mt-4">
          <button
            type="button"
            onClick={() => refresh()}
            className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800"
          >
            Apply Filters
          </button>
        </div>

        {error ? (
          <div className="mt-4">
            <ErrorBanner message={error} />
          </div>
        ) : null}
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Deposits</h2>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : deposits.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No deposits.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Tx</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Amount</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Height</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Invoice</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Detected At</th>
                </tr>
              </thead>
              <tbody>
                {deposits.map((d) => (
                  <tr key={`${d.wallet_id}-${d.txid}-${d.action_index}`} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{d.txid}</div>
                      <div className="text-xs text-zinc-500">action_index={d.action_index}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.amount_zat}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.height}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      {d.invoice_id ? (
                        <span className="font-mono text-xs">{d.invoice_id}</span>
                      ) : (
                        <span className="text-xs text-zinc-500">—</span>
                      )}
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{d.detected_at}</td>
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

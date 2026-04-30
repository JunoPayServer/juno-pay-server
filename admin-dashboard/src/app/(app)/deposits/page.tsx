"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type Deposit, listDeposits } from "@/lib/api";
import { hydrateInvoiceLabels } from "@/lib/hydrateInvoiceLabels";
import { inputCls } from "@/lib/styles";
import { useListPage, urlParam } from "@/lib/useListPage";

export default function DepositsPage() {
  const [merchantID, setMerchantID] = useState(() => urlParam("merchant_id"));
  const [invoiceID, setInvoiceID] = useState(() => urlParam("invoice_id"));
  const [txid, setTxID] = useState(() => urlParam("txid"));

  const [deposits, setDeposits] = useState<Deposit[]>([]);
  const [invoiceLabels, setInvoiceLabels] = useState<Record<string, string>>({});
  const { loading, refreshing, error, syncURL, run } = useListPage("/deposits");

  async function refresh(override?: { merchantID?: string; invoiceID?: string; txid?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      invoiceID: override?.invoiceID ?? invoiceID,
      txid: override?.txid ?? txid,
    };
    await run(async () => {
      syncURL({ merchant_id: next.merchantID, invoice_id: next.invoiceID, txid: next.txid });
      const out = await listDeposits({
        merchant_id: next.merchantID.trim() || undefined,
        invoice_id: next.invoiceID.trim() || undefined,
        txid: next.txid.trim() || undefined,
        limit: "100",
      });
      setDeposits(out.deposits);
      void hydrateInvoiceLabels(out.deposits.map((d) => d.invoice_id), invoiceLabels, setInvoiceLabels);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Deposits</h1>
        <p className="mt-1 text-xs th-dim">Detected deposits (attributed and unattributed).</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
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
            <label className="block text-xs th-muted mb-1">Invoice ID</label>
            <input
              value={invoiceID}
              onChange={(e) => setInvoiceID(e.target.value)}
              className={inputCls}
              placeholder="inv_..."
            />
          </div>
          <div>
            <label className="block text-xs th-muted mb-1">TxID</label>
            <input
              value={txid}
              onChange={(e) => setTxID(e.target.value)}
              className={inputCls}
              placeholder="..."
            />
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
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Deposits</h2>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : deposits.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No deposits.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Tx</th>
                  <th className="border-b th-border px-3 py-2">Amount</th>
                  <th className="border-b th-border px-3 py-2">Height</th>
                  <th className="border-b th-border px-3 py-2">Status</th>
                  <th className="border-b th-border px-3 py-2">Invoice</th>
                  <th className="border-b th-border px-3 py-2">Detected At</th>
                </tr>
              </thead>
              <tbody>
                {deposits.map((d) => (
                  <tr key={`${d.wallet_id}-${d.txid}-${d.action_index}`} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-mono text-xs th-dim">{d.txid}</div>
                      <div className="text-xs th-faint">action_index={d.action_index}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.amount_zat}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.height}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.status}</td>
                    <td className="border-b th-border px-3 py-2">
                      {d.invoice_id ? (
                        <div>
                          <Link
                            href={`/invoice?invoice_id=${encodeURIComponent(d.invoice_id)}`}
                            className="th-text hover:text-[#dc8548] transition-colors font-medium"
                          >
                            {invoiceLabels[d.invoice_id] ?? "Invoice"}
                          </Link>
                          <div className="font-mono text-xs th-faint">{d.invoice_id}</div>
                        </div>
                      ) : (
                        <span className="text-xs th-faint">—</span>
                      )}
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{d.detected_at}</td>
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

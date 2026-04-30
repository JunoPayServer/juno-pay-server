"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type Invoice, listInvoices } from "@/lib/api";
import { inputCls, selectCls } from "@/lib/styles";
import { useListPage, urlParam } from "@/lib/useListPage";

export default function InvoicesPage() {
  const [merchantID, setMerchantID] = useState(() => urlParam("merchant_id"));
  const [status, setStatus] = useState(() => urlParam("status"));
  const [externalOrderID, setExternalOrderID] = useState(() => urlParam("external_order_id"));

  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const { loading, refreshing, error, syncURL, run } = useListPage("/invoices");

  async function refresh(override?: { merchantID?: string; status?: string; externalOrderID?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      status: override?.status ?? status,
      externalOrderID: override?.externalOrderID ?? externalOrderID,
    };
    await run(async () => {
      syncURL({ merchant_id: next.merchantID, status: next.status, external_order_id: next.externalOrderID });
      const out = await listInvoices({
        merchant_id: next.merchantID.trim() || undefined,
        status: next.status.trim() || undefined,
        external_order_id: next.externalOrderID.trim() || undefined,
        limit: "100",
      });
      setInvoices(out.invoices);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Invoices</h1>
        <p className="mt-1 text-xs th-dim">List invoices with filters.</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
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
              <option value="partial_pending">partial_pending</option>
              <option value="pending">pending</option>
              <option value="partial_confirmed">partial_confirmed</option>
              <option value="confirmed">confirmed</option>
              <option value="overpaid">overpaid</option>
              <option value="expired">expired</option>
              <option value="paid_late">paid_late</option>
              <option value="canceled">canceled</option>
            </select>
          </div>
          <div className="sm:col-span-2">
            <label className="block text-xs th-muted mb-1">External Order ID</label>
            <input
              value={externalOrderID}
              onChange={(e) => setExternalOrderID(e.target.value)}
              className={inputCls}
              placeholder="order-123"
            />
          </div>
        </div>

        <div className="mt-4 flex items-center gap-2">
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
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Invoices</h2>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : invoices.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No invoices.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Invoice</th>
                  <th className="border-b th-border px-3 py-2">Merchant</th>
                  <th className="border-b th-border px-3 py-2">Amount</th>
                  <th className="border-b th-border px-3 py-2">Received</th>
                  <th className="border-b th-border px-3 py-2">Status</th>
                  <th className="border-b th-border px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {invoices.map((inv) => (
                  <tr key={inv.invoice_id} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-medium">
                        <Link href={`/invoice?invoice_id=${encodeURIComponent(inv.invoice_id)}`} className="th-text hover:text-[#dc8548] transition-colors">
                          {inv.external_order_id}
                        </Link>
                      </div>
                      <div className="font-mono text-xs th-faint">{inv.invoice_id}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-mono text-xs th-dim">{inv.merchant_id}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{inv.amount_zat}</td>
                    <td className="border-b th-border px-3 py-2">
                      <div className="text-xs th-dim">
                        pending={inv.received_zat_pending} confirmed={inv.received_zat_confirmed}
                      </div>
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{inv.status}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{inv.updated_at}</td>
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

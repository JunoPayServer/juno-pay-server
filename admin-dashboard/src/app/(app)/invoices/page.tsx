"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type Invoice, listInvoices } from "@/lib/api";

export default function InvoicesPage() {
  const router = useRouter();
  const [merchantID, setMerchantID] = useState("");
  const [status, setStatus] = useState("");
  const [externalOrderID, setExternalOrderID] = useState("");

  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function syncURL(next: { merchantID: string; status: string; externalOrderID: string }) {
    const p = new URLSearchParams();
    if (next.merchantID.trim()) p.set("merchant_id", next.merchantID.trim());
    if (next.status.trim()) p.set("status", next.status.trim());
    if (next.externalOrderID.trim()) p.set("external_order_id", next.externalOrderID.trim());
    const q = p.toString();
    router.replace(`/invoices${q ? `?${q}` : ""}`);
  }

  async function refresh(override?: { merchantID?: string; status?: string; externalOrderID?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      status: override?.status ?? status,
      externalOrderID: override?.externalOrderID ?? externalOrderID,
    };
    try {
      setRefreshing(true);
      setError(null);
      syncURL(next);
      const out = await listInvoices({
        merchant_id: next.merchantID.trim() || undefined,
        status: next.status.trim() || undefined,
        external_order_id: next.externalOrderID.trim() || undefined,
        limit: "100",
      });
      setInvoices(out.invoices);
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
    const st = sp.get("status") ?? "";
    const ext = sp.get("external_order_id") ?? "";
    setMerchantID(m);
    setStatus(st);
    setExternalOrderID(ext);
    void refresh({ merchantID: m, status: st, externalOrderID: ext });
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold tracking-tight">Invoices</h1>
        <p className="mt-1 text-sm text-zinc-600">List invoices with filters.</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
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
            <label className="block text-sm font-medium text-zinc-700">External Order ID</label>
            <input
              value={externalOrderID}
              onChange={(e) => setExternalOrderID(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm"
              placeholder="order-123"
            />
          </div>
        </div>

        <div className="mt-4 flex items-center gap-2">
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
        <h2 className="text-sm font-semibold text-zinc-950">Invoices</h2>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : invoices.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No invoices.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Invoice</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Merchant</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Amount</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Received</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {invoices.map((inv) => (
                  <tr key={inv.invoice_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-medium">
                        <Link href={`/invoice?invoice_id=${encodeURIComponent(inv.invoice_id)}`} className="hover:underline">
                          {inv.external_order_id}
                        </Link>
                      </div>
                      <div className="font-mono text-xs text-zinc-500">{inv.invoice_id}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-mono text-xs">{inv.merchant_id}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{inv.amount_zat}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="text-xs text-zinc-700">
                        pending={inv.received_zat_pending} confirmed={inv.received_zat_confirmed}
                      </div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{inv.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{inv.updated_at}</td>
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

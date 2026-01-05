"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type Deposit, getInvoice, listDeposits } from "@/lib/api";

export default function DepositsPage() {
  const router = useRouter();
  const [merchantID, setMerchantID] = useState("");
  const [invoiceID, setInvoiceID] = useState("");
  const [txid, setTxID] = useState("");

  const [deposits, setDeposits] = useState<Deposit[]>([]);
  const [invoiceLabels, setInvoiceLabels] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function syncURL(next: { merchantID: string; invoiceID: string; txid: string }) {
    const p = new URLSearchParams();
    if (next.merchantID.trim()) p.set("merchant_id", next.merchantID.trim());
    if (next.invoiceID.trim()) p.set("invoice_id", next.invoiceID.trim());
    if (next.txid.trim()) p.set("txid", next.txid.trim());
    const q = p.toString();
    router.replace(`/deposits${q ? `?${q}` : ""}`);
  }

  async function refresh(override?: { merchantID?: string; invoiceID?: string; txid?: string }) {
    const next = {
      merchantID: override?.merchantID ?? merchantID,
      invoiceID: override?.invoiceID ?? invoiceID,
      txid: override?.txid ?? txid,
    };
    try {
      setRefreshing(true);
      setError(null);
      syncURL(next);
      const out = await listDeposits({
        merchant_id: next.merchantID.trim() || undefined,
        invoice_id: next.invoiceID.trim() || undefined,
        txid: next.txid.trim() || undefined,
        limit: "100",
      });
      setDeposits(out.deposits);
      void hydrateInvoiceLabels(out.deposits);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  async function hydrateInvoiceLabels(deps: Deposit[]) {
    const ids = Array.from(new Set(deps.map((d) => d.invoice_id).filter((v): v is string => Boolean(v && v.trim()))));
    const missing = ids.filter((id) => !invoiceLabels[id]);
    if (missing.length === 0) return;

    const entries = await Promise.all(
      missing.map(async (id) => {
        try {
          const inv = await getInvoice(id);
          return [id, inv.external_order_id] as const;
        } catch {
          return null;
        }
      }),
    );

    const next: Record<string, string> = {};
    for (const e of entries) {
      if (!e) continue;
      next[e[0]] = e[1];
    }
    if (Object.keys(next).length === 0) return;
    setInvoiceLabels((prev) => ({ ...prev, ...next }));
  }

  useEffect(() => {
    const sp = new URLSearchParams(window.location.search);
    const m = sp.get("merchant_id") ?? "";
    const inv = sp.get("invoice_id") ?? "";
    const t = sp.get("txid") ?? "";
    setMerchantID(m);
    setInvoiceID(inv);
    setTxID(t);
    void refresh({ merchantID: m, invoiceID: inv, txid: t });
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
                        <div>
                          <Link
                            href={`/invoice?invoice_id=${encodeURIComponent(d.invoice_id)}`}
                            className="font-medium text-zinc-950 hover:underline"
                          >
                            {invoiceLabels[d.invoice_id] ?? "Invoice"}
                          </Link>
                          <div className="font-mono text-xs text-zinc-500">{d.invoice_id}</div>
                        </div>
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

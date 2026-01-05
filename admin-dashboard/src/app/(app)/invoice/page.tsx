"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type InvoiceDetails, getInvoice } from "@/lib/api";

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-zinc-100 py-2">
      <div className="text-sm text-zinc-600">{label}</div>
      <div className="text-sm font-medium text-zinc-950">{value}</div>
    </div>
  );
}

export default function InvoiceDetailPage() {
  const [invoiceID, setInvoiceID] = useState("");
  const [inv, setInv] = useState<InvoiceDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function refresh(id: string) {
    const v = id.trim();
    if (!v) return;
    try {
      setRefreshing(true);
      setError(null);
      const out = await getInvoice(v);
      setInv(out);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) return;
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("invoice_id") ?? "";
    setInvoiceID(id);
    void refresh(id);
  }, []);

  if (!invoiceID) {
    return <div className="text-sm text-zinc-600">invoice_id is required.</div>;
  }
  if (loading && !inv) {
    return <div className="text-sm text-zinc-600">Loading...</div>;
  }
  if (error) {
    return <ErrorBanner message={error} />;
  }
  if (!inv) {
    return <div className="text-sm text-zinc-600">Invoice not found.</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-lg font-semibold tracking-tight">{inv.external_order_id}</h1>
          <div className="mt-1 font-mono text-xs text-zinc-500">{inv.invoice_id}</div>
        </div>
        <button
          type="button"
          onClick={() => refresh(inv.invoice_id)}
          disabled={refreshing}
          className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50 disabled:opacity-60"
        >
          {refreshing ? "Refreshing..." : "Refresh"}
        </button>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Invoice</h2>
        <div className="mt-3">
          <Row label="Merchant ID" value={<span className="font-mono text-xs">{inv.merchant_id}</span>} />
          <Row label="Status" value={inv.status} />
          <Row label="External Order ID" value={<span className="font-mono text-xs break-all">{inv.external_order_id}</span>} />
          <Row label="Wallet ID" value={<span className="font-mono text-xs break-all">{inv.wallet_id}</span>} />
          <Row label="Address Index" value={inv.address_index} />
          <Row label="Created After Height" value={inv.created_after_height} />
          <Row label="Created After Hash" value={<span className="font-mono text-xs break-all">{inv.created_after_hash}</span>} />
          <Row label="Address" value={<span className="font-mono text-xs break-all">{inv.address}</span>} />
          <Row label="Amount (zat)" value={inv.amount_zat} />
          <Row label="Received Pending (zat)" value={inv.received_zat_pending} />
          <Row label="Received Confirmed (zat)" value={inv.received_zat_confirmed} />
          <Row label="Required Confirmations" value={inv.required_confirmations} />
          <Row
            label="Policies"
            value={
              <span className="font-mono text-xs">
                late={inv.policies.late_payment_policy} partial={inv.policies.partial_payment_policy} overpay={inv.policies.overpayment_policy}
              </span>
            }
          />
          <Row label="Expires At" value={inv.expires_at ?? "—"} />
          <Row label="Created At" value={inv.created_at} />
          <Row label="Updated At" value={inv.updated_at} />
        </div>
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Actions</h2>
        <div className="mt-3 text-sm text-zinc-700">
          <Link href={`/deposits?invoice_id=${encodeURIComponent(inv.invoice_id)}`} className="font-medium text-zinc-950 hover:underline">
            View deposits for this invoice
          </Link>
        </div>
      </section>
    </div>
  );
}

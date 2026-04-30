"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ErrorBanner } from "@/components/ErrorBanner";
import { Row } from "@/components/Row";
import { APIError, type InvoiceDetails, getInvoice } from "@/lib/api";

export default function InvoiceDetailPage() {
  const router = useRouter();
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
      if (e instanceof APIError && e.status === 401) { router.replace("/login"); return; }
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
    return <div className="text-sm th-dim">invoice_id is required.</div>;
  }
  if (loading && !inv) {
    return <div className="text-sm th-dim">Loading...</div>;
  }
  if (error) {
    return <ErrorBanner message={error} />;
  }
  if (!inv) {
    return <div className="text-sm th-dim">Invoice not found.</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-base font-semibold th-text">{inv.external_order_id}</h1>
          <div className="mt-1 font-mono text-xs th-faint">{inv.invoice_id}</div>
        </div>
        <button
          type="button"
          onClick={() => refresh(inv.invoice_id)}
          disabled={refreshing}
          className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-sm transition-colors disabled:opacity-60"
        >
          {refreshing ? "Refreshing..." : "Refresh"}
        </button>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Invoice</h2>
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

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Actions</h2>
        <div className="mt-3 text-sm th-dim">
          <Link href={`/deposits?invoice_id=${encodeURIComponent(inv.invoice_id)}`} className="th-text hover:text-[#dc8548] transition-colors font-medium">
            View deposits for this invoice
          </Link>
        </div>
      </section>
    </div>
  );
}

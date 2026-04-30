"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { type Merchant, createMerchant, listMerchants } from "@/lib/api";
import { inputCls } from "@/lib/styles";
import { useListPage } from "@/lib/useListPage";

export default function MerchantsPage() {
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [name, setName] = useState("");
  const [creating, setCreating] = useState(false);
  const { loading, refreshing, error, setError, run } = useListPage("/merchants");

  async function refresh() {
    await run(async () => {
      const ms = await listMerchants();
      setMerchants(ms);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void refresh(); }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-base font-semibold th-text">Merchants</h1>
        <p className="mt-1 text-xs th-dim">Create and manage merchant profiles.</p>
      </div>

      <section className="rounded-2xl border th-border th-surface p-5">
        <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Create Merchant</h2>
        <form
          className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end"
          onSubmit={async (e) => {
            e.preventDefault();
            const v = name.trim();
            if (!v) return;
            setCreating(true);
            setError(null);
            try {
              await createMerchant(v);
              setName("");
              await refresh();
            } catch (e) {
              setError(e instanceof Error ? e.message : "create failed");
            } finally {
              setCreating(false);
            }
          }}
        >
          <div className="flex-1">
            <label htmlFor="name" className="block text-xs th-muted mb-1">Name</label>
            <input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className={inputCls}
              placeholder="acme"
              required
            />
          </div>
          <button
            type="submit"
            disabled={creating}
            className="btn-gold rounded-lg px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
          >
            {creating ? "Creating..." : "Create"}
          </button>
        </form>
        {error ? <div className="mt-3"><ErrorBanner message={error} /></div> : null}
      </section>

      <section className="rounded-2xl border th-border th-surface p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-xs font-semibold uppercase tracking-wider th-muted">Merchants</h2>
          <button
            type="button"
            onClick={() => refresh()}
            disabled={refreshing}
            className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-xs transition-colors disabled:opacity-60"
          >
            {refreshing ? "Refreshing..." : "Refresh"}
          </button>
        </div>

        {loading ? (
          <div className="mt-4 text-sm th-dim">Loading...</div>
        ) : merchants.length === 0 ? (
          <div className="mt-4 text-sm th-dim">No merchants.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint">
                  <th className="border-b th-border px-3 py-2">Merchant</th>
                  <th className="border-b th-border px-3 py-2">Status</th>
                  <th className="border-b th-border px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {merchants.map((m) => (
                  <tr key={m.merchant_id} className="text-sm th-text">
                    <td className="border-b th-border px-3 py-2">
                      <div className="font-medium">
                        <Link href={`/merchant?merchant_id=${encodeURIComponent(m.merchant_id)}`} className="th-text hover:text-[#dc8548] transition-colors">
                          {m.name}
                        </Link>
                      </div>
                      <div className="font-mono text-xs th-faint">{m.merchant_id}</div>
                    </td>
                    <td className="border-b th-border px-3 py-2 th-dim">{m.status}</td>
                    <td className="border-b th-border px-3 py-2 th-dim">{m.updated_at}</td>
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

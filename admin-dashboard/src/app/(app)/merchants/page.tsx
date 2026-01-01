"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorBanner } from "@/components/ErrorBanner";
import { APIError, type Merchant, createMerchant, listMerchants } from "@/lib/api";

export default function MerchantsPage() {
  const [merchants, setMerchants] = useState<Merchant[]>([]);
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    try {
      setError(null);
      const ms = await listMerchants();
      setMerchants(ms);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) {
        return;
      }
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
        <h1 className="text-lg font-semibold tracking-tight">Merchants</h1>
        <p className="mt-1 text-sm text-zinc-600">Create and manage merchant profiles.</p>
      </div>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <h2 className="text-sm font-semibold text-zinc-950">Create Merchant</h2>
        <form
          className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-end"
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
            <label htmlFor="name" className="block text-sm font-medium text-zinc-700">
              Name
            </label>
            <input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1 w-full rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-950 shadow-sm focus:border-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-200"
              placeholder="acme"
              required
            />
          </div>
          <button
            type="submit"
            disabled={creating}
            className="rounded-md bg-zinc-950 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-60"
          >
            {creating ? "Creating..." : "Create"}
          </button>
        </form>
        {error ? <div className="mt-3"><ErrorBanner message={error} /></div> : null}
      </section>

      <section className="rounded-lg border border-zinc-200 bg-white p-4">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-zinc-950">Merchants</h2>
          <button
            type="button"
            onClick={() => refresh()}
            className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50"
          >
            Refresh
          </button>
        </div>

        {loading ? (
          <div className="mt-4 text-sm text-zinc-600">Loading...</div>
        ) : merchants.length === 0 ? (
          <div className="mt-4 text-sm text-zinc-600">No merchants.</div>
        ) : (
          <div className="mt-4 overflow-x-auto">
            <table className="min-w-full border-separate border-spacing-0">
              <thead>
                <tr className="text-left text-xs font-semibold uppercase tracking-wider text-zinc-500">
                  <th className="border-b border-zinc-200 px-3 py-2">Merchant</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Status</th>
                  <th className="border-b border-zinc-200 px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {merchants.map((m) => (
                  <tr key={m.merchant_id} className="text-sm text-zinc-950">
                    <td className="border-b border-zinc-100 px-3 py-2">
                      <div className="font-medium">
                        <Link href={`/merchant?merchant_id=${encodeURIComponent(m.merchant_id)}`} className="hover:underline">
                          {m.name}
                        </Link>
                      </div>
                      <div className="font-mono text-xs text-zinc-500">{m.merchant_id}</div>
                    </td>
                    <td className="border-b border-zinc-100 px-3 py-2">{m.status}</td>
                    <td className="border-b border-zinc-100 px-3 py-2">{m.updated_at}</td>
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

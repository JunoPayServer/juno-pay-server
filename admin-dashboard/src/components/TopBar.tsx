"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { adminLogout } from "@/lib/api";
import { useAdmin } from "@/components/AdminProvider";

export function TopBar() {
  const router = useRouter();
  const { refreshStatus } = useAdmin();
  const [query, setQuery] = useState("");

  function go(raw: string) {
    const q = raw.trim();
    if (!q) return;

    if (q.startsWith("inv_")) {
      router.push(`/invoice?invoice_id=${encodeURIComponent(q)}`);
      return;
    }
    if (q.startsWith("m_")) {
      router.push(`/merchant?merchant_id=${encodeURIComponent(q)}`);
      return;
    }
    if (/^[0-9a-f]{64}$/i.test(q)) {
      router.push(`/deposits?txid=${encodeURIComponent(q)}`);
      return;
    }

    router.push(`/invoices?external_order_id=${encodeURIComponent(q)}`);
  }

  return (
    <header className="flex h-14 items-center justify-between gap-4 border-b border-zinc-200 bg-white px-4">
      <div className="flex min-w-0 items-center gap-3">
        <div className="text-sm text-zinc-600">Admin</div>
        <form
          className="hidden min-w-0 items-center gap-2 sm:flex"
          onSubmit={(e) => {
            e.preventDefault();
            go(query);
          }}
        >
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="w-[420px] max-w-full rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 shadow-sm"
            placeholder="Search inv_, m_, txid, or external_order_id"
          />
          <button type="submit" className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50">
            Go
          </button>
        </form>
      </div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => refreshStatus()}
          className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50"
        >
          Refresh
        </button>
        <button
          type="button"
          onClick={async () => {
            await adminLogout();
            router.replace("/login");
          }}
          className="rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-950 hover:bg-zinc-50"
        >
          Logout
        </button>
      </div>
    </header>
  );
}

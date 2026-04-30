"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { useAdmin } from "@/components/AdminProvider";
import { useAdminLogout } from "@/lib/useAdminLogout";

export function TopBar() {
  const router = useRouter();
  const { refreshStatus } = useAdmin();
  const [query, setQuery] = useState("");
  const { logout } = useAdminLogout();

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
    <header className="flex h-14 items-center justify-between gap-4 border-b th-border th-page-alpha backdrop-blur-md px-6 sticky top-0 z-30">
      <div className="flex min-w-0 items-center gap-3">
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
            className="w-[380px] max-w-full rounded-lg border th-border th-input th-text px-3 py-1.5 text-sm focus:outline-none focus:border-[#dc8548]/40"
            placeholder="Search inv_, m_, txid, or external_order_id"
          />
          <button
            type="submit"
            className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-sm transition-colors"
          >
            Go
          </button>
        </form>
      </div>
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={() => refreshStatus()}
          className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-sm transition-colors"
        >
          Refresh
        </button>
        <button
          type="button"
          onClick={logout}
          className="rounded-lg border th-border th-hover th-muted px-3 py-1.5 text-sm transition-colors"
        >
          Logout
        </button>
      </div>
    </header>
  );
}

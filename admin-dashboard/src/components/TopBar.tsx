"use client";

import { useRouter } from "next/navigation";
import { adminLogout } from "@/lib/api";
import { useAdmin } from "@/components/AdminProvider";

export function TopBar() {
  const router = useRouter();
  const { refreshStatus } = useAdmin();

  return (
    <header className="flex h-14 items-center justify-between border-b border-zinc-200 bg-white px-4">
      <div className="text-sm text-zinc-600">Admin</div>
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


"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { APIError, type AdminStatusSnapshot, getAdminStatus } from "@/lib/api";

type AdminContextValue = {
  status: AdminStatusSnapshot | null;
  refreshStatus: () => Promise<void>;
  loading: boolean;
  error: string | null;
};

const AdminContext = createContext<AdminContextValue | undefined>(undefined);

export function AdminProvider({ children }: Readonly<{ children: React.ReactNode }>) {
  const router = useRouter();
  const pathname = usePathname();

  const [status, setStatus] = useState<AdminStatusSnapshot | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refreshStatus = useCallback(async () => {
    try {
      setError(null);
      const s = await getAdminStatus();
      setStatus(s);
    } catch (e) {
      if (e instanceof APIError && e.status === 401) {
        const clean = pathname.replace(/\/+$/, "");
        if (!clean.endsWith("/login")) {
          router.replace("/login");
        }
        return;
      }
      setError(e instanceof Error ? e.message : "unknown error");
    }
  }, [router, pathname]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      if (!mounted) return;
      setLoading(true);
      await refreshStatus();
      if (!mounted) return;
      setLoading(false);
    })();
    return () => {
      mounted = false;
    };
  }, [refreshStatus]);

  const value = useMemo<AdminContextValue>(
    () => ({
      status,
      refreshStatus,
      loading,
      error,
    }),
    [status, refreshStatus, loading, error],
  );

  return <AdminContext.Provider value={value}>{children}</AdminContext.Provider>;
}

export function useAdmin() {
  const v = useContext(AdminContext);
  if (!v) {
    throw new Error("useAdmin must be used within <AdminProvider>");
  }
  return v;
}

"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { APIError } from "@/lib/api";

export function urlParam(key: string, fallback = ""): string {
  if (typeof window === "undefined") return fallback;
  return new URLSearchParams(window.location.search).get(key) ?? fallback;
}

export function useListPage(pathname: string) {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function syncURL(params: Record<string, string>) {
    const p = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
      if (value.trim()) p.set(key, value.trim());
    }
    const q = p.toString();
    router.replace(`${pathname}${q ? `?${q}` : ""}`);
  }

  async function run(fn: () => Promise<void>) {
    try {
      setRefreshing(true);
      setError(null);
      await fn();
    } catch (e) {
      if (e instanceof APIError && e.status === 401) { router.replace("/login"); return; }
      setError(e instanceof Error ? e.message : "load failed");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }

  return { loading, refreshing, error, setError, syncURL, run };
}

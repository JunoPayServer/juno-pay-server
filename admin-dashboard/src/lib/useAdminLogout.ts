"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { adminLogout } from "@/lib/api";

export function useAdminLogout() {
  const router = useRouter();
  const [loggingOut, setLoggingOut] = useState(false);

  async function logout() {
    setLoggingOut(true);
    try {
      await adminLogout();
    } catch (e) {
      console.error("logout failed", e);
    }
    router.replace("/login");
  }

  return { logout, loggingOut };
}

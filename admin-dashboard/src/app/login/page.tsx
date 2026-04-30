"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { adminLogin } from "@/lib/api";
import { ErrorBanner } from "@/components/ErrorBanner";
import { inputCls } from "@/lib/styles";
import { JunoPayLogo } from "@junopayserver/widgets";

export default function LoginPage() {
  const router = useRouter();

  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  return (
    <div className="flex min-h-screen items-center justify-center th-page px-6">
      <div className="w-full max-w-sm rounded-2xl border th-border th-surface p-6 shadow-2xl">
        <div className="flex items-center gap-2 mb-6">
          <JunoPayLogo className="w-8 h-8 rounded-lg shrink-0" />
          <span className="font-semibold th-text">
            Juno<span className="text-[#dc8548]">Pay</span>
            <span className="text-xs th-faint ml-1.5 font-normal">Admin</span>
          </span>
        </div>

        <h1 className="text-base font-semibold th-text">Sign in</h1>
        <p className="mt-1 text-xs th-dim">Enter the admin password to continue.</p>

        <form
          className="mt-5 space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            setLoading(true);
            setError(null);
            try {
              await adminLogin(password);
              router.replace("/status");
            } catch (e) {
              setError(e instanceof Error ? e.message : "login failed");
            } finally {
              setLoading(false);
            }
          }}
        >
          {error ? <ErrorBanner message={error} /> : null}

          <div>
            <label htmlFor="password" className="block text-xs th-muted mb-1">
              Admin Password
            </label>
            <input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className={inputCls}
              autoComplete="current-password"
              required
            />
          </div>

          <button
            type="submit"
            disabled={loading}
            className="btn-gold w-full rounded-lg px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
          >
            {loading ? "Signing in..." : "Sign In"}
          </button>
        </form>
      </div>
    </div>
  );
}

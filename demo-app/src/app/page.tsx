"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { createAirInvoice } from "@/app/actions";
import { InvoiceCheckoutCard } from "@/app/_components/InvoiceCheckoutCard";
import { Sidebar } from "@/app/_components/Sidebar";
import { clearUser, loadOrders, loadUser, saveOrders, saveUser, type DemoOrder, type DemoUser, REMEMBER_CREDS_KEY } from "@/lib/storage";
import { uuidv4 } from "@/lib/uuid";
import { getTheme, setTheme, type Theme } from "@junopayserver/widgets";
import { formatJUNO } from "@/lib/format";

export default function HomePage() {
  const [user, setUser] = useState<DemoUser | null>(null);
  const [orders, setOrders] = useState<DemoOrder[]>([]);
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [rememberMe, setRememberMe] = useState(false);
  const [buying, setBuying] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [theme, setThemeState] = useState<Theme>("dark");

  useEffect(() => {
    setUser(loadUser());
    setOrders(loadOrders());
    setThemeState(getTheme());
    const saved = localStorage.getItem(REMEMBER_CREDS_KEY);
    if (saved) {
      setRememberMe(true);
      const { username: u, email: e } = JSON.parse(saved) as { username: string; email: string };
      setUsername(u ?? "");
      setEmail(e ?? "");
    }
  }, []);


  function handleThemeToggle() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    setThemeState(next);
  }

  const lastOrder = useMemo(() => orders[0] ?? null, [orders]);

  function handleReset() {
    clearUser();
    setUser(null);
    setOrders([]);
    // Re-sync theme from localStorage (Sidebar may have changed it)
    setThemeState(getTheme());
    // Re-sync rememberMe from localStorage
    const saved = localStorage.getItem(REMEMBER_CREDS_KEY);
    if (saved) {
      setRememberMe(true);
      try {
        const { username: u, email: e } = JSON.parse(saved) as { username: string; email: string };
        setUsername(u ?? "");
        setEmail(e ?? "");
      } catch { /* ignore */ }
    }
  }

  /* ── Unregistered: centered sign-in card ── */
  if (!user) {
    return (
      <div className="min-h-screen th-page flex flex-col items-center justify-center px-4 relative overflow-hidden">
        {/* Theme toggle */}
        <button
          type="button"
          onClick={handleThemeToggle}
          className="absolute top-4 right-4 z-20 p-2 rounded-lg th-surface border th-border th-muted th-hover transition-colors"
          title={theme === "dark" ? "Switch to light" : "Switch to dark"}
        >
          {theme === "dark" ? (
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M10 2a1 1 0 011 1v1a1 1 0 11-2 0V3a1 1 0 011-1zm4 8a4 4 0 11-8 0 4 4 0 018 0zm-.464 4.95l.707.707a1 1 0 001.414-1.414l-.707-.707a1 1 0 00-1.414 1.414zm2.12-10.607a1 1 0 010 1.414l-.706.707a1 1 0 11-1.414-1.414l.707-.707a1 1 0 011.414 0zM17 11a1 1 0 100-2h-1a1 1 0 100 2h1zm-7 4a1 1 0 011 1v1a1 1 0 11-2 0v-1a1 1 0 011-1zM5.05 6.464A1 1 0 106.465 5.05l-.708-.707a1 1 0 00-1.414 1.414l.707.707zm1.414 8.486l-.707.707a1 1 0 01-1.414-1.414l.707-.707a1 1 0 011.414 1.414zM4 11a1 1 0 100-2H3a1 1 0 000 2h1z" clipRule="evenodd" />
            </svg>
          ) : (
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path d="M17.293 13.293A8 8 0 016.707 2.707a8.001 8.001 0 1010.586 10.586z" />
            </svg>
          )}
        </button>

        {/* Background blobs */}
        <div className="absolute inset-0 pointer-events-none">
          <div className="absolute inset-0 bg-[radial-gradient(ellipse_70%_50%_at_50%_-10%,rgba(220,133,72,0.10),transparent)]" />
          <div className="animate-float-slow absolute -bottom-20 -left-16 w-[300px] h-[300px] rounded-full bg-[#dc8548]/8 blur-sm" />
          <div className="animate-float absolute top-10 right-[8%] w-[150px] h-[150px] rounded-full bg-[#dc8548]/12 blur-md" style={{ animationDelay: "1s" }} />
        </div>

        {/* Logo */}
        <div className="relative z-10 flex flex-col items-center mb-8">
          <div className="w-16 h-16 rounded-2xl overflow-hidden mb-4">
            <img src="/juno-pay-server-logo.svg" alt="JunoPay" className="w-full h-full object-contain" />
          </div>
          <h1 className="text-2xl font-bold th-text">Welcome to JunoPay</h1>
          <p className="mt-1 text-sm th-dim">Self-hosted Juno Cash payment processor</p>
        </div>

        {/* Card */}
        <div className="relative z-10 w-full max-w-sm rounded-2xl border th-border th-modal p-7 shadow-xl">
          <h2 className="text-base font-semibold th-text mb-5">Sign in to Demo</h2>

          {error && (
            <div className="mb-4 rounded-lg border border-red-500/30 bg-red-500/8 px-3 py-2.5 text-sm text-red-400">
              {error}
            </div>
          )}

          <form
            onSubmit={(e) => {
              e.preventDefault();
              const u = username.trim();
              const v = email.trim();
              if (!u || !v) return;
              if (rememberMe) {
                localStorage.setItem(REMEMBER_CREDS_KEY, JSON.stringify({ username: u, email: v }));
              } else {
                localStorage.removeItem(REMEMBER_CREDS_KEY);
              }
              const newUser: DemoUser = { user_id: uuidv4(), email: v, username: u };
              saveUser(newUser);
              setUser(newUser);
              setOrders(loadOrders());
            }}
            className="space-y-4"
          >
            <div>
              <label className="block text-xs font-medium th-muted mb-1.5" htmlFor="username">
                Username
              </label>
              <input
                id="username"
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full rounded-lg border th-border th-input px-3 py-2.5 text-sm th-text placeholder-[#888] focus:border-[#dc8548]/50 focus:outline-none transition-colors"
                placeholder="your name"
                required
                autoFocus
              />
            </div>
            <div>
              <label className="block text-xs font-medium th-muted mb-1.5" htmlFor="email">
                Email address
              </label>
              <input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="w-full rounded-lg border th-border th-input px-3 py-2.5 text-sm th-text placeholder-[#888] focus:border-[#dc8548]/50 focus:outline-none transition-colors"
                placeholder="you@example.com"
                required
              />
            </div>
            <label className="flex items-center gap-2.5 cursor-pointer select-none">
              <div className="relative shrink-0 w-4 h-4">
                <input
                  type="checkbox"
                  checked={rememberMe}
                  onChange={(e) => {
                    const checked = e.target.checked;
                    setRememberMe(checked);
                    if (!checked) localStorage.removeItem(REMEMBER_CREDS_KEY);
                  }}
                  className="absolute inset-0 opacity-0 w-full h-full cursor-pointer"
                />
                <div className={`w-4 h-4 rounded border transition-colors pointer-events-none ${
                  rememberMe ? "bg-[#dc8548] border-[#dc8548]" : "th-border th-input"
                }`} />
                <svg
                  className={`absolute inset-0 m-auto w-2.5 h-2.5 transition-opacity pointer-events-none ${
                    rememberMe ? "opacity-100" : "opacity-0"
                  } ${theme === "dark" ? "text-white" : "text-black"}`}
                  viewBox="0 0 10 10" fill="none"
                >
                  <path d="M1.5 5l2.5 2.5 4.5-4.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </div>
              <span className="text-xs th-muted">Remember me</span>
            </label>
            <button type="submit" className="w-full btn-gold text-black font-semibold py-2.5 rounded-lg text-sm">
              Sign in
            </button>
          </form>

          <p className="mt-4 text-center text-xs th-dim">
            Demo only — stored in your browser, no server account needed.
          </p>
        </div>
      </div>
    );
  }

  /* ── Registered: BTCPay-style sidebar + main ── */
  return (
    <div className="min-h-screen th-page flex">
      <Sidebar username={user.username ?? user.email} email={user.email} onReset={handleReset} />

      {/* Main content */}
      <div className="flex-1 ml-64 flex flex-col min-h-screen th-content">
        {/* Top bar */}
        <header className="flex items-center justify-between px-8 py-4 border-b th-border th-page-alpha backdrop-blur-md sticky top-0 z-30">
          <div>
            <h1 className="text-base font-semibold th-text">Dashboard</h1>
            <p className="text-xs th-dim mt-0.5">Buy a gallon of air for 1 JUNO</p>
          </div>
          <div className="text-xs th-faint">
            {orders.length > 0 && `${orders.length} order${orders.length > 1 ? "s" : ""}`}
          </div>
        </header>

        {/* Content */}
        <main className="flex-1 px-8 py-8 max-w-2xl">
          {error && (
            <div className="mb-6 rounded-lg border border-red-500/30 bg-red-500/8 px-4 py-3 text-sm text-red-400">
              {error}
            </div>
          )}

          {/* Buy card */}
          <div className="rounded-2xl border th-border th-surface p-6 mb-6">
            <h2 className="text-sm font-semibold th-muted uppercase tracking-wider mb-4">Store</h2>

            <div className="flex items-center justify-between rounded-xl border th-border th-input px-5 py-4">
              <div>
                <div className="text-sm font-medium th-text">1 gallon of air</div>
                <div className="text-xs th-dim mt-0.5">Price: 1 JUNO</div>
              </div>
              <button
                type="button"
                disabled={buying}
                onClick={async () => {
                  setError(null);
                  setBuying(true);
                  try {
                    const orderID = uuidv4();
                    const externalOrderID = `demo-air:${user.user_id}:${orderID}`;
                    const out = await createAirInvoice({ external_order_id: externalOrderID, demo_user_id: user.user_id, email: user.email });
                    if (!out.ok) { setError(out.error); return; }
                    const o: DemoOrder = {
                      order_id: orderID,
                      external_order_id: externalOrderID,
                      invoice_id: out.data.invoice.invoice_id,
                      invoice_token: out.data.invoice_token,
                      address: out.data.invoice.address,
                      amount_zat: out.data.invoice.amount_zat,
                      status: out.data.invoice.status,
                      received_zat_pending: out.data.invoice.received_zat_pending,
                      received_zat_confirmed: out.data.invoice.received_zat_confirmed,
                      created_at: out.data.invoice.created_at,
                      updated_at: out.data.invoice.updated_at,
                      events_cursor: "0",
                    };
                    const next = [o, ...orders];
                    saveOrders(next);
                    setOrders(next);
                  } catch (e) {
                    setError(e instanceof Error ? e.message : "create invoice failed");
                  } finally {
                    setBuying(false);
                  }
                }}
                className="btn-gold text-black font-semibold px-5 py-2 rounded-lg text-sm"
              >
                {buying ? "Creating…" : "Buy"}
              </button>
            </div>
          </div>

          {/* Latest invoice */}
          {lastOrder ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between mb-2">
                <h2 className="text-sm font-semibold th-muted uppercase tracking-wider">Latest Invoice</h2>
                <Link href="/orders" className="text-xs text-[#dc8548] hover:text-[#e89a68] transition-colors">
                  View all ({orders.length}) →
                </Link>
              </div>
              <InvoiceCheckoutCard
                order={lastOrder}
                onOrderUpdate={(next) => {
                  setOrders((prev) => {
                    const updated = prev.map((o) => (o.order_id === next.order_id ? next : o));
                    saveOrders(updated);
                    return updated;
                  });
                }}
              />
            </div>
          ) : (
            <div className="rounded-2xl border th-border th-surface p-8 text-center">
              <div className="th-dim text-sm">No invoices yet. Buy something to get started.</div>
            </div>
          )}
        </main>

        {/* Footer */}
        <footer className="px-8 py-4 border-t th-border text-xs th-faint">
          Amounts in JUNO · stored as zatoshis
          {orders.length > 0 && (
            <span> · latest: <span className="font-mono">{formatJUNO(orders[0]!.amount_zat)} JUNO</span></span>
          )}
        </footer>
      </div>
    </div>
  );
}

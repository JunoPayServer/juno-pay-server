"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { JunoPayLogo, ThemeToggle, useClickOutside } from "@junopayserver/widgets";
import { useAdminLogout } from "@/lib/useAdminLogout";

function cleanPath(p: string) {
  const v = p.replace(/\/+$/, "");
  return v === "" ? "/" : v;
}

const navItems = [
  {
    href: "/status",
    label: "Status",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
      </svg>
    ),
  },
  {
    href: "/merchants",
    label: "Merchants",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" />
      </svg>
    ),
  },
  {
    href: "/invoices",
    label: "Invoices",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
      </svg>
    ),
  },
  {
    href: "/deposits",
    label: "Deposits",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
      </svg>
    ),
  },
  {
    href: "/review-cases",
    label: "Review Cases",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
      </svg>
    ),
  },
  {
    href: "/refunds",
    label: "Refunds",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" />
      </svg>
    ),
  },
  {
    href: "/event-sinks",
    label: "Event Sinks",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
      </svg>
    ),
  },
  {
    href: "/events",
    label: "Outbound Events",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 19l9 2-9-18-9 18 9-2zm0 0v-8" />
      </svg>
    ),
  },
  {
    href: "/event-deliveries",
    label: "Event Deliveries",
    icon: (
      <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
    ),
  },
];

export function Sidebar() {
  const pathname = usePathname();
  const [accountOpen, setAccountOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const { logout, loggingOut } = useAdminLogout();

  useClickOutside(dropdownRef, () => setAccountOpen(false), accountOpen);

  return (
    <aside className="th-aside fixed top-0 left-0 h-full w-64 flex flex-col border-r th-border z-40">
      {/* Logo */}
      <div className="px-5 py-5 border-b th-border">
        <Link href="/status" className="flex items-center gap-2">
          <JunoPayLogo className="w-8 h-8 rounded-lg shrink-0" />
          <span className="font-semibold th-text">
            Juno<span className="text-[#dc8548]">Pay</span>
            <span className="text-xs th-faint ml-1.5 font-normal">Admin</span>
          </span>
        </Link>
      </div>

      {/* Nav */}
      <nav className="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
        {navItems.map((item) => {
          const withoutBase = pathname.startsWith("/admin") ? pathname.slice("/admin".length) || "/" : pathname;
          const cur = cleanPath(withoutBase);
          const target = cleanPath(item.href);
          const active = cur === target || (target !== "/" && cur.startsWith(target + "/"));

          return (
            <Link
              key={item.href}
              href={item.href}
              className={`flex items-center gap-2.5 px-2 py-2 rounded-lg text-sm transition-colors ${
                active
                  ? "bg-[#dc8548]/15 text-[#dc8548] font-medium"
                  : "th-text th-hover"
              }`}
            >
              {item.icon}
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Bottom: Account dropdown */}
      <div className="px-3 py-4 border-t th-border relative" ref={dropdownRef}>
        {/* Dropdown (pops upward) */}
        {accountOpen && (
          <div className="absolute bottom-full left-3 right-3 mb-2 rounded-xl border th-border th-surface shadow-2xl overflow-hidden">
            {/* Theme toggle */}
            <div className="px-4 py-3 border-b th-border flex items-center justify-between">
              <span className="text-xs th-muted">Theme</span>
              <ThemeToggle />
            </div>

            {/* Logout */}
            <button
              type="button"
              onClick={logout}
              disabled={loggingOut}
              className="w-full flex items-center gap-2.5 px-4 py-3 text-left text-xs text-red-400 hover:bg-red-500/5 transition-colors disabled:opacity-60"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
              </svg>
              {loggingOut ? "Logging out..." : "Logout"}
            </button>
          </div>
        )}

        {/* Account trigger button */}
        <button
          type="button"
          onClick={() => setAccountOpen((o) => !o)}
          className={`w-full flex items-center gap-2.5 px-2 py-2 rounded-lg text-sm transition-colors ${
            accountOpen ? "bg-[#dc8548]/10 text-[#dc8548]" : "th-muted th-hover"
          }`}
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5.121 17.804A13.937 13.937 0 0112 16c2.5 0 4.847.655 6.879 1.804M15 10a3 3 0 11-6 0 3 3 0 016 0zm6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          Account
        </button>
      </div>
    </aside>
  );
}

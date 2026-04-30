"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { JunoPayLogo, ThemeToggle, useClickOutside } from "@junopayserver/widgets";

type SidebarProps = {
  username: string;
  email: string;
  onReset: () => void;
};

const navItems = [
  {
    href: "/",
    label: "Dashboard",
    icon: (
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
      </svg>
    ),
  },
  {
    href: "/orders",
    label: "Orders",
    icon: (
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
      </svg>
    ),
  },
];

export function Sidebar({ username, email, onReset }: SidebarProps) {
  const pathname = usePathname();
  const [accountOpen, setAccountOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  useClickOutside(dropdownRef, () => setAccountOpen(false), accountOpen);

const displayName = username || email;

  return (
    <aside className="th-aside fixed top-0 left-0 h-full w-64 flex flex-col border-r th-border z-40">
      {/* Logo */}
      <div className="px-5 py-5 border-b th-border">
        <Link href="/" className="flex items-center gap-2">
          <JunoPayLogo className="w-8 h-8 rounded-lg overflow-hidden shrink-0" />
          <span className="font-semibold th-text">
            Juno<span className="text-[#dc8548]">Pay</span>
          </span>
        </Link>
      </div>

      {/* Username display */}
      <div className="px-4 py-3 border-b th-border">
        <div className="flex items-center gap-2 px-2 py-1.5 rounded-lg th-input border th-border">
          <div className="w-6 h-6 rounded-md bg-[#dc8548]/20 flex items-center justify-center shrink-0">
            <span className="text-xs font-semibold text-[#dc8548]">{displayName[0]?.toUpperCase()}</span>
          </div>
          <span className="text-xs th-muted truncate">{displayName}</span>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 px-3 py-4 space-y-0.5 overflow-y-auto">
        <div className="mb-3">
          <div className="px-2 mb-1.5 text-[10px] font-semibold uppercase tracking-wider th-faint">Demo</div>
          {navItems.map((item) => {
            const active = pathname === item.href;
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
        </div>

        <div className="mt-4">
          <div className="px-2 mb-1.5 text-[10px] font-semibold uppercase tracking-wider th-faint">Admin</div>
          <Link
            href="/admin/"
            className="flex items-center gap-2.5 px-2 py-2 rounded-lg text-sm th-text th-hover transition-colors"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
            Admin Panel
          </Link>
        </div>
      </nav>

      {/* Account button + dropdown */}
      <div className="px-3 py-4 border-t th-border relative" ref={dropdownRef}>
        {/* Dropdown (pops upward) */}
        {accountOpen && (
          <div className="absolute bottom-full left-3 right-3 mb-2 rounded-xl border th-border th-surface shadow-2xl overflow-hidden">
            {/* Email */}
            <div className="px-4 py-3 border-b th-border">
              <div className="text-[11px] th-faint mb-0.5">Your Email</div>
              <div className="text-xs font-semibold th-text truncate">{email}</div>
            </div>

            {/* Theme toggle */}
            <div className="px-4 py-3 border-b th-border flex items-center justify-between">
              <span className="text-xs th-muted">Theme</span>
              <ThemeToggle />
            </div>

            {/* Manage Account */}
            <button
              type="button"
              className="w-full flex items-center gap-2.5 px-4 py-3 text-left text-xs th-muted th-hover border-b th-border transition-colors"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
              </svg>
              Manage Account
            </button>

            {/* Reset demo */}
            <button
              type="button"
              onClick={() => { setAccountOpen(false); onReset(); }}
              className="w-full flex items-center gap-2.5 px-4 py-3 text-left text-xs text-red-400 hover:bg-red-500/5 transition-colors"
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.8}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
              </svg>
              Reset demo
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

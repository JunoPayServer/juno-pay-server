"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

function NavLink({ href, label }: { href: string; label: string }) {
  const pathname = usePathname();
  const active = pathname === href || (href !== "/" && pathname.startsWith(href + "/"));

  return (
    <Link
      href={href}
      className={[
        "block rounded-md px-3 py-2 text-sm font-medium",
        active ? "bg-zinc-950 text-white" : "text-zinc-700 hover:bg-zinc-100 hover:text-zinc-950",
      ].join(" ")}
    >
      {label}
    </Link>
  );
}

export function Sidebar() {
  return (
    <aside className="w-64 shrink-0 border-r border-zinc-200 bg-white">
      <div className="flex h-14 items-center px-4">
        <div className="text-sm font-semibold tracking-tight text-zinc-950">Juno Pay Admin</div>
      </div>
      <nav className="px-2 py-4">
        <div className="space-y-1">
          <NavLink href="/status" label="Status" />
          <NavLink href="/merchants" label="Merchants" />
          <NavLink href="/invoices" label="Invoices" />
          <NavLink href="/deposits" label="Deposits" />
          <NavLink href="/review-cases" label="Review Cases" />
          <NavLink href="/refunds" label="Refunds" />
          <NavLink href="/event-sinks" label="Event Sinks" />
          <NavLink href="/events" label="Outbound Events" />
          <NavLink href="/event-deliveries" label="Event Deliveries" />
        </div>
      </nav>
    </aside>
  );
}


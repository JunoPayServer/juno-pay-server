'use client'

import Link from 'next/link'
import React, { useEffect, useRef, useState } from 'react'
import { getPublicInvoice } from '@/app/actions'
import { InvoiceCheckoutCard } from '@/app/_components/InvoiceCheckoutCard'
import { Sidebar } from '@/app/_components/Sidebar'
import { clearUser, loadOrders, loadUser, saveOrders, type DemoOrder } from '@/lib/storage'
import { formatJUNO } from '@/lib/format'

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    open: 'th-border th-input th-muted',
    pending: 'border-amber-500/30 bg-amber-500/8 text-amber-400',
    partial_pending: 'border-amber-500/30 bg-amber-500/8 text-amber-400',
    partial_confirmed: 'border-amber-500/30 bg-amber-500/8 text-amber-400',
    confirmed: 'border-emerald-500/30 bg-emerald-500/8 text-emerald-400',
    paid: 'border-emerald-500/30 bg-emerald-500/8 text-emerald-400',
    paid_late: 'border-emerald-500/30 bg-emerald-500/8 text-emerald-400',
    overpaid: 'border-blue-500/30 bg-blue-500/8 text-blue-400',
    expired: 'border-red-500/30 bg-red-500/8 text-red-400',
    canceled: 'border-red-500/30 bg-red-500/8 text-red-400',
  }
  const cls = colors[status] ?? 'th-border th-input th-muted'
  return (
    <span
      className={`inline-flex items-center justify-center rounded-full border w-20 py-0.5 text-xs font-medium ${cls}`}
    >
      {status}
    </span>
  )
}

export default function OrdersPage() {
  const [user, setUser] = useState<ReturnType<typeof loadUser>>(null)
  const [orders, setOrders] = useState<DemoOrder[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [syncing, setSyncing] = useState(false)
  const [lit, setLit] = useState(false)
  useEffect(() => {
    if (syncing) {
      setLit(true)
    } else {
      const t = setTimeout(() => setLit(false), 500)
      return () => clearTimeout(t)
    }
  }, [syncing])
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setUser(loadUser())
    setOrders(loadOrders())
  }, [])

  const ordersRef = useRef<DemoOrder[]>([])
  useEffect(() => {
    ordersRef.current = orders
  }, [orders])

  useEffect(() => {
    let stopped = false
    let timer: ReturnType<typeof setTimeout> | undefined
    let inflight = false

    async function tick() {
      if (stopped || inflight) return
      inflight = true
      setSyncing(true)
      try {
        const current = ordersRef.current
        if (!current.length) return
        const results = await Promise.all(
          current.map((o) => getPublicInvoice({ invoice_id: o.invoice_id, invoice_token: o.invoice_token }))
        )
        let anyError: string | null = null
        const next: DemoOrder[] = current.map((o, i) => {
          const invRes = results[i]
          if (!invRes.ok) {
            anyError = invRes.error
            return o
          }
          const inv = invRes.data
          return {
            ...o,
            status: inv.status,
            received_zat_pending: inv.received_zat_pending,
            received_zat_confirmed: inv.received_zat_confirmed,
            updated_at: inv.updated_at,
          }
        })
        if (stopped) return
        saveOrders(next)
        setOrders(next)
        setError(anyError)
      } catch (e) {
        if (!stopped) setError(e instanceof Error ? e.message : 'update failed')
      } finally {
        inflight = false
        if (!stopped) setSyncing(false)
      }
    }

    const schedule = () => {
      timer = setTimeout(async () => {
        await tick()
        if (!stopped) schedule()
      }, 10000)
    }
    void tick()
    schedule()
    return () => {
      stopped = true
      if (timer) clearTimeout(timer)
    }
  }, [])

  /* Not registered */
  if (!user) {
    return (
      <div className="min-h-screen th-page flex items-center justify-center px-4">
        <div className="text-center">
          <div className="text-sm th-dim mb-3">You need to sign in first.</div>
          <Link
            href="/"
            className="text-sm text-[#dc8548] hover:text-[#e89a68] transition-colors"
          >
            ← Go to sign in
          </Link>
        </div>
      </div>
    )
  }

  function handleReset() {
    clearUser()
    window.location.href = '/'
  }

  const handleOrderUpdate = (next: DemoOrder) => {
    setOrders((prev) => {
      const updated = prev.map((o) => (o.order_id === next.order_id ? next : o))
      saveOrders(updated)
      return updated
    })
  }

  return (
    <div className="min-h-screen th-page flex">
      <Sidebar
        username={user.username ?? user.email}
        email={user.email}
        onReset={handleReset}
      />

      <div className="flex-1 ml-64 flex flex-col min-h-screen th-content">
        {/* Top bar */}
        <header className="flex items-center justify-between px-8 py-4 border-b th-border th-page-alpha backdrop-blur-md sticky top-0 z-30">
          <div>
            <h1 className="text-base font-semibold th-text">Orders</h1>
            <p className="text-xs th-dim mt-0.5">Track invoice status</p>
          </div>
          <div className="flex items-center gap-2 text-xs th-faint">
            <span
              className={`w-1.5 h-1.5 rounded-full bg-[#dc8548] transition-opacity duration-500 ${lit ? 'opacity-100 animate-pulse' : 'opacity-30'}`}
            />
            Auto-updating
          </div>
        </header>

        <main className="flex-1 px-8 py-8">
          {error && (
            <div className="mb-6 rounded-lg border border-red-500/30 bg-red-500/8 px-4 py-3 text-sm text-red-400">
              {error}
            </div>
          )}

          {/* Orders table */}
          <div className="rounded-2xl border th-border th-surface overflow-hidden">
            <div className="px-6 py-4 border-b th-border flex items-center justify-between">
              <h2 className="text-sm font-semibold th-muted uppercase tracking-wider">All Orders</h2>
              <span className="text-xs th-faint">{orders.length} total</span>
            </div>

            {orders.length === 0 ? (
              <div className="px-6 py-12 text-center text-sm th-faint">
                No orders yet.{' '}
                <Link
                  href="/"
                  className="text-[#dc8548] hover:text-[#e89a68] transition-colors"
                >
                  Buy something →
                </Link>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="min-w-full">
                  <thead>
                    <tr className="text-left text-[10px] font-semibold uppercase tracking-wider th-faint border-b border-white/5">
                      <th className="px-6 py-3">Order</th>
                      <th className="px-6 py-3">Invoice</th>
                      <th className="px-6 py-3">Amount</th>
                      <th className="px-6 py-3">Received</th>
                      <th className="px-6 py-3">Status</th>
                      <th className="px-6 py-3" />
                    </tr>
                  </thead>
                  <tbody>
                    {orders.map((o) => (
                      <React.Fragment key={o.order_id}>
                        <tr
                          key={o.order_id}
                          className={`border-b th-border transition-colors text-sm ${selected === o.order_id ? 'bg-[#dc8548]/5' : 'th-hover'}`}
                        >
                          <td className="px-6 py-3">
                            <div className="font-mono text-xs th-muted">{o.order_id.slice(0, 8)}…</div>
                            <div className="text-[10px] th-faint mt-0.5">
                              {new Date(o.created_at).toLocaleDateString()}
                            </div>
                          </td>
                          <td className="px-6 py-3 font-mono text-xs th-dim">{o.invoice_id.slice(0, 8)}…</td>
                          <td className="px-6 py-3 font-mono text-xs th-text">{formatJUNO(o.amount_zat)} JUNO</td>
                          <td className="px-6 py-3 font-mono text-xs th-dim">
                            {formatJUNO(o.received_zat_confirmed)} / {formatJUNO(o.amount_zat)}
                          </td>
                          <td className="px-6 py-3">
                            <StatusBadge status={o.status} />
                          </td>
                          <td className="px-6 py-3">
                            <button
                              type="button"
                              onClick={() => setSelected(selected === o.order_id ? null : o.order_id)}
                              className="text-xs border th-border th-hover hover:border-[#dc8548]/40 hover:text-[#dc8548] th-muted px-3 py-1.5 rounded-lg transition-colors"
                            >
                              {selected === o.order_id ? 'Close' : 'View'}
                            </button>
                          </td>
                        </tr>
                        {selected === o.order_id && (
                          <tr
                            key={`${o.order_id}-detail`}
                            className="border-b th-border bg-[#dc8548]/5"
                          >
                            <td
                              colSpan={6}
                              className="px-6 py-5"
                            >
                              <InvoiceCheckoutCard
                                order={o}
                                onOrderUpdate={handleOrderUpdate}
                                hidePhasePill
                              />
                            </td>
                          </tr>
                        )}
                      </React.Fragment>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </main>
      </div>
    </div>
  )
}

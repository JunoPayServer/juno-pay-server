"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { getPublicInvoice, getPublicStatus, listPublicInvoiceEvents, type Invoice, type InvoiceEvent } from "@/app/actions";
import { confirmationsCount, depositHeightForConfirmations } from "@/lib/invoice";

export type LiveInvoice = {
  invoice: Invoice | null;
  bestHeight: number | null;
  confirmations: number | null;
  depositHeight: number | null;
  events: InvoiceEvent[];
  nextCursor: string;
  loading: boolean;
  error: string | null;
};

export function useLiveInvoice(input: { invoice_id: string; invoice_token: string; cursor?: string } | null, opts?: { pollMs?: number }): LiveInvoice {
  const pollMs = Math.max(250, opts?.pollMs ?? 1000);

  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [bestHeight, setBestHeight] = useState<number | null>(null);
  const [events, setEvents] = useState<InvoiceEvent[]>([]);
  const [nextCursor, setNextCursor] = useState("0");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const cursorRef = useRef("0");
  const inflightRef = useRef(false);

  useEffect(() => {
    setError(null);
    setInvoice(null);
    setBestHeight(null);
    setEvents([]);
    const c = (input?.cursor ?? "0").trim() || "0";
    cursorRef.current = c;
    setNextCursor(c);
  }, [input?.invoice_id, input?.invoice_token]);

  useEffect(() => {
    if (!input) return;

    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    async function tick() {
      if (stopped) return;
      if (inflightRef.current) return;
      inflightRef.current = true;
      setLoading(true);

      try {
        const [invRes, statusRes, eventsRes] = await Promise.all([
          getPublicInvoice({ invoice_id: input.invoice_id, invoice_token: input.invoice_token }),
          getPublicStatus(),
          listPublicInvoiceEvents({ invoice_id: input.invoice_id, invoice_token: input.invoice_token, cursor: cursorRef.current }),
        ]);

        if (stopped) return;

        if (invRes.ok) setInvoice(invRes.data);
        else setError(invRes.error);

        if (statusRes.ok) setBestHeight(statusRes.data.chain.best_height);

        if (eventsRes.ok) {
          if (eventsRes.data.events.length) {
            setEvents((prev) => [...prev, ...eventsRes.data.events]);
          }
          const nc = (eventsRes.data.next_cursor ?? "").trim();
          if (nc && nc !== "0" && nc !== cursorRef.current) {
            cursorRef.current = nc;
            setNextCursor(nc);
          }
        }
      } catch (e) {
        if (!stopped) setError(e instanceof Error ? e.message : "update failed");
      } finally {
        inflightRef.current = false;
        if (!stopped) setLoading(false);
      }
    }

    void tick();

    const schedule = () => {
      timer = setTimeout(async () => {
        await tick();
        if (!stopped) schedule();
      }, pollMs);
    };

    schedule();

    return () => {
      stopped = true;
      if (timer) clearTimeout(timer);
    };
  }, [input?.invoice_id, input?.invoice_token, pollMs]);

  const depositHeight = useMemo(() => depositHeightForConfirmations(events), [events]);
  const confirmations = useMemo(() => confirmationsCount(bestHeight, depositHeight), [bestHeight, depositHeight]);

  return { invoice, bestHeight, confirmations, depositHeight, events, nextCursor, loading, error };
}


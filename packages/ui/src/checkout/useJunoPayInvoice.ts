"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { confirmationsCount, depositHeightForConfirmations } from "./invoice.js";
import type { JunoPayClient } from "./api.js";
import type { JunoPayInvoice, JunoPayInvoiceEvent } from "./types.js";

export type JunoPayLiveInvoice = {
  invoice: JunoPayInvoice | null;
  bestHeight: number | null;
  confirmations: number | null;
  depositHeight: number | null;
  events: JunoPayInvoiceEvent[];
  nextCursor: string;
  loading: boolean;
  error: string | null;
};

export function useJunoPayInvoice(
  client: Pick<JunoPayClient, "getPublicInvoice" | "getStatus" | "listPublicInvoiceEvents"> | null,
  input: { invoice_id: string; invoice_token: string; cursor?: string } | null,
  opts?: { pollMs?: number },
): JunoPayLiveInvoice {
  const pollMs = Math.max(250, opts?.pollMs ?? 1000);

  const [invoice, setInvoice] = useState<JunoPayInvoice | null>(null);
  const [bestHeight, setBestHeight] = useState<number | null>(null);
  const [events, setEvents] = useState<JunoPayInvoiceEvent[]>([]);
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
    if (!client || !input) return;

    const activeClient = client;
    const invoiceID = input.invoice_id;
    const invoiceToken = input.invoice_token;

    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    async function tick() {
      if (stopped || inflightRef.current) return;
      inflightRef.current = true;
      setLoading(true);

      try {
        const [invoiceRes, statusRes, eventsRes] = await Promise.all([
          activeClient.getPublicInvoice({ invoice_id: invoiceID, invoice_token: invoiceToken }),
          activeClient.getStatus(),
          activeClient.listPublicInvoiceEvents({ invoice_id: invoiceID, invoice_token: invoiceToken, cursor: cursorRef.current }),
        ]);

        if (stopped) return;

        if (invoiceRes.ok) {
          setInvoice((prev) => {
            const next = invoiceRes.data;
            if (
              prev &&
              prev.status === next.status &&
              prev.received_zat_pending === next.received_zat_pending &&
              prev.received_zat_confirmed === next.received_zat_confirmed &&
              prev.updated_at === next.updated_at
            ) {
              return prev;
            }
            return next;
          });
        } else {
          setError(invoiceRes.error);
        }

        if (statusRes.ok) setBestHeight(statusRes.data.chain.best_height);

        if (eventsRes.ok) {
          if (eventsRes.data.events.length) setEvents((prev) => [...prev, ...eventsRes.data.events]);
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
  }, [client, input?.invoice_id, input?.invoice_token, pollMs]);

  const depositHeight = useMemo(() => depositHeightForConfirmations(events), [events]);
  const confirmations = useMemo(() => confirmationsCount(bestHeight, depositHeight), [bestHeight, depositHeight]);

  return { invoice, bestHeight, confirmations, depositHeight, events, nextCursor, loading, error };
}

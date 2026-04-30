"use server";

type ErrorBody = {
  status: "error";
  error: { code: string; message: string };
};

type OkBody<T> = {
  status: "ok";
  data: T;
};

export type ActionResult<T> =
  | {
      ok: true;
      data: T;
    }
  | {
      ok: false;
      error: string;
      code?: string;
    };

export type Invoice = {
  invoice_id: string;
  merchant_id: string;
  external_order_id: string;
  status: string;
  address: string;
  amount_zat: number;
  required_confirmations: number;
  received_zat_pending: number;
  received_zat_confirmed: number;
  expires_at?: string | null;
  created_at: string;
  updated_at: string;
};

export type PublicInvoice = {
  invoice: Invoice;
  invoice_token: string;
};

export type DepositRef = {
  wallet_id: string;
  txid: string;
  action_index: number;
  amount_zat: number;
  height: number;
};

export type InvoiceEvent = {
  event_id: string;
  type: string;
  occurred_at: string;
  invoice_id: string;
  deposit?: DepositRef | null;
  refund?: unknown | null;
};

export type InvoiceEventsPage = {
  events: InvoiceEvent[];
  next_cursor: string;
};

export type StatusSnapshot = {
  chain: {
    best_height: number;
    best_hash: string;
    uptime_seconds: number;
  };
  scanner: {
    connected: boolean;
    last_cursor_applied: number;
    last_event_at?: string | null;
  };
  event_delivery: {
    pending_deliveries: number;
  };
};

function baseURL(): string | null {
  const v = (process.env.JUNO_PAY_BASE_URL ?? "").trim();
  return v ? v.replace(/\/+$/, "") : null;
}

function merchantAPIKey(): string | null {
  const v = (process.env.JUNO_PAY_MERCHANT_API_KEY ?? "").trim();
  return v ? v : null;
}

async function parseJSONSafe(res: Response): Promise<unknown | undefined> {
  const text = await res.text();
  if (text.trim() === "") return undefined;
  return JSON.parse(text);
}

async function fetchAPI<T>(path: string, init?: RequestInit): Promise<ActionResult<T>> {
  const base = baseURL();
  if (!base) {
    return { ok: false, code: "config", error: "JUNO_PAY_BASE_URL is not set" };
  }

  let res: Response;
  try {
    res = await fetch(`${base}${path}`, { ...init, cache: "no-store" });
  } catch (e) {
    return { ok: false, code: "network_error", error: e instanceof Error ? e.message : "network error" };
  }

  let body: unknown = undefined;
  try {
    body = await parseJSONSafe(res);
  } catch {
    body = undefined;
  }

  if (!res.ok) {
    const b = body as Partial<ErrorBody> | undefined;
    const msg = b?.error?.message ?? `HTTP ${res.status}`;
    return { ok: false, code: b?.error?.code, error: msg };
  }

  const ok = body as OkBody<T>;
  if (!ok || ok.status !== "ok") {
    return { ok: false, code: "invalid_response", error: "invalid response" };
  }

  return { ok: true, data: ok.data };
}

export async function createAirInvoice(input: { external_order_id: string; demo_user_id: string; email?: string }): Promise<ActionResult<PublicInvoice>> {
  const key = merchantAPIKey();
  if (!key) {
    return { ok: false, code: "config", error: "Demo is not configured yet (missing merchant API key). Try again in a minute." };
  }

  const amountZat = 100_000_000; // 1 JUNO
  return fetchAPI<PublicInvoice>("/v1/invoices", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${key}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      external_order_id: input.external_order_id,
      amount_zat: amountZat,
      metadata: {
        demo_user_id: input.demo_user_id,
        email: input.email ?? null,
        item: "gallon_of_air",
      },
    }),
  });
}

export async function getPublicInvoice(input: { invoice_id: string; invoice_token: string }): Promise<ActionResult<Invoice>> {
  const q = new URLSearchParams({ token: input.invoice_token });
  return fetchAPI<Invoice>(`/v1/public/invoices/${encodeURIComponent(input.invoice_id)}?${q.toString()}`, { method: "GET" });
}

export async function listPublicInvoiceEvents(input: {
  invoice_id: string;
  invoice_token: string;
  cursor?: string;
}): Promise<ActionResult<InvoiceEventsPage>> {
  const q = new URLSearchParams({ token: input.invoice_token });
  if (input.cursor && input.cursor.trim() !== "") {
    q.set("cursor", input.cursor.trim());
  }
  return fetchAPI<InvoiceEventsPage>(`/v1/public/invoices/${encodeURIComponent(input.invoice_id)}/events?${q.toString()}`, { method: "GET" });
}

export async function getPublicStatus(): Promise<ActionResult<StatusSnapshot>> {
  return fetchAPI<StatusSnapshot>("/v1/status", { method: "GET" });
}

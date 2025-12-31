"use server";

type ErrorBody = {
  status: "error";
  error: { code: string; message: string };
};

type OkBody<T> = {
  status: "ok";
  data: T;
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

function baseURL(): string {
  const v = (process.env.JUNO_PAY_BASE_URL ?? "").trim();
  if (!v) {
    throw new Error("JUNO_PAY_BASE_URL is not set");
  }
  return v.replace(/\/+$/, "");
}

function merchantAPIKey(): string {
  const v = (process.env.JUNO_PAY_MERCHANT_API_KEY ?? "").trim();
  if (!v) {
    throw new Error("JUNO_PAY_MERCHANT_API_KEY is not set");
  }
  return v;
}

async function parseJSONSafe(res: Response): Promise<unknown | undefined> {
  const text = await res.text();
  if (text.trim() === "") return undefined;
  return JSON.parse(text);
}

async function fetchOK<T>(path: string, init?: RequestInit): Promise<T> {
  const url = `${baseURL()}${path}`;
  const res = await fetch(url, { ...init, cache: "no-store" });

  let body: unknown = undefined;
  try {
    body = await parseJSONSafe(res);
  } catch {
    body = undefined;
  }

  if (!res.ok) {
    const b = body as Partial<ErrorBody> | undefined;
    const msg = b?.error?.message ?? `HTTP ${res.status}`;
    throw new Error(msg);
  }

  const ok = body as OkBody<T>;
  if (!ok || ok.status !== "ok") {
    throw new Error("invalid response");
  }
  return ok.data;
}

export async function createAirInvoice(input: { external_order_id: string; demo_user_id: string; email?: string }): Promise<PublicInvoice> {
  const amountZat = 100_000_000; // 1 JUNO
  return fetchOK<PublicInvoice>("/v1/invoices", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${merchantAPIKey()}`,
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

export async function getPublicInvoice(input: { invoice_id: string; invoice_token: string }): Promise<Invoice> {
  const q = new URLSearchParams({ token: input.invoice_token });
  return fetchOK<Invoice>(`/v1/public/invoices/${encodeURIComponent(input.invoice_id)}?${q.toString()}`, { method: "GET" });
}

export async function listPublicInvoiceEvents(input: {
  invoice_id: string;
  invoice_token: string;
  cursor?: string;
}): Promise<InvoiceEventsPage> {
  const q = new URLSearchParams({ token: input.invoice_token });
  if (input.cursor && input.cursor.trim() !== "") {
    q.set("cursor", input.cursor.trim());
  }
  return fetchOK<InvoiceEventsPage>(`/v1/public/invoices/${encodeURIComponent(input.invoice_id)}/events?${q.toString()}`, { method: "GET" });
}


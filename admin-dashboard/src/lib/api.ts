export type APIErrorBody = {
  status: "error";
  error: {
    code: string;
    message: string;
  };
};

export class APIError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "APIError";
    this.status = status;
    this.code = code;
  }
}

type OkResponse<T> = {
  status: "ok";
  data: T;
};

async function parseJSONSafe(res: Response): Promise<unknown | undefined> {
  const text = await res.text();
  if (text.trim() === "") return undefined;
  return JSON.parse(text);
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });

  if (res.status === 204) {
    return undefined as T;
  }

  let body: unknown;
  try {
    body = await parseJSONSafe(res);
  } catch {
    body = undefined;
  }

  if (!res.ok) {
    const b = body as Partial<APIErrorBody> | undefined;
    const code = b?.error?.code;
    const message = b?.error?.message ?? `HTTP ${res.status}`;
    throw new APIError(message, res.status, code);
  }

  const ok = body as OkResponse<T>;
  if (!ok || ok.status !== "ok") {
    throw new APIError("invalid response", res.status);
  }
  return ok.data;
}

export type MerchantSettings = {
  invoice_ttl_seconds: number;
  required_confirmations: number;
  policies: {
    late_payment_policy: string;
    partial_payment_policy: string;
    overpayment_policy: string;
  };
};

export type MerchantWallet = {
  merchant_id: string;
  wallet_id: string;
  ufvk: string;
  chain: string;
  ua_hrp: string;
  coin_type: number;
  created_at: string;
};

export type MerchantAPIKey = {
  key_id: string;
  merchant_id: string;
  label: string;
  created_at: string;
  revoked_at: string | null;
};

export type Merchant = {
  merchant_id: string;
  name: string;
  status: string;
  settings: MerchantSettings;
  wallet?: MerchantWallet | null;
  api_keys?: MerchantAPIKey[];
  created_at: string;
  updated_at: string;
};

export type EventSink = {
  sink_id: string;
  merchant_id: string;
  kind: string;
  status: string;
  config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type CloudEvent = {
  specversion: string;
  id: string;
  source: string;
  type: string;
  subject?: string;
  time: string;
  datacontenttype: string;
  data: unknown;
};

export type EventDelivery = {
  delivery_id: string;
  sink_id: string;
  event_id: string;
  status: string;
  attempt: number;
  next_retry_at: string | null;
  last_error: string | null;
  created_at: string;
  updated_at: string;
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
  expires_at: string | null;
  created_at: string;
  updated_at: string;
};

export type InvoiceDetails = Invoice & {
  wallet_id: string;
  address_index: number;
  created_after_height: number;
  created_after_hash: string;
  policies: {
    late_payment_policy: string;
    partial_payment_policy: string;
    overpayment_policy: string;
  };
};

export type Deposit = {
  wallet_id: string;
  txid: string;
  action_index: number;
  recipient_address: string;
  amount_zat: number;
  height: number;
  status: string;
  confirmed_height: number | null;
  invoice_id: string | null;
  detected_at: string;
  updated_at: string;
};

export type Refund = {
  refund_id: string;
  merchant_id: string;
  invoice_id: string | null;
  external_refund_id: string | null;
  to_address: string;
  amount_zat: number;
  status: string;
  sent_txid: string | null;
  notes: string;
  created_at: string;
  updated_at: string;
};

export type ReviewCase = {
  review_id: string;
  merchant_id: string;
  invoice_id: string | null;
  reason: string;
  status: string;
  notes: string;
  created_at: string;
  updated_at: string;
};

export type StatusSnapshot = {
  chain: {
    best_height: number;
    best_hash: string;
    uptime_seconds: number | null;
  };
  scanner: {
    connected: boolean;
    last_cursor_applied: number;
    last_event_at: string | null;
  };
  event_delivery: {
    pending_deliveries: number;
  };
};

export type AdminStatusSnapshot = StatusSnapshot & {
  restarts: {
    junocashd_restarts_detected: number;
    last_restart_at: string | null;
  };
};

export async function adminLogin(password: string): Promise<void> {
  const body = JSON.stringify({ password });
  const res = await fetch("/admin/login", { method: "POST", body, headers: { "Content-Type": "application/json" } });
  if (res.status === 204) return;
  let msg = `HTTP ${res.status}`;
  try {
    const b = (await parseJSONSafe(res)) as Partial<APIErrorBody> | undefined;
    msg = b?.error?.message ?? msg;
  } catch {}
  throw new APIError(msg, res.status);
}

export async function adminLogout(): Promise<void> {
  const res = await fetch("/admin/logout", { method: "POST" });
  if (res.status === 204) return;
  throw new APIError(`HTTP ${res.status}`, res.status);
}

export function getAdminStatus(): Promise<AdminStatusSnapshot> {
  return fetchJSON<AdminStatusSnapshot>("/v1/admin/status");
}

export function listMerchants(): Promise<Merchant[]> {
  return fetchJSON<Merchant[]>("/v1/admin/merchants");
}

export function createMerchant(name: string, settings?: MerchantSettings): Promise<Merchant> {
  return fetchJSON<Merchant>("/v1/admin/merchants", {
    method: "POST",
    body: JSON.stringify({ name, settings }),
  });
}

export function getMerchant(merchantId: string): Promise<Merchant> {
  return fetchJSON<Merchant>(`/v1/admin/merchants/${encodeURIComponent(merchantId)}`);
}

export function setMerchantWallet(merchantId: string, wallet: { ufvk: string; chain: string; ua_hrp: string; coin_type: number; wallet_id: string }): Promise<MerchantWallet> {
  return fetchJSON<MerchantWallet>(`/v1/admin/merchants/${encodeURIComponent(merchantId)}/wallet`, {
    method: "POST",
    body: JSON.stringify(wallet),
  });
}

export function setMerchantSettings(merchantId: string, settings: MerchantSettings): Promise<Merchant> {
  return fetchJSON<Merchant>(`/v1/admin/merchants/${encodeURIComponent(merchantId)}/settings`, {
    method: "PUT",
    body: JSON.stringify(settings),
  });
}

export function createAPIKey(merchantId: string, label: string): Promise<{ api_key: string; key: MerchantAPIKey }> {
  return fetchJSON<{ api_key: string; key: MerchantAPIKey }>(`/v1/admin/merchants/${encodeURIComponent(merchantId)}/api-keys`, {
    method: "POST",
    body: JSON.stringify({ label }),
  });
}

export function revokeAPIKey(keyId: string): Promise<unknown> {
  return fetchJSON(`/v1/admin/api-keys/${encodeURIComponent(keyId)}`, { method: "DELETE" });
}

export function listReviewCases(params?: { merchant_id?: string; status?: string }): Promise<ReviewCase[]> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  if (params?.status) p.set("status", params.status);
  const q = p.toString();
  return fetchJSON<ReviewCase[]>(`/v1/admin/review-cases${q ? `?${q}` : ""}`);
}

export function resolveReviewCase(reviewId: string, notes: string): Promise<unknown> {
  return fetchJSON(`/v1/admin/review-cases/${encodeURIComponent(reviewId)}/resolve`, {
    method: "POST",
    body: JSON.stringify({ notes }),
  });
}

export function rejectReviewCase(reviewId: string, notes: string): Promise<unknown> {
  return fetchJSON(`/v1/admin/review-cases/${encodeURIComponent(reviewId)}/reject`, {
    method: "POST",
    body: JSON.stringify({ notes }),
  });
}

export function listInvoices(params?: { merchant_id?: string; status?: string; external_order_id?: string; cursor?: string; limit?: string }): Promise<{ invoices: Invoice[]; next_cursor: string }> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  if (params?.status) p.set("status", params.status);
  if (params?.external_order_id) p.set("external_order_id", params.external_order_id);
  if (params?.cursor) p.set("cursor", params.cursor);
  if (params?.limit) p.set("limit", params.limit);
  const q = p.toString();
  return fetchJSON<{ invoices: Invoice[]; next_cursor: string }>(`/v1/admin/invoices${q ? `?${q}` : ""}`);
}

export function getInvoice(invoiceId: string): Promise<InvoiceDetails> {
  return fetchJSON<InvoiceDetails>(`/v1/admin/invoices/${encodeURIComponent(invoiceId)}`);
}

export function listDeposits(params?: { merchant_id?: string; invoice_id?: string; txid?: string; cursor?: string; limit?: string }): Promise<{ deposits: Deposit[]; next_cursor: string }> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  if (params?.invoice_id) p.set("invoice_id", params.invoice_id);
  if (params?.txid) p.set("txid", params.txid);
  if (params?.cursor) p.set("cursor", params.cursor);
  if (params?.limit) p.set("limit", params.limit);
  const q = p.toString();
  return fetchJSON<{ deposits: Deposit[]; next_cursor: string }>(`/v1/admin/deposits${q ? `?${q}` : ""}`);
}

export function createEventSink(req: { merchant_id: string; kind: string; config: Record<string, unknown> }): Promise<EventSink> {
  return fetchJSON<EventSink>("/v1/admin/event-sinks", { method: "POST", body: JSON.stringify(req) });
}

export function listEventSinks(params?: { merchant_id?: string }): Promise<EventSink[]> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  const q = p.toString();
  return fetchJSON<EventSink[]>(`/v1/admin/event-sinks${q ? `?${q}` : ""}`);
}

export function testEventSink(sinkId: string): Promise<unknown> {
  return fetchJSON(`/v1/admin/event-sinks/${encodeURIComponent(sinkId)}/test`, { method: "POST" });
}

export function listRefunds(params?: { merchant_id?: string; invoice_id?: string; status?: string; cursor?: string; limit?: string }): Promise<{ refunds: Refund[]; next_cursor: string }> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  if (params?.invoice_id) p.set("invoice_id", params.invoice_id);
  if (params?.status) p.set("status", params.status);
  if (params?.cursor) p.set("cursor", params.cursor);
  if (params?.limit) p.set("limit", params.limit);
  const q = p.toString();
  return fetchJSON<{ refunds: Refund[]; next_cursor: string }>(`/v1/admin/refunds${q ? `?${q}` : ""}`);
}

export function createRefund(req: { merchant_id: string; invoice_id?: string; external_refund_id?: string; to_address: string; amount_zat: number; sent_txid?: string; notes?: string }): Promise<Refund> {
  return fetchJSON<Refund>("/v1/admin/refunds", { method: "POST", body: JSON.stringify(req) });
}

export async function listOutboundEvents(params?: { merchant_id?: string; cursor?: string; limit?: string }): Promise<{ events: CloudEvent[]; next_cursor: string }> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  if (params?.cursor) p.set("cursor", params.cursor);
  if (params?.limit) p.set("limit", params.limit);
  const q = p.toString();
  const out = await fetchJSON<{ events: CloudEvent[] | null; next_cursor: string }>(`/v1/admin/events${q ? `?${q}` : ""}`);
  return { ...out, events: Array.isArray(out.events) ? out.events : [] };
}

export function listEventDeliveries(params?: { merchant_id?: string; sink_id?: string; status?: string; limit?: string }): Promise<EventDelivery[]> {
  const p = new URLSearchParams();
  if (params?.merchant_id) p.set("merchant_id", params.merchant_id);
  if (params?.sink_id) p.set("sink_id", params.sink_id);
  if (params?.status) p.set("status", params.status);
  if (params?.limit) p.set("limit", params.limit);
  const q = p.toString();
  return fetchJSON<EventDelivery[]>(`/v1/admin/event-deliveries${q ? `?${q}` : ""}`);
}

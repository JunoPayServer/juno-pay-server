import type {
  JunoPayInvoice,
  JunoPayInvoiceEventsPage,
  JunoPayPublicInvoice,
  JunoPayResult,
  JunoPayStatusSnapshot,
} from "./types.js";

type APIBody<T> =
  | {
      status: "ok";
      data: T;
    }
  | {
      status: "error";
      error: { code?: string; message?: string };
    };

export type JunoPayCreateInvoiceInput = {
  external_order_id: string;
  amount_zat: number;
  metadata?: Record<string, unknown>;
};

export type JunoPayClientOptions = {
  baseUrl: string;
  merchantApiKey?: string;
  headers?: HeadersInit | (() => HeadersInit | Promise<HeadersInit>);
  fetcher?: typeof fetch;
};

export type JunoPayClient = {
  createInvoice(input: JunoPayCreateInvoiceInput): Promise<JunoPayResult<JunoPayPublicInvoice>>;
  getPublicInvoice(input: { invoice_id: string; invoice_token: string }): Promise<JunoPayResult<JunoPayInvoice>>;
  listPublicInvoiceEvents(input: {
    invoice_id: string;
    invoice_token: string;
    cursor?: string;
  }): Promise<JunoPayResult<JunoPayInvoiceEventsPage>>;
  getStatus(): Promise<JunoPayResult<JunoPayStatusSnapshot>>;
};

async function parseJSONSafe(res: Response): Promise<unknown | undefined> {
  const text = await res.text();
  if (text.trim() === "") return undefined;
  return JSON.parse(text);
}

async function resolveHeaders(input?: HeadersInit | (() => HeadersInit | Promise<HeadersInit>)): Promise<Headers> {
  if (!input) return new Headers();
  if (typeof input === "function") return new Headers(await input());
  return new Headers(input);
}

export function createJunoPayClient(options: JunoPayClientOptions): JunoPayClient {
  const baseUrl = options.baseUrl.replace(/\/+$/, "");
  const fetcher = options.fetcher ?? fetch;

  async function request<T>(path: string, init?: RequestInit): Promise<JunoPayResult<T>> {
    let headers = await resolveHeaders(options.headers);
    const initHeaders = new Headers(init?.headers);
    initHeaders.forEach((value, key) => headers.set(key, value));

    let res: Response;
    try {
      res = await fetcher(`${baseUrl}${path}`, { ...init, headers, cache: "no-store" });
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
      const err = body as Partial<Extract<APIBody<T>, { status: "error" }>> | undefined;
      return {
        ok: false,
        code: err?.error?.code,
        error: err?.error?.message ?? `HTTP ${res.status}`,
      };
    }

    const ok = body as APIBody<T> | undefined;
    if (!ok || ok.status !== "ok") {
      return { ok: false, code: "invalid_response", error: "invalid response" };
    }

    return { ok: true, data: ok.data };
  }

  return {
    createInvoice(input) {
      if (!options.merchantApiKey) {
        return Promise.resolve({ ok: false, code: "config", error: "merchantApiKey is required to create invoices" });
      }

      return request<JunoPayPublicInvoice>("/v1/invoices", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${options.merchantApiKey}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(input),
      });
    },
    getPublicInvoice(input) {
      const q = new URLSearchParams({ token: input.invoice_token });
      return request<JunoPayInvoice>(`/v1/public/invoices/${encodeURIComponent(input.invoice_id)}?${q.toString()}`, { method: "GET" });
    },
    listPublicInvoiceEvents(input) {
      const q = new URLSearchParams({ token: input.invoice_token });
      if (input.cursor && input.cursor.trim() !== "") q.set("cursor", input.cursor.trim());
      return request<JunoPayInvoiceEventsPage>(`/v1/public/invoices/${encodeURIComponent(input.invoice_id)}/events?${q.toString()}`, { method: "GET" });
    },
    getStatus() {
      return request<JunoPayStatusSnapshot>("/v1/status", { method: "GET" });
    },
  };
}

export type JunoPayInvoice = {
  invoice_id: string;
  merchant_id?: string;
  external_order_id?: string;
  status: string;
  address: string;
  amount_zat: number;
  required_confirmations: number;
  received_zat_pending: number;
  received_zat_confirmed: number;
  expires_at?: string | null;
  created_at?: string;
  updated_at?: string;
};

export type JunoPayInvoicePhase = "awaiting_payment" | "pending_confirmations" | "payment_complete" | "expired";

export type JunoPayPublicInvoice = {
  invoice: JunoPayInvoice;
  invoice_token: string;
};

export type JunoPayDepositRef = {
  wallet_id: string;
  txid: string;
  action_index: number;
  amount_zat: number;
  height: number;
};

export type JunoPayInvoiceEvent = {
  event_id: string;
  type: string;
  occurred_at: string;
  invoice_id: string;
  deposit?: JunoPayDepositRef | null;
  refund?: unknown | null;
};

export type JunoPayInvoiceEventsPage = {
  events: JunoPayInvoiceEvent[];
  next_cursor: string;
};

export type JunoPayStatusSnapshot = {
  chain: {
    best_height: number;
    best_hash?: string;
    uptime_seconds?: number;
  };
  scanner?: {
    connected?: boolean;
    last_cursor_applied?: number;
    last_event_at?: string | null;
  };
  event_delivery?: {
    pending_deliveries?: number;
  };
};

export type JunoPayResult<T> =
  | { ok: true; data: T }
  | { ok: false; error: string; code?: string };

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

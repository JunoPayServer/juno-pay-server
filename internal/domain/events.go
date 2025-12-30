package domain

import "time"

type InvoiceEventType string

const (
	InvoiceEventInvoiceCreated   InvoiceEventType = "invoice.created"
	InvoiceEventInvoiceExpired   InvoiceEventType = "invoice.expired"
	InvoiceEventDepositDetected  InvoiceEventType = "deposit.detected"
	InvoiceEventDepositConfirmed InvoiceEventType = "deposit.confirmed"
	InvoiceEventInvoicePaid      InvoiceEventType = "invoice.paid"
	InvoiceEventInvoiceOverpaid  InvoiceEventType = "invoice.overpaid"
	InvoiceEventRefundRequested  InvoiceEventType = "refund.requested"
	InvoiceEventRefundSent       InvoiceEventType = "refund.sent"
)

type DepositRef struct {
	WalletID    string
	TxID        string
	ActionIndex int32
	AmountZat   int64
	Height      int64
}

type InvoiceEvent struct {
	EventID    string
	Type       InvoiceEventType
	OccurredAt time.Time
	InvoiceID  string
	Deposit    *DepositRef
	Refund     *Refund
}

package domain

import "time"

type RefundStatus string

const (
	RefundRequested RefundStatus = "requested"
	RefundSent      RefundStatus = "sent"
	RefundCanceled  RefundStatus = "canceled"
)

type Refund struct {
	RefundID string

	MerchantID string
	InvoiceID  *string

	ExternalRefundID *string
	ToAddress        string
	AmountZat        int64
	Status           RefundStatus
	SentTxID         *string
	Notes            string

	CreatedAt time.Time
	UpdatedAt time.Time
}

package domain

import "time"

type ReviewStatus string

const (
	ReviewOpen     ReviewStatus = "open"
	ReviewResolved ReviewStatus = "resolved"
	ReviewRejected ReviewStatus = "rejected"
)

type ReviewReason string

const (
	ReviewLatePayment    ReviewReason = "late_payment"
	ReviewPartialPayment ReviewReason = "partial_payment"
	ReviewOverpayment    ReviewReason = "overpayment"
	ReviewUnknownAddress ReviewReason = "unknown_address"
	ReviewReorg          ReviewReason = "reorg"
	ReviewManualRefund   ReviewReason = "manual_refund"
)

type ReviewCase struct {
	ReviewID string

	MerchantID string
	InvoiceID  *string

	Reason ReviewReason
	Status ReviewStatus
	Notes  string

	CreatedAt time.Time
	UpdatedAt time.Time
}

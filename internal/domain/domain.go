package domain

import (
	"errors"
	"fmt"
	"time"
)

type ErrorCode string

const (
	ErrInvalidArgument ErrorCode = "invalid_argument"
	ErrInvariantViolated ErrorCode = "invariant_violated"
)

type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string { return e.Message }

func NewError(code ErrorCode, msg string, args ...any) *Error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(msg, args...),
	}
}

func AsError(err error) (*Error, bool) {
	var de *Error
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

type MerchantStatus string

const (
	MerchantActive   MerchantStatus = "active"
	MerchantDisabled MerchantStatus = "disabled"
)

type LatePaymentPolicy string

const (
	LatePaymentMarkPaidLate LatePaymentPolicy = "mark_paid_late"
	LatePaymentManualReview LatePaymentPolicy = "manual_review"
	LatePaymentIgnore       LatePaymentPolicy = "ignore"
)

type PartialPaymentPolicy string

const (
	PartialPaymentAccept PartialPaymentPolicy = "accept_partial"
	PartialPaymentReject PartialPaymentPolicy = "reject_partial"
)

type OverpaymentPolicy string

const (
	OverpaymentMarkOverpaid OverpaymentPolicy = "mark_overpaid"
	OverpaymentManualReview OverpaymentPolicy = "manual_review"
)

type InvoicePolicies struct {
	LatePayment    LatePaymentPolicy
	PartialPayment PartialPaymentPolicy
	Overpayment    OverpaymentPolicy
}

type MerchantSettings struct {
	InvoiceTTLSeconds     int64
	RequiredConfirmations int32
	Policies              InvoicePolicies
}

func (s MerchantSettings) InvoiceTTL() time.Duration {
	return time.Duration(s.InvoiceTTLSeconds) * time.Second
}

func (s MerchantSettings) Validate() error {
	if s.InvoiceTTLSeconds < 0 {
		return NewError(ErrInvalidArgument, "invoice_ttl_seconds must be >= 0")
	}
	if s.RequiredConfirmations < 0 {
		return NewError(ErrInvalidArgument, "required_confirmations must be >= 0")
	}

	switch s.Policies.LatePayment {
	case LatePaymentMarkPaidLate, LatePaymentManualReview, LatePaymentIgnore:
	default:
		return NewError(ErrInvalidArgument, "late_payment_policy invalid")
	}
	switch s.Policies.PartialPayment {
	case PartialPaymentAccept, PartialPaymentReject:
	default:
		return NewError(ErrInvalidArgument, "partial_payment_policy invalid")
	}
	switch s.Policies.Overpayment {
	case OverpaymentMarkOverpaid, OverpaymentManualReview:
	default:
		return NewError(ErrInvalidArgument, "overpayment_policy invalid")
	}

	return nil
}

type Merchant struct {
	MerchantID string
	Name       string
	Status     MerchantStatus
	Settings   MerchantSettings

	CreatedAt time.Time
	UpdatedAt time.Time
}

type InvoiceStatus string

const (
	InvoiceOpen             InvoiceStatus = "open"
	InvoicePartialPending   InvoiceStatus = "partial_pending"
	InvoicePending          InvoiceStatus = "pending"
	InvoicePartialConfirmed InvoiceStatus = "partial_confirmed"
	InvoiceConfirmed        InvoiceStatus = "confirmed"
	InvoiceOverpaid         InvoiceStatus = "overpaid"
	InvoiceExpired          InvoiceStatus = "expired"
	InvoicePaidLate         InvoiceStatus = "paid_late"
	InvoiceCanceled         InvoiceStatus = "canceled"
)

type Invoice struct {
	InvoiceID        string
	MerchantID       string
	ExternalOrderID  string
	WalletID         string
	AddressIndex     uint32
	Address          string
	CreatedAfterHeight int64
	CreatedAfterHash   string

	AmountZat            int64
	RequiredConfirmations int32
	Policies              InvoicePolicies

	ReceivedPendingZat   int64
	ReceivedConfirmedZat int64
	Status               InvoiceStatus
	ExpiresAt             *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (inv Invoice) CanApplyDeposit(depositHeight int64) bool {
	return depositHeight > inv.CreatedAfterHeight
}

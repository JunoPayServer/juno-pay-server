package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

type MerchantWallet struct {
	MerchantID string
	WalletID   string

	UFVK     string
	Chain    string
	UAHRP    string
	CoinType int32

	CreatedAt time.Time
}

type InvoiceCreate struct {
	MerchantID      string
	ExternalOrderID string

	WalletID           string
	AddressIndex       uint32
	Address            string
	CreatedAfterHeight int64
	CreatedAfterHash   string

	AmountZat             int64
	RequiredConfirmations int32
	Policies              domain.InvoicePolicies
	ExpiresAt             *time.Time
}

type InvoiceFilter struct {
	MerchantID      string
	Status          domain.InvoiceStatus
	ExternalOrderID string

	AfterID int64
	Limit   int
}

type DepositFilter struct {
	MerchantID string
	InvoiceID  string
	TxID       string

	AfterID int64
	Limit   int
}

type RefundCreate struct {
	MerchantID string
	InvoiceID  string

	ExternalRefundID string
	ToAddress        string
	AmountZat        int64
	SentTxID         string
	Notes            string
}

type RefundFilter struct {
	MerchantID string
	InvoiceID  string
	Status     domain.RefundStatus

	AfterID int64
	Limit   int
}

type ReviewCaseFilter struct {
	MerchantID string
	Status     domain.ReviewStatus
}

type ScanEvent struct {
	WalletID   string
	Cursor     int64
	Kind       string
	Height     int64
	Payload    json.RawMessage
	OccurredAt time.Time
}

type EventSinkCreate struct {
	MerchantID string
	Kind       domain.EventSinkKind
	Config     json.RawMessage
}

type EventDeliveryFilter struct {
	MerchantID string
	SinkID     string
	Status     domain.EventDeliveryStatus
	Limit      int
}

type DueDelivery struct {
	Delivery domain.EventDelivery
	Sink     domain.EventSink
	Event    domain.CloudEvent
}

type Store interface {
	// Merchants
	CreateMerchant(ctx context.Context, name string, settings domain.MerchantSettings) (domain.Merchant, error)
	GetMerchant(ctx context.Context, merchantID string) (domain.Merchant, bool, error)
	ListMerchants(ctx context.Context) ([]domain.Merchant, error)
	UpdateMerchantSettings(ctx context.Context, merchantID string, settings domain.MerchantSettings) (domain.Merchant, error)

	// Wallets (one UFVK per merchant; immutable)
	SetMerchantWallet(ctx context.Context, merchantID string, w MerchantWallet) (MerchantWallet, error)
	GetMerchantWallet(ctx context.Context, merchantID string) (MerchantWallet, bool, error)
	ListMerchantWallets(ctx context.Context) ([]MerchantWallet, error)
	NextAddressIndex(ctx context.Context, merchantID string) (uint32, error)

	// Merchant API keys (for invoice creation only)
	CreateMerchantAPIKey(ctx context.Context, merchantID, label string) (keyID string, apiKey string, err error)
	RevokeMerchantAPIKey(ctx context.Context, keyID string) error
	LookupMerchantIDByAPIKey(ctx context.Context, apiKey string) (merchantID string, ok bool, err error)

	// Invoices (idempotent by merchant_id + external_order_id)
	CreateInvoice(ctx context.Context, req InvoiceCreate) (domain.Invoice, bool, error)
	GetInvoice(ctx context.Context, invoiceID string) (domain.Invoice, bool, error)
	FindInvoiceByExternalOrderID(ctx context.Context, merchantID, externalOrderID string) (domain.Invoice, bool, error)
	ListInvoices(ctx context.Context, f InvoiceFilter) (invoices []domain.Invoice, nextCursor int64, err error)

	// Invoice checkout token (stored encrypted-at-rest in production DB).
	PutInvoiceToken(ctx context.Context, invoiceID string, token string) error
	GetInvoiceToken(ctx context.Context, invoiceID string) (token string, ok bool, err error)

	// Scan ingestion + invoice accounting.
	ScanCursor(ctx context.Context, walletID string) (cursor int64, err error)
	ApplyScanEvent(ctx context.Context, ev ScanEvent) error

	// Invoice events (for polling and SSE).
	ListInvoiceEvents(ctx context.Context, invoiceID string, afterID int64, limit int) (events []domain.InvoiceEvent, nextCursor int64, err error)

	// Deposits (debug/admin).
	ListDeposits(ctx context.Context, f DepositFilter) (deposits []domain.Deposit, nextCursor int64, err error)

	// Refunds (manual recordkeeping; no signing/broadcast).
	CreateRefund(ctx context.Context, req RefundCreate) (domain.Refund, error)
	ListRefunds(ctx context.Context, f RefundFilter) (refunds []domain.Refund, nextCursor int64, err error)

	// Review cases (manual review queue).
	ListReviewCases(ctx context.Context, f ReviewCaseFilter) ([]domain.ReviewCase, error)
	ResolveReviewCase(ctx context.Context, reviewID string, notes string) error
	RejectReviewCase(ctx context.Context, reviewID string, notes string) error

	// Outbound events (webhook/brokers) + sinks.
	CreateEventSink(ctx context.Context, req EventSinkCreate) (domain.EventSink, error)
	GetEventSink(ctx context.Context, sinkID string) (domain.EventSink, bool, error)
	ListEventSinks(ctx context.Context, merchantID string) ([]domain.EventSink, error)
	ListOutboundEvents(ctx context.Context, merchantID string, afterID int64, limit int) (events []domain.CloudEvent, nextCursor int64, err error)
	ListEventDeliveries(ctx context.Context, f EventDeliveryFilter) ([]domain.EventDelivery, error)
	ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]DueDelivery, error)
	UpdateEventDelivery(ctx context.Context, deliveryID string, status domain.EventDeliveryStatus, attempt int32, nextRetryAt *time.Time, lastError *string) error
}

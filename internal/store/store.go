package store

import (
	"context"
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

	WalletID          string
	AddressIndex      uint32
	Address           string
	CreatedAfterHeight int64
	CreatedAfterHash   string

	AmountZat             int64
	RequiredConfirmations int32
	Policies              domain.InvoicePolicies
	ExpiresAt             *time.Time
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
	NextAddressIndex(ctx context.Context, merchantID string) (uint32, error)

	// Merchant API keys (for invoice creation only)
	CreateMerchantAPIKey(ctx context.Context, merchantID, label string) (keyID string, apiKey string, err error)
	RevokeMerchantAPIKey(ctx context.Context, keyID string) error
	LookupMerchantIDByAPIKey(ctx context.Context, apiKey string) (merchantID string, ok bool, err error)

	// Invoices (idempotent by merchant_id + external_order_id)
	CreateInvoice(ctx context.Context, req InvoiceCreate) (domain.Invoice, bool, error)
	GetInvoice(ctx context.Context, invoiceID string) (domain.Invoice, bool, error)
	FindInvoiceByExternalOrderID(ctx context.Context, merchantID, externalOrderID string) (domain.Invoice, bool, error)

	// Invoice checkout token (stored encrypted-at-rest in production DB).
	PutInvoiceToken(ctx context.Context, invoiceID string, token string) error
	GetInvoiceToken(ctx context.Context, invoiceID string) (token string, ok bool, err error)
}

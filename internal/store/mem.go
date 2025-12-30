package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
)

type MemStore struct {
	mu sync.Mutex

	merchantSeq int64
	invoiceSeq  int64

	merchants      map[string]domain.Merchant
	merchantWallet map[string]MerchantWallet

	invoices          map[string]domain.Invoice
	invoiceByExternal map[string]map[string]string // merchant_id -> external_order_id -> invoice_id
	invoiceByAddress  map[string]string            // wallet_id + "|" + address -> invoice_id
}

func NewMem() *MemStore {
	return &MemStore{
		merchants:        make(map[string]domain.Merchant),
		merchantWallet:   make(map[string]MerchantWallet),
		invoices:         make(map[string]domain.Invoice),
		invoiceByExternal: make(map[string]map[string]string),
		invoiceByAddress: make(map[string]string),
	}
}

func (s *MemStore) CreateMerchant(_ context.Context, name string, settings domain.MerchantSettings) (domain.Merchant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Merchant{}, domain.NewError(domain.ErrInvalidArgument, "name is required")
	}
	if err := settings.Validate(); err != nil {
		return domain.Merchant{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.merchantSeq++
	now := time.Now().UTC()
	id := fmt.Sprintf("m_%016x", s.merchantSeq)
	m := domain.Merchant{
		MerchantID: id,
		Name:       name,
		Status:     domain.MerchantActive,
		Settings:   settings,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.merchants[id] = m
	return m, nil
}

func (s *MemStore) GetMerchant(_ context.Context, merchantID string) (domain.Merchant, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.merchants[merchantID]
	return m, ok, nil
}

func (s *MemStore) ListMerchants(_ context.Context) ([]domain.Merchant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Merchant, 0, len(s.merchants))
	for _, m := range s.merchants {
		out = append(out, m)
	}
	return out, nil
}

func (s *MemStore) UpdateMerchantSettings(_ context.Context, merchantID string, settings domain.MerchantSettings) (domain.Merchant, error) {
	if err := settings.Validate(); err != nil {
		return domain.Merchant{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.merchants[merchantID]
	if !ok {
		return domain.Merchant{}, ErrNotFound
	}
	m.Settings = settings
	m.UpdatedAt = time.Now().UTC()
	s.merchants[merchantID] = m
	return m, nil
}

func (s *MemStore) SetMerchantWallet(_ context.Context, merchantID string, w MerchantWallet) (MerchantWallet, error) {
	w.MerchantID = merchantID
	w.WalletID = strings.TrimSpace(w.WalletID)
	w.UFVK = strings.TrimSpace(w.UFVK)
	w.Chain = strings.TrimSpace(w.Chain)
	w.UAHRP = strings.TrimSpace(w.UAHRP)

	if w.WalletID == "" {
		return MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}
	if w.UFVK == "" {
		return MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "ufvk is required")
	}
	if w.UAHRP == "" {
		return MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "ua_hrp is required")
	}
	if w.Chain == "" {
		return MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "chain is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.merchants[merchantID]; !ok {
		return MerchantWallet{}, ErrNotFound
	}
	if _, ok := s.merchantWallet[merchantID]; ok {
		return MerchantWallet{}, ErrConflict
	}

	w.CreatedAt = time.Now().UTC()
	s.merchantWallet[merchantID] = w
	return w, nil
}

func (s *MemStore) GetMerchantWallet(_ context.Context, merchantID string) (MerchantWallet, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.merchantWallet[merchantID]
	return w, ok, nil
}

func (s *MemStore) CreateInvoice(_ context.Context, req InvoiceCreate) (domain.Invoice, bool, error) {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.ExternalOrderID = strings.TrimSpace(req.ExternalOrderID)
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.Address = strings.ToLower(strings.TrimSpace(req.Address))
	if req.MerchantID == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if req.ExternalOrderID == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "external_order_id is required")
	}
	if req.AmountZat <= 0 {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "amount_zat must be > 0")
	}
	if req.Address == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "address is required")
	}
	if req.WalletID == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.merchants[req.MerchantID]; !ok {
		return domain.Invoice{}, false, ErrNotFound
	}

	extMap := s.invoiceByExternal[req.MerchantID]
	if extMap == nil {
		extMap = make(map[string]string)
		s.invoiceByExternal[req.MerchantID] = extMap
	}
	if id, ok := extMap[req.ExternalOrderID]; ok {
		existing := s.invoices[id]
		if existing.AmountZat != req.AmountZat {
			return domain.Invoice{}, false, ErrConflict
		}
		return existing, false, nil
	}

	addrKey := req.WalletID + "|" + req.Address
	if _, ok := s.invoiceByAddress[addrKey]; ok {
		return domain.Invoice{}, false, ErrConflict
	}

	s.invoiceSeq++
	now := time.Now().UTC()
	id := fmt.Sprintf("inv_%016x", s.invoiceSeq)

	inv := domain.Invoice{
		InvoiceID:         id,
		MerchantID:        req.MerchantID,
		ExternalOrderID:   req.ExternalOrderID,
		WalletID:          req.WalletID,
		AddressIndex:      req.AddressIndex,
		Address:           req.Address,
		CreatedAfterHeight: req.CreatedAfterHeight,
		CreatedAfterHash:   req.CreatedAfterHash,
		AmountZat:          req.AmountZat,
		RequiredConfirmations: req.RequiredConfirmations,
		Policies:              req.Policies,
		Status:                domain.InvoiceOpen,
		ExpiresAt:              req.ExpiresAt,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	s.invoices[id] = inv
	extMap[req.ExternalOrderID] = id
	s.invoiceByAddress[addrKey] = id

	return inv, true, nil
}

func (s *MemStore) GetInvoice(_ context.Context, invoiceID string) (domain.Invoice, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.invoices[invoiceID]
	return inv, ok, nil
}

func (s *MemStore) FindInvoiceByExternalOrderID(_ context.Context, merchantID, externalOrderID string) (domain.Invoice, bool, error) {
	merchantID = strings.TrimSpace(merchantID)
	externalOrderID = strings.TrimSpace(externalOrderID)
	s.mu.Lock()
	defer s.mu.Unlock()
	extMap := s.invoiceByExternal[merchantID]
	if extMap == nil {
		return domain.Invoice{}, false, nil
	}
	id, ok := extMap[externalOrderID]
	if !ok {
		return domain.Invoice{}, false, nil
	}
	inv, ok := s.invoices[id]
	return inv, ok, nil
}


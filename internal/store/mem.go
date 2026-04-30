package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/domain"
	"github.com/JunoPayServer/juno-sdk-go/types"
)

type MemStore struct {
	mu sync.Mutex

	merchantSeq int64
	invoiceSeq  int64
	depositSeq  int64
	refundSeq   int64
	reviewSeq   int64
	sinkSeq     int64
	outboxSeq   int64
	deliverySeq int64

	merchants        map[string]domain.Merchant
	merchantWallet   map[string]MerchantWallet
	nextAddressIndex map[string]uint32 // merchant_id -> next address index

	invoices          map[string]domain.Invoice
	invoiceByExternal map[string]map[string]string // merchant_id -> external_order_id -> invoice_id
	invoiceByAddress  map[string]string            // wallet_id + "|" + address -> invoice_id

	invoiceToken map[string]string // invoice_id -> token (plaintext; tests/dev only)

	apiKeySeq     int64
	apiKeysByID   map[string]apiKeyRecord
	apiKeysByHash map[string]string // sha256(token) hex -> key_id

	scanCursor      map[string]int64     // wallet_id -> last applied cursor
	scanLastEventAt map[string]time.Time // wallet_id -> last applied event time
	deposits        map[string]depositRecord

	invoiceEventSeq int64
	invoiceEvents   map[string][]domain.InvoiceEvent // invoice_id -> events

	refunds []refundRecord // ordered by seq

	reviewCases map[string]reviewRecord // review_id -> record
	reviewOrder []string                // insertion order

	eventSinks map[string]domain.EventSink     // sink_id -> sink
	outbox     []outboxEventRecord             // ordered by seq
	deliveries map[string]domain.EventDelivery // delivery_id -> delivery
}

type apiKeyRecord struct {
	KeyID      string
	MerchantID string
	Label      string
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type depositRecord struct {
	Seq              int64
	WalletID         string
	TxID             string
	ActionIndex      int32
	RecipientAddress string
	AmountZat        int64
	Height           int64
	Status           string
	ConfirmedHeight  *int64
	InvoiceID        string
	DetectedAt       time.Time
	UpdatedAt        time.Time
}

type outboxEventRecord struct {
	Seq        int64
	MerchantID string
	Event      domain.CloudEvent
	CreatedAt  time.Time
}

type refundRecord struct {
	Seq    int64
	Refund domain.Refund
}

type reviewRecord struct {
	Case domain.ReviewCase

	DepositWalletID    string
	DepositTxID        string
	DepositActionIndex int32
}

func NewMem() *MemStore {
	return &MemStore{
		merchants:         make(map[string]domain.Merchant),
		merchantWallet:    make(map[string]MerchantWallet),
		nextAddressIndex:  make(map[string]uint32),
		invoices:          make(map[string]domain.Invoice),
		invoiceByExternal: make(map[string]map[string]string),
		invoiceByAddress:  make(map[string]string),
		invoiceToken:      make(map[string]string),
		apiKeysByID:       make(map[string]apiKeyRecord),
		apiKeysByHash:     make(map[string]string),
		scanCursor:        make(map[string]int64),
		scanLastEventAt:   make(map[string]time.Time),
		deposits:          make(map[string]depositRecord),
		invoiceEvents:     make(map[string][]domain.InvoiceEvent),
		reviewCases:       make(map[string]reviewRecord),
		eventSinks:        make(map[string]domain.EventSink),
		deliveries:        make(map[string]domain.EventDelivery),
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
	s.nextAddressIndex[merchantID] = 0
	return w, nil
}

func (s *MemStore) GetMerchantWallet(_ context.Context, merchantID string) (MerchantWallet, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.merchantWallet[merchantID]
	return w, ok, nil
}

func (s *MemStore) NextAddressIndex(_ context.Context, merchantID string) (uint32, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return 0, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.merchants[merchantID]; !ok {
		return 0, ErrNotFound
	}
	if _, ok := s.merchantWallet[merchantID]; !ok {
		return 0, ErrNotFound
	}

	idx := s.nextAddressIndex[merchantID]
	s.nextAddressIndex[merchantID] = idx + 1
	return idx, nil
}

func (s *MemStore) CreateMerchantAPIKey(_ context.Context, merchantID, label string) (keyID string, apiKey string, err error) {
	merchantID = strings.TrimSpace(merchantID)
	label = strings.TrimSpace(label)
	if merchantID == "" {
		return "", "", domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.merchants[merchantID]; !ok {
		return "", "", ErrNotFound
	}

	s.apiKeySeq++
	keyID = fmt.Sprintf("key_%016x", s.apiKeySeq)

	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	apiKey = "jps_" + hex.EncodeToString(raw[:])

	sum := sha256.Sum256([]byte(apiKey))
	hashHex := hex.EncodeToString(sum[:])
	s.apiKeysByHash[hashHex] = keyID

	now := time.Now().UTC()
	s.apiKeysByID[keyID] = apiKeyRecord{
		KeyID:      keyID,
		MerchantID: merchantID,
		Label:      label,
		CreatedAt:  now,
	}

	return keyID, apiKey, nil
}

func (s *MemStore) RevokeMerchantAPIKey(_ context.Context, keyID string) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "key_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.apiKeysByID[keyID]
	if !ok {
		return ErrNotFound
	}
	now := time.Now().UTC()
	rec.RevokedAt = &now
	s.apiKeysByID[keyID] = rec
	return nil
}

func (s *MemStore) LookupMerchantIDByAPIKey(_ context.Context, apiKey string) (merchantID string, ok bool, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", false, nil
	}
	sum := sha256.Sum256([]byte(apiKey))
	hashHex := hex.EncodeToString(sum[:])

	s.mu.Lock()
	defer s.mu.Unlock()
	keyID, ok := s.apiKeysByHash[hashHex]
	if !ok {
		return "", false, nil
	}
	rec, ok := s.apiKeysByID[keyID]
	if !ok {
		return "", false, nil
	}
	if rec.RevokedAt != nil {
		return "", false, nil
	}
	return rec.MerchantID, true, nil
}

func (s *MemStore) ListMerchantAPIKeys(_ context.Context, merchantID string) ([]MerchantAPIKey, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return nil, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]MerchantAPIKey, 0)
	for _, rec := range s.apiKeysByID {
		if rec.MerchantID != merchantID {
			continue
		}
		out = append(out, MerchantAPIKey{
			KeyID:      rec.KeyID,
			MerchantID: rec.MerchantID,
			Label:      rec.Label,
			CreatedAt:  rec.CreatedAt,
			RevokedAt:  rec.RevokedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].KeyID > out[j].KeyID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
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
		InvoiceID:             id,
		MerchantID:            req.MerchantID,
		ExternalOrderID:       req.ExternalOrderID,
		WalletID:              req.WalletID,
		AddressIndex:          req.AddressIndex,
		Address:               req.Address,
		CreatedAfterHeight:    req.CreatedAfterHeight,
		CreatedAfterHash:      req.CreatedAfterHash,
		AmountZat:             req.AmountZat,
		RequiredConfirmations: req.RequiredConfirmations,
		Policies:              req.Policies,
		Status:                domain.InvoiceOpen,
		ExpiresAt:             req.ExpiresAt,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	s.invoices[id] = inv
	extMap[req.ExternalOrderID] = id
	s.invoiceByAddress[addrKey] = id

	s.appendInvoiceEventLocked(id, domain.InvoiceEventInvoiceCreated, now, nil, nil)

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

func (s *MemStore) ListInvoices(_ context.Context, f InvoiceFilter) ([]domain.Invoice, int64, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.ExternalOrderID = strings.TrimSpace(f.ExternalOrderID)
	if f.AfterID < 0 {
		f.AfterID = 0
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	type rec struct {
		Seq int64
		Inv domain.Invoice
	}

	recs := make([]rec, 0, len(s.invoices))
	for _, inv := range s.invoices {
		if f.MerchantID != "" && inv.MerchantID != f.MerchantID {
			continue
		}
		if f.Status != "" && inv.Status != f.Status {
			continue
		}
		if f.ExternalOrderID != "" && inv.ExternalOrderID != f.ExternalOrderID {
			continue
		}
		seq := int64(0)
		if parts := strings.SplitN(inv.InvoiceID, "_", 2); len(parts) == 2 {
			if n, err := strconv.ParseInt(parts[1], 16, 64); err == nil && n > 0 {
				seq = n
			}
		}
		if seq <= f.AfterID {
			continue
		}
		recs = append(recs, rec{Seq: seq, Inv: inv})
	}

	sort.Slice(recs, func(i, j int) bool { return recs[i].Seq < recs[j].Seq })

	out := make([]domain.Invoice, 0, f.Limit)
	var nextCursor int64
	for _, r := range recs {
		out = append(out, r.Inv)
		nextCursor = r.Seq
		if len(out) >= f.Limit {
			break
		}
	}
	return out, nextCursor, nil
}

func (s *MemStore) PutInvoiceToken(_ context.Context, invoiceID string, token string) error {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "invoice_id is required")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return domain.NewError(domain.ErrInvalidArgument, "token is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.invoices[invoiceID]; !ok {
		return ErrNotFound
	}
	s.invoiceToken[invoiceID] = token
	return nil
}

func (s *MemStore) GetInvoiceToken(_ context.Context, invoiceID string) (token string, ok bool, err error) {
	invoiceID = strings.TrimSpace(invoiceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok = s.invoiceToken[invoiceID]
	return token, ok, nil
}

func (s *MemStore) ListMerchantWallets(_ context.Context) ([]MerchantWallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]MerchantWallet, 0, len(s.merchantWallet))
	for _, w := range s.merchantWallet {
		out = append(out, w)
	}
	return out, nil
}

func (s *MemStore) ScanCursor(_ context.Context, walletID string) (int64, error) {
	walletID = strings.TrimSpace(walletID)
	if walletID == "" {
		return 0, domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanCursor[walletID], nil
}

func (s *MemStore) ApplyScanEvent(_ context.Context, ev ScanEvent) error {
	ev.WalletID = strings.TrimSpace(ev.WalletID)
	ev.Kind = strings.TrimSpace(ev.Kind)
	if ev.WalletID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}
	if ev.Cursor <= 0 {
		return domain.NewError(domain.ErrInvalidArgument, "cursor must be > 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if cur := s.scanCursor[ev.WalletID]; ev.Cursor <= cur {
		return nil
	}

	switch types.WalletEventKind(ev.Kind) {
	case types.WalletEventKindDepositEvent:
		var p types.DepositEventPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		s.applyDepositLocked(ev, p.WalletID, p.TxID, int32(p.ActionIndex), p.RecipientAddress, p.AmountZatoshis, p.Height, "detected", nil)
	case types.WalletEventKindDepositConfirmed:
		var p types.DepositConfirmedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		// Note: do not mark deposits as confirmed here. Confirmation is per-invoice (required_confirmations)
		// and is handled by UpdateInvoiceConfirmations based on chain tip height.
		s.applyDepositLocked(ev, p.WalletID, p.TxID, int32(p.ActionIndex), p.RecipientAddress, p.AmountZatoshis, p.Height, "detected", nil)
	case types.WalletEventKindDepositUnconfirmed:
		var p types.DepositUnconfirmedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		s.applyDepositLocked(ev, p.WalletID, p.TxID, int32(p.ActionIndex), p.RecipientAddress, p.AmountZatoshis, p.Height, "unconfirmed", nil)
	case types.WalletEventKindDepositOrphaned:
		var p types.DepositOrphanedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		s.applyDepositLocked(ev, p.WalletID, p.TxID, int32(p.ActionIndex), p.RecipientAddress, p.AmountZatoshis, p.Height, "orphaned", nil)
	default:
		// Ignore other event kinds.
	}

	s.scanCursor[ev.WalletID] = ev.Cursor
	t := ev.OccurredAt.UTC()
	if ev.OccurredAt.IsZero() {
		t = time.Now().UTC()
	}
	s.scanLastEventAt[ev.WalletID] = t
	return nil
}

func (s *MemStore) UpdateInvoiceConfirmations(_ context.Context, bestHeight int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()

	affectedInvoices := make(map[string]struct{})
	for k, d := range s.deposits {
		if d.InvoiceID == "" || d.Status != "detected" {
			continue
		}

		inv, ok := s.invoices[d.InvoiceID]
		if !ok {
			continue
		}

		required := int64(inv.RequiredConfirmations)
		if required <= 0 {
			required = 1
		}

		confs := bestHeight - d.Height + 1
		if confs < required {
			continue
		}

		ch := d.Height + required - 1
		d.Status = "confirmed"
		d.ConfirmedHeight = &ch
		d.UpdatedAt = now
		s.deposits[k] = d

		depRef := &domain.DepositRef{
			WalletID:    d.WalletID,
			TxID:        d.TxID,
			ActionIndex: d.ActionIndex,
			AmountZat:   d.AmountZat,
			Height:      d.Height,
		}
		s.appendInvoiceEventLocked(d.InvoiceID, domain.InvoiceEventDepositConfirmed, now, depRef, nil)
		affectedInvoices[d.InvoiceID] = struct{}{}
	}

	for invoiceID := range affectedInvoices {
		s.recomputeInvoiceLocked(invoiceID)
	}
	return nil
}

func (s *MemStore) ScannerStatus(_ context.Context) (lastCursor int64, lastEventAt *time.Time, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cur := range s.scanCursor {
		if cur > lastCursor {
			lastCursor = cur
		}
	}

	var max time.Time
	for _, t := range s.scanLastEventAt {
		if t.IsZero() {
			continue
		}
		if max.IsZero() || t.After(max) {
			max = t
		}
	}
	if max.IsZero() {
		return lastCursor, nil, nil
	}
	mt := max.UTC()
	return lastCursor, &mt, nil
}

func (s *MemStore) PendingDeliveries(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int64
	for _, d := range s.deliveries {
		if d.Status == domain.EventDeliveryPending {
			n++
		}
	}
	return n, nil
}

func (s *MemStore) ListInvoiceEvents(_ context.Context, invoiceID string, afterID int64, limit int) ([]domain.InvoiceEvent, int64, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return nil, 0, domain.NewError(domain.ErrInvalidArgument, "invoice_id is required")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	evs := s.invoiceEvents[invoiceID]
	if len(evs) == 0 {
		return nil, 0, nil
	}

	out := make([]domain.InvoiceEvent, 0, limit)
	var nextCursor int64 = 0
	for _, e := range evs {
		idNum, _ := strconv.ParseInt(e.EventID, 10, 64)
		if idNum <= afterID {
			continue
		}
		out = append(out, e)
		nextCursor = idNum
		if len(out) >= limit {
			break
		}
	}
	return out, nextCursor, nil
}

func (s *MemStore) ListDeposits(_ context.Context, f DepositFilter) ([]domain.Deposit, int64, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.InvoiceID = strings.TrimSpace(f.InvoiceID)
	f.TxID = strings.TrimSpace(f.TxID)
	if f.AfterID < 0 {
		f.AfterID = 0
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	walletToMerchant := make(map[string]string, len(s.merchantWallet))
	for _, w := range s.merchantWallet {
		walletToMerchant[w.WalletID] = w.MerchantID
	}

	type rec struct {
		Seq int64
		Dep domain.Deposit
	}
	recs := make([]rec, 0, len(s.deposits))
	for _, d := range s.deposits {
		if f.AfterID > 0 && d.Seq <= f.AfterID {
			continue
		}
		if f.TxID != "" && d.TxID != f.TxID {
			continue
		}
		if f.InvoiceID != "" && d.InvoiceID != f.InvoiceID {
			continue
		}
		if f.MerchantID != "" {
			mid, ok := walletToMerchant[d.WalletID]
			if !ok || mid != f.MerchantID {
				continue
			}
		}

		var invoiceID *string
		if d.InvoiceID != "" {
			v := d.InvoiceID
			invoiceID = &v
		}

		recs = append(recs, rec{
			Seq: d.Seq,
			Dep: domain.Deposit{
				WalletID:         d.WalletID,
				TxID:             d.TxID,
				ActionIndex:      d.ActionIndex,
				RecipientAddress: d.RecipientAddress,
				AmountZat:        d.AmountZat,
				Height:           d.Height,
				Status:           domain.DepositStatus(d.Status),
				ConfirmedHeight:  d.ConfirmedHeight,
				InvoiceID:        invoiceID,
				DetectedAt:       d.DetectedAt,
				UpdatedAt:        d.UpdatedAt,
			},
		})
	}

	sort.Slice(recs, func(i, j int) bool { return recs[i].Seq < recs[j].Seq })

	out := make([]domain.Deposit, 0, f.Limit)
	var nextCursor int64
	for _, r := range recs {
		out = append(out, r.Dep)
		nextCursor = r.Seq
		if len(out) >= f.Limit {
			break
		}
	}
	return out, nextCursor, nil
}

func (s *MemStore) CreateRefund(_ context.Context, req RefundCreate) (domain.Refund, error) {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.InvoiceID = strings.TrimSpace(req.InvoiceID)
	req.ExternalRefundID = strings.TrimSpace(req.ExternalRefundID)
	req.ToAddress = strings.TrimSpace(req.ToAddress)
	req.SentTxID = strings.TrimSpace(req.SentTxID)
	req.Notes = strings.TrimSpace(req.Notes)
	if req.MerchantID == "" {
		return domain.Refund{}, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if req.ToAddress == "" {
		return domain.Refund{}, domain.NewError(domain.ErrInvalidArgument, "to_address is required")
	}
	if req.AmountZat <= 0 {
		return domain.Refund{}, domain.NewError(domain.ErrInvalidArgument, "amount_zat must be > 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.merchants[req.MerchantID]; !ok {
		return domain.Refund{}, ErrNotFound
	}

	if req.InvoiceID != "" {
		inv, ok := s.invoices[req.InvoiceID]
		if !ok {
			return domain.Refund{}, ErrNotFound
		}
		if inv.MerchantID != req.MerchantID {
			return domain.Refund{}, ErrForbidden
		}
	}

	s.refundSeq++
	now := time.Now().UTC()
	refundID := fmt.Sprintf("refund_%016x", s.refundSeq)

	var invoiceID *string
	if req.InvoiceID != "" {
		v := req.InvoiceID
		invoiceID = &v
	}
	var externalRefundID *string
	if req.ExternalRefundID != "" {
		v := req.ExternalRefundID
		externalRefundID = &v
	}
	var sentTxID *string
	if req.SentTxID != "" {
		v := req.SentTxID
		sentTxID = &v
	}
	status := domain.RefundRequested
	if sentTxID != nil {
		status = domain.RefundSent
	}

	refund := domain.Refund{
		RefundID:         refundID,
		MerchantID:       req.MerchantID,
		InvoiceID:        invoiceID,
		ExternalRefundID: externalRefundID,
		ToAddress:        req.ToAddress,
		AmountZat:        req.AmountZat,
		Status:           status,
		SentTxID:         sentTxID,
		Notes:            req.Notes,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.refunds = append(s.refunds, refundRecord{
		Seq:    s.refundSeq,
		Refund: refund,
	})

	if invoiceID != nil {
		typ := domain.InvoiceEventRefundRequested
		if status == domain.RefundSent {
			typ = domain.InvoiceEventRefundSent
		}
		s.appendInvoiceEventLocked(*invoiceID, typ, now, nil, &refund)
	}

	return refund, nil
}

func (s *MemStore) ListRefunds(_ context.Context, f RefundFilter) ([]domain.Refund, int64, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.InvoiceID = strings.TrimSpace(f.InvoiceID)
	if f.AfterID < 0 {
		f.AfterID = 0
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.Refund, 0, f.Limit)
	var nextCursor int64
	for _, r := range s.refunds {
		if r.Seq <= f.AfterID {
			continue
		}
		refund := r.Refund
		if f.MerchantID != "" && refund.MerchantID != f.MerchantID {
			continue
		}
		if f.InvoiceID != "" {
			if refund.InvoiceID == nil || *refund.InvoiceID != f.InvoiceID {
				continue
			}
		}
		if f.Status != "" && refund.Status != f.Status {
			continue
		}
		out = append(out, refund)
		nextCursor = r.Seq
		if len(out) >= f.Limit {
			break
		}
	}
	return out, nextCursor, nil
}

func (s *MemStore) ListReviewCases(_ context.Context, f ReviewCaseFilter) ([]domain.ReviewCase, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.ReviewCase, 0, len(s.reviewCases))
	for _, id := range s.reviewOrder {
		r, ok := s.reviewCases[id]
		if !ok {
			continue
		}
		c := r.Case
		if f.MerchantID != "" && c.MerchantID != f.MerchantID {
			continue
		}
		if f.Status != "" && c.Status != f.Status {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (s *MemStore) ResolveReviewCase(_ context.Context, reviewID string, notes string) error {
	reviewID = strings.TrimSpace(reviewID)
	notes = strings.TrimSpace(notes)
	if reviewID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "review_id is required")
	}
	if notes == "" {
		return domain.NewError(domain.ErrInvalidArgument, "notes is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reviewCases[reviewID]
	if !ok {
		return ErrNotFound
	}

	now := time.Now().UTC()
	r.Case.Status = domain.ReviewResolved
	r.Case.Notes = notes
	r.Case.UpdatedAt = now
	s.reviewCases[reviewID] = r
	return nil
}

func (s *MemStore) RejectReviewCase(_ context.Context, reviewID string, notes string) error {
	reviewID = strings.TrimSpace(reviewID)
	notes = strings.TrimSpace(notes)
	if reviewID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "review_id is required")
	}
	if notes == "" {
		return domain.NewError(domain.ErrInvalidArgument, "notes is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.reviewCases[reviewID]
	if !ok {
		return ErrNotFound
	}

	now := time.Now().UTC()
	r.Case.Status = domain.ReviewRejected
	r.Case.Notes = notes
	r.Case.UpdatedAt = now
	s.reviewCases[reviewID] = r
	return nil
}

func (s *MemStore) createReviewCaseLocked(merchantID string, invoiceID *string, reason domain.ReviewReason, notes string, depWalletID, depTxID string, depActionIndex int32) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return
	}

	for _, r := range s.reviewCases {
		if r.Case.Status != domain.ReviewOpen || r.Case.MerchantID != merchantID || r.Case.Reason != reason {
			continue
		}
		if invoiceID != nil && r.Case.InvoiceID != nil && *r.Case.InvoiceID == *invoiceID {
			return
		}
		if invoiceID == nil && depWalletID != "" && depTxID != "" {
			if r.DepositWalletID == depWalletID && r.DepositTxID == depTxID && r.DepositActionIndex == depActionIndex {
				return
			}
		}
	}

	s.reviewSeq++
	now := time.Now().UTC()
	id := fmt.Sprintf("rev_%016x", s.reviewSeq)
	c := domain.ReviewCase{
		ReviewID:   id,
		MerchantID: merchantID,
		InvoiceID:  invoiceID,
		Reason:     reason,
		Status:     domain.ReviewOpen,
		Notes:      strings.TrimSpace(notes),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.reviewCases[id] = reviewRecord{
		Case:               c,
		DepositWalletID:    depWalletID,
		DepositTxID:        depTxID,
		DepositActionIndex: depActionIndex,
	}
	s.reviewOrder = append(s.reviewOrder, id)
}

func (s *MemStore) applyDepositLocked(ev ScanEvent, walletID, txid string, actionIndex int32, recipientAddress string, amountZatoshis uint64, height int64, status string, confirmedHeight *int64) {
	if walletID == "" {
		walletID = ev.WalletID
	}
	if recipientAddress == "" {
		return
	}
	now := time.Now().UTC()

	maxInt64 := uint64(^uint64(0) >> 1)
	if amountZatoshis > maxInt64 {
		return
	}
	amountZat := int64(amountZatoshis)

	addr := strings.ToLower(strings.TrimSpace(recipientAddress))
	addrKey := walletID + "|" + addr
	invoiceID := ""
	if invID, ok := s.invoiceByAddress[addrKey]; ok {
		inv := s.invoices[invID]
		if inv.CanApplyDeposit(height) {
			invoiceID = invID
		}
	}

	depKey := walletID + "|" + txid + "|" + strconv.FormatInt(int64(actionIndex), 10)
	rec, ok := s.deposits[depKey]
	if !ok {
		s.depositSeq++
		detectedAt := ev.OccurredAt.UTC()
		if detectedAt.IsZero() {
			detectedAt = now
		}
		if status == "unconfirmed" || status == "orphaned" {
			confirmedHeight = nil
		}
		rec = depositRecord{
			Seq:              s.depositSeq,
			WalletID:         walletID,
			TxID:             txid,
			ActionIndex:      actionIndex,
			RecipientAddress: addr,
			AmountZat:        amountZat,
			Height:           height,
			Status:           status,
			ConfirmedHeight:  confirmedHeight,
			InvoiceID:        invoiceID,
			DetectedAt:       detectedAt,
			UpdatedAt:        now,
		}
	} else {
		rec.RecipientAddress = addr
		rec.AmountZat = amountZat
		rec.Height = height

		incomingStatus := status
		nextStatus := incomingStatus
		if rec.Status == "confirmed" && incomingStatus == "detected" {
			nextStatus = rec.Status
		}
		rec.Status = nextStatus

		switch incomingStatus {
		case "confirmed":
			rec.ConfirmedHeight = confirmedHeight
		case "unconfirmed", "orphaned":
			rec.ConfirmedHeight = nil
		case "detected":
			if nextStatus != "confirmed" {
				rec.ConfirmedHeight = nil
			}
		}
		rec.UpdatedAt = now
		if rec.InvoiceID == "" && invoiceID != "" {
			rec.InvoiceID = invoiceID
		}
	}
	s.deposits[depKey] = rec

	if rec.InvoiceID == "" {
		// Unknown address deposit (unattributed).
		merchantID := ""
		for _, w := range s.merchantWallet {
			if w.WalletID == walletID {
				merchantID = w.MerchantID
				break
			}
		}
		if merchantID != "" {
			notes := fmt.Sprintf("wallet_id=%s txid=%s action_index=%d recipient_address=%s amount_zat=%d height=%d",
				walletID, txid, actionIndex, addr, amountZat, height,
			)
			s.createReviewCaseLocked(merchantID, nil, domain.ReviewUnknownAddress, notes, walletID, txid, actionIndex)
		}
		return
	}

	depRef := &domain.DepositRef{
		WalletID:    walletID,
		TxID:        txid,
		ActionIndex: actionIndex,
		AmountZat:   amountZat,
		Height:      height,
	}

	switch status {
	case "detected":
		s.appendInvoiceEventLocked(rec.InvoiceID, domain.InvoiceEventDepositDetected, ev.OccurredAt.UTC(), depRef, nil)
	case "confirmed":
		s.appendInvoiceEventLocked(rec.InvoiceID, domain.InvoiceEventDepositConfirmed, ev.OccurredAt.UTC(), depRef, nil)
	}

	s.recomputeInvoiceLocked(rec.InvoiceID)
}

func (s *MemStore) recomputeInvoiceLocked(invoiceID string) {
	inv, ok := s.invoices[invoiceID]
	if !ok {
		return
	}
	var pending int64
	var confirmed int64
	for _, d := range s.deposits {
		if d.InvoiceID != invoiceID {
			continue
		}
		switch d.Status {
		case "confirmed":
			confirmed += d.AmountZat
		case "detected", "unconfirmed":
			pending += d.AmountZat
		}
	}

	inv.ReceivedPendingZat = pending
	inv.ReceivedConfirmedZat = confirmed

	prevStatus := inv.Status
	now := time.Now().UTC()
	inv.Status = computeInvoiceStatus(inv, now)
	inv.UpdatedAt = now
	s.invoices[invoiceID] = inv

	if inv.Status != prevStatus {
		switch inv.Status {
		case domain.InvoiceExpired:
			s.appendInvoiceEventLocked(invoiceID, domain.InvoiceEventInvoiceExpired, now, nil, nil)
		case domain.InvoiceConfirmed, domain.InvoicePaidLate:
			s.appendInvoiceEventLocked(invoiceID, domain.InvoiceEventInvoicePaid, now, nil, nil)
		case domain.InvoiceOverpaid:
			s.appendInvoiceEventLocked(invoiceID, domain.InvoiceEventInvoiceOverpaid, now, nil, nil)
		}

		invID := invoiceID
		switch {
		case inv.Status == domain.InvoicePartialConfirmed && inv.Policies.PartialPayment == domain.PartialPaymentReject:
			s.createReviewCaseLocked(inv.MerchantID, &invID, domain.ReviewPartialPayment, "partial payment requires review", "", "", 0)
		case inv.Status == domain.InvoiceOverpaid && inv.Policies.Overpayment == domain.OverpaymentManualReview:
			s.createReviewCaseLocked(inv.MerchantID, &invID, domain.ReviewOverpayment, "overpayment requires review", "", "", 0)
		case inv.Status == domain.InvoiceConfirmed || inv.Status == domain.InvoicePaidLate:
			expired := inv.ExpiresAt != nil && now.After(inv.ExpiresAt.UTC())
			if expired && inv.ReceivedConfirmedZat == inv.AmountZat && inv.Policies.LatePayment == domain.LatePaymentManualReview {
				s.createReviewCaseLocked(inv.MerchantID, &invID, domain.ReviewLatePayment, "late payment requires review", "", "", 0)
			}
		}
	}
}

func computeInvoiceStatus(inv domain.Invoice, now time.Time) domain.InvoiceStatus {
	expired := inv.ExpiresAt != nil && now.After(inv.ExpiresAt.UTC())
	total := inv.ReceivedPendingZat + inv.ReceivedConfirmedZat

	switch {
	case total == 0 && expired:
		return domain.InvoiceExpired
	case total == 0:
		return domain.InvoiceOpen
	case inv.ReceivedConfirmedZat > inv.AmountZat:
		return domain.InvoiceOverpaid
	case inv.ReceivedConfirmedZat == inv.AmountZat:
		if expired && inv.Policies.LatePayment == domain.LatePaymentMarkPaidLate {
			return domain.InvoicePaidLate
		}
		return domain.InvoiceConfirmed
	case total >= inv.AmountZat:
		return domain.InvoicePending
	case inv.ReceivedConfirmedZat > 0:
		return domain.InvoicePartialConfirmed
	default:
		return domain.InvoicePartialPending
	}
}

func (s *MemStore) appendInvoiceEventLocked(invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef, refund *domain.Refund) {
	if invoiceID == "" {
		return
	}

	// Ensure idempotency for deposit/refund events by checking existing refs.
	switch {
	case dep != nil:
		for _, e := range s.invoiceEvents[invoiceID] {
			if e.Type != typ || e.Deposit == nil {
				continue
			}
			if e.Deposit.WalletID == dep.WalletID &&
				e.Deposit.TxID == dep.TxID &&
				e.Deposit.ActionIndex == dep.ActionIndex {
				return
			}
		}
	case refund != nil:
		for _, e := range s.invoiceEvents[invoiceID] {
			if e.Type != typ || e.Refund == nil {
				continue
			}
			if e.Refund.RefundID == refund.RefundID {
				return
			}
		}
	default:
		for _, e := range s.invoiceEvents[invoiceID] {
			if e.Type == typ && e.Deposit == nil && e.Refund == nil {
				return
			}
		}
	}

	s.invoiceEventSeq++
	idStr := strconv.FormatInt(s.invoiceEventSeq, 10)

	var refundCopy *domain.Refund
	if refund != nil {
		c := *refund
		refundCopy = &c
	}
	s.invoiceEvents[invoiceID] = append(s.invoiceEvents[invoiceID], domain.InvoiceEvent{
		EventID:    idStr,
		Type:       typ,
		OccurredAt: occurredAt,
		InvoiceID:  invoiceID,
		Deposit:    dep,
		Refund:     refundCopy,
	})

	s.enqueueOutboxLocked(invoiceID, typ, occurredAt, dep, refundCopy)
}

func (s *MemStore) enqueueOutboxLocked(invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef, refund *domain.Refund) {
	inv, ok := s.invoices[invoiceID]
	if !ok {
		return
	}

	data := map[string]any{
		"merchant_id":       inv.MerchantID,
		"invoice_id":        invoiceID,
		"external_order_id": inv.ExternalOrderID,
	}
	if dep != nil {
		data["deposit"] = map[string]any{
			"wallet_id":    dep.WalletID,
			"txid":         dep.TxID,
			"action_index": dep.ActionIndex,
			"amount_zat":   dep.AmountZat,
			"height":       dep.Height,
		}
	}
	if refund != nil {
		data["refund"] = map[string]any{
			"refund_id":  refund.RefundID,
			"to_address": refund.ToAddress,
			"amount_zat": refund.AmountZat,
			"status":     string(refund.Status),
			"sent_txid":  refund.SentTxID,
			"notes":      refund.Notes,
		}
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return
	}

	s.outboxSeq++
	eventID := fmt.Sprintf("evt_%016x", s.outboxSeq)
	ce := domain.CloudEvent{
		SpecVersion:     "1.0",
		ID:              eventID,
		Source:          "juno-pay-server",
		Type:            string(typ),
		Subject:         "invoice/" + invoiceID,
		Time:            occurredAt.UTC(),
		DataContentType: "application/json",
		Data:            dataBytes,
	}
	s.outbox = append(s.outbox, outboxEventRecord{
		Seq:        s.outboxSeq,
		MerchantID: inv.MerchantID,
		Event:      ce,
		CreatedAt:  time.Now().UTC(),
	})

	for _, sink := range s.eventSinks {
		if sink.MerchantID != inv.MerchantID || sink.Status != domain.EventSinkActive {
			continue
		}
		s.deliverySeq++
		deliveryID := fmt.Sprintf("del_%016x", s.deliverySeq)
		now := time.Now().UTC()
		s.deliveries[deliveryID] = domain.EventDelivery{
			DeliveryID: deliveryID,
			SinkID:     sink.SinkID,
			EventID:    eventID,
			Status:     domain.EventDeliveryPending,
			Attempt:    0,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
}

func (s *MemStore) CreateEventSink(_ context.Context, req EventSinkCreate) (domain.EventSink, error) {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	if req.MerchantID == "" {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if len(req.Config) == 0 {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "config is required")
	}

	switch req.Kind {
	case domain.EventSinkWebhook, domain.EventSinkKafka, domain.EventSinkNATS, domain.EventSinkRabbitMQ:
	default:
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "kind invalid")
	}

	var cfgAny any
	if err := json.Unmarshal(req.Config, &cfgAny); err != nil {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "config invalid json")
	}
	if _, ok := cfgAny.(map[string]any); !ok {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "config must be an object")
	}
	cfgBytes, _ := json.Marshal(cfgAny)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.merchants[req.MerchantID]; !ok {
		return domain.EventSink{}, ErrNotFound
	}

	s.sinkSeq++
	now := time.Now().UTC()
	sinkID := fmt.Sprintf("sink_%016x", s.sinkSeq)
	sink := domain.EventSink{
		SinkID:     sinkID,
		MerchantID: req.MerchantID,
		Kind:       req.Kind,
		Status:     domain.EventSinkActive,
		Config:     cfgBytes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.eventSinks[sinkID] = sink
	return sink, nil
}

func (s *MemStore) GetEventSink(_ context.Context, sinkID string) (domain.EventSink, bool, error) {
	sinkID = strings.TrimSpace(sinkID)
	if sinkID == "" {
		return domain.EventSink{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sink, ok := s.eventSinks[sinkID]
	return sink, ok, nil
}

func (s *MemStore) ListEventSinks(_ context.Context, merchantID string) ([]domain.EventSink, error) {
	merchantID = strings.TrimSpace(merchantID)
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.EventSink, 0, len(s.eventSinks))
	for _, s0 := range s.eventSinks {
		if merchantID != "" && s0.MerchantID != merchantID {
			continue
		}
		out = append(out, s0)
	}
	return out, nil
}

func (s *MemStore) ListOutboundEvents(_ context.Context, merchantID string, afterID int64, limit int) ([]domain.CloudEvent, int64, error) {
	merchantID = strings.TrimSpace(merchantID)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.CloudEvent, 0, limit)
	var next int64
	for _, r := range s.outbox {
		if r.Seq <= afterID {
			continue
		}
		if merchantID != "" && r.MerchantID != merchantID {
			continue
		}
		out = append(out, r.Event)
		next = r.Seq
		if len(out) >= limit {
			break
		}
	}
	return out, next, nil
}

func (s *MemStore) ListEventDeliveries(_ context.Context, f EventDeliveryFilter) ([]domain.EventDelivery, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.SinkID = strings.TrimSpace(f.SinkID)
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]domain.EventDelivery, 0, f.Limit)
	for _, d := range s.deliveries {
		if f.SinkID != "" && d.SinkID != f.SinkID {
			continue
		}
		if f.Status != "" && d.Status != f.Status {
			continue
		}
		if f.MerchantID != "" {
			sink, ok := s.eventSinks[d.SinkID]
			if !ok || sink.MerchantID != f.MerchantID {
				continue
			}
		}
		out = append(out, d)
		if len(out) >= f.Limit {
			break
		}
	}
	return out, nil
}

func (s *MemStore) ListDueDeliveries(_ context.Context, now time.Time, limit int) ([]DueDelivery, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	findEvent := func(eventID string) (domain.CloudEvent, bool) {
		for i := len(s.outbox) - 1; i >= 0; i-- {
			if s.outbox[i].Event.ID == eventID {
				return s.outbox[i].Event, true
			}
		}
		return domain.CloudEvent{}, false
	}

	out := make([]DueDelivery, 0, limit)
	for _, d := range s.deliveries {
		if d.Status != domain.EventDeliveryPending {
			continue
		}
		if d.NextRetryAt != nil && d.NextRetryAt.After(now) {
			continue
		}
		sink, ok := s.eventSinks[d.SinkID]
		if !ok || sink.Status != domain.EventSinkActive {
			continue
		}
		ev, ok := findEvent(d.EventID)
		if !ok {
			continue
		}
		out = append(out, DueDelivery{
			Delivery: d,
			Sink:     sink,
			Event:    ev,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *MemStore) UpdateEventDelivery(_ context.Context, deliveryID string, status domain.EventDeliveryStatus, attempt int32, nextRetryAt *time.Time, lastError *string) error {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "delivery_id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.deliveries[deliveryID]
	if !ok {
		return ErrNotFound
	}
	d.Status = status
	d.Attempt = attempt
	d.NextRetryAt = nextRetryAt
	d.LastError = lastError
	d.UpdatedAt = time.Now().UTC()
	s.deliveries[deliveryID] = d
	return nil
}

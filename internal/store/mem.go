package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-sdk-go/types"
)

type MemStore struct {
	mu sync.Mutex

	merchantSeq int64
	invoiceSeq  int64
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

	scanCursor map[string]int64 // wallet_id -> last applied cursor
	deposits   map[string]depositRecord

	invoiceEventSeq int64
	invoiceEvents   map[string][]domain.InvoiceEvent // invoice_id -> events

	eventSinks  map[string]domain.EventSink         // sink_id -> sink
	outbox      []outboxEventRecord                 // ordered by seq
	deliveries  map[string]domain.EventDelivery     // delivery_id -> delivery
}

type apiKeyRecord struct {
	KeyID      string
	MerchantID string
	Label      string
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

type depositRecord struct {
	WalletID         string
	TxID             string
	ActionIndex      int32
	RecipientAddress string
	AmountZat        int64
	Height           int64
	Status           string
	ConfirmedHeight  *int64
	InvoiceID        string
	UpdatedAt        time.Time
}

type outboxEventRecord struct {
	Seq        int64
	MerchantID string
	Event      domain.CloudEvent
	CreatedAt  time.Time
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
		deposits:          make(map[string]depositRecord),
		invoiceEvents:     make(map[string][]domain.InvoiceEvent),
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

	s.appendInvoiceEventLocked(id, domain.InvoiceEventInvoiceCreated, now, nil)

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
		ch := p.ConfirmedHeight
		s.applyDepositLocked(ev, p.WalletID, p.TxID, int32(p.ActionIndex), p.RecipientAddress, p.AmountZatoshis, p.Height, "confirmed", &ch)
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
	return nil
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

func (s *MemStore) applyDepositLocked(ev ScanEvent, walletID, txid string, actionIndex int32, recipientAddress string, amountZatoshis uint64, height int64, status string, confirmedHeight *int64) {
	if walletID == "" {
		walletID = ev.WalletID
	}
	if recipientAddress == "" {
		return
	}

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
		rec = depositRecord{
			WalletID:         walletID,
			TxID:             txid,
			ActionIndex:      actionIndex,
			RecipientAddress: addr,
			AmountZat:        amountZat,
			Height:           height,
			Status:           status,
			ConfirmedHeight:  confirmedHeight,
			InvoiceID:        invoiceID,
			UpdatedAt:        time.Now().UTC(),
		}
	} else {
		rec.Status = status
		rec.ConfirmedHeight = confirmedHeight
		rec.UpdatedAt = time.Now().UTC()
		if rec.InvoiceID == "" && invoiceID != "" {
			rec.InvoiceID = invoiceID
		}
	}
	s.deposits[depKey] = rec

	if rec.InvoiceID == "" {
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
		s.appendInvoiceEventLocked(rec.InvoiceID, domain.InvoiceEventDepositDetected, ev.OccurredAt.UTC(), depRef)
	case "confirmed":
		s.appendInvoiceEventLocked(rec.InvoiceID, domain.InvoiceEventDepositConfirmed, ev.OccurredAt.UTC(), depRef)
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
			s.appendInvoiceEventLocked(invoiceID, domain.InvoiceEventInvoiceExpired, now, nil)
		case domain.InvoicePaid, domain.InvoicePaidLate:
			s.appendInvoiceEventLocked(invoiceID, domain.InvoiceEventInvoicePaid, now, nil)
		case domain.InvoiceOverpaid:
			s.appendInvoiceEventLocked(invoiceID, domain.InvoiceEventInvoiceOverpaid, now, nil)
		}
	}
}

func computeInvoiceStatus(inv domain.Invoice, now time.Time) domain.InvoiceStatus {
	expired := inv.ExpiresAt != nil && now.After(inv.ExpiresAt.UTC())

	switch {
	case inv.ReceivedConfirmedZat == 0 && expired:
		return domain.InvoiceExpired
	case inv.ReceivedConfirmedZat == 0:
		return domain.InvoiceOpen
	case inv.ReceivedConfirmedZat < inv.AmountZat:
		return domain.InvoicePartial
	case inv.ReceivedConfirmedZat == inv.AmountZat:
		if expired && inv.Policies.LatePayment == domain.LatePaymentMarkPaidLate {
			return domain.InvoicePaidLate
		}
		return domain.InvoicePaid
	default:
		return domain.InvoiceOverpaid
	}
}

func (s *MemStore) appendInvoiceEventLocked(invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef) {
	if invoiceID == "" {
		return
	}

	// Ensure idempotency for deposit events by checking existing refs.
	if dep != nil {
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
	} else {
		for _, e := range s.invoiceEvents[invoiceID] {
			if e.Type == typ && e.Deposit == nil {
				return
			}
		}
	}

	s.invoiceEventSeq++
	idStr := strconv.FormatInt(s.invoiceEventSeq, 10)
	s.invoiceEvents[invoiceID] = append(s.invoiceEvents[invoiceID], domain.InvoiceEvent{
		EventID:    idStr,
		Type:       typ,
		OccurredAt: occurredAt,
		InvoiceID:  invoiceID,
		Deposit:    dep,
	})

	s.enqueueOutboxLocked(invoiceID, typ, occurredAt, dep)
}

func (s *MemStore) enqueueOutboxLocked(invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef) {
	inv, ok := s.invoices[invoiceID]
	if !ok {
		return
	}

	data := map[string]any{
		"merchant_id": inv.MerchantID,
		"invoice_id":  invoiceID,
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

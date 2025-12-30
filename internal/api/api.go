package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

type Deriver interface {
	Derive(ufvk string, index uint32) (string, error)
}

type TipSource interface {
	BestTip(ctx context.Context) (height int64, hash string, err error)
}

type Clock interface {
	Now() time.Time
}

type TokenGenerator interface {
	NewInvoiceToken() (string, error)
}

type Server struct {
	st store.Store

	deriver  Deriver
	tip      TipSource
	clock    Clock
	tokenGen TokenGenerator
}

func New(st store.Store, deriver Deriver, tip TipSource, clock Clock, tokenGen TokenGenerator) (*Server, error) {
	if st == nil {
		return nil, errors.New("api: store is nil")
	}
	if deriver == nil {
		return nil, errors.New("api: deriver is nil")
	}
	if tip == nil {
		return nil, errors.New("api: tip source is nil")
	}
	if clock == nil {
		return nil, errors.New("api: clock is nil")
	}
	if tokenGen == nil {
		return nil, errors.New("api: token generator is nil")
	}
	return &Server{
		st:       st,
		deriver:  deriver,
		tip:      tip,
		clock:    clock,
		tokenGen: tokenGen,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/invoices", s.handleInvoices)
	mux.HandleFunc("/v1/public/invoices/", s.handlePublicInvoices)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"healthy": true,
		},
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// TODO: include scanner sync, backlog, and outbox metrics.
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"ok": true,
		},
	})
}

func (s *Server) handleInvoices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateInvoice(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type invoiceCreateRequest struct {
	ExternalOrderID string         `json:"external_order_id"`
	AmountZat       int64          `json:"amount_zat"`
	Metadata        map[string]any `json:"metadata"`
}

func (s *Server) handleCreateInvoice(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	merchantID, ok, err := s.authMerchant(ctx, r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "auth error")
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req invoiceCreateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	req.ExternalOrderID = strings.TrimSpace(req.ExternalOrderID)
	if req.ExternalOrderID == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "external_order_id is required")
		return
	}
	if req.AmountZat <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_argument", "amount_zat must be > 0")
		return
	}

	m, found, err := s.st.GetMerchant(ctx, merchantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !found {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if err := m.Settings.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "invalid merchant settings")
		return
	}

	wallet, found, err := s.st.GetMerchantWallet(ctx, merchantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !found {
		writeError(w, http.StatusConflict, "wallet_not_set", "merchant wallet is not set")
		return
	}

	addrIndex, err := s.st.NextAddressIndex(ctx, merchantID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusConflict, "wallet_not_set", "merchant wallet is not set")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	addr, err := s.deriver.Derive(wallet.UFVK, addrIndex)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "address derivation failed")
		return
	}

	tipHeight, tipHash, err := s.tip.BestTip(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "tip error")
		return
	}

	var expiresAt *time.Time
	if ttl := m.Settings.InvoiceTTL(); ttl > 0 {
		exp := s.clock.Now().UTC().Add(ttl)
		expiresAt = &exp
	}

	inv, created, err := s.st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            merchantID,
		ExternalOrderID:       req.ExternalOrderID,
		WalletID:              wallet.WalletID,
		AddressIndex:          addrIndex,
		Address:               addr,
		CreatedAfterHeight:    tipHeight,
		CreatedAfterHash:      tipHash,
		AmountZat:             req.AmountZat,
		RequiredConfirmations: m.Settings.RequiredConfirmations,
		Policies:              m.Settings.Policies,
		ExpiresAt:             expiresAt,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "conflict")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "not_found", "not found")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	var token string
	if created {
		token, err = s.tokenGen.NewInvoiceToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "token generation failed")
			return
		}
		if err := s.st.PutInvoiceToken(ctx, inv.InvoiceID, token); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "db error")
			return
		}
	} else {
		var ok bool
		token, ok, err = s.st.GetInvoiceToken(ctx, inv.InvoiceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "db error")
			return
		}
		if !ok {
			writeError(w, http.StatusInternalServerError, "internal", "invoice token missing")
			return
		}
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	writeJSON(w, status, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"invoice":       toInvoiceJSON(inv),
			"invoice_token": token,
		},
	})
}

func (s *Server) handlePublicInvoices(w http.ResponseWriter, r *http.Request) {
	// /v1/public/invoices/{invoice_id}[/(events|stream)]
	path := strings.TrimPrefix(r.URL.Path, "/v1/public/invoices/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	invoiceID := parts[0]
	if invoiceID == "" {
		http.NotFound(w, r)
		return
	}

	if len(parts) == 1 {
		s.handleGetPublicInvoice(w, r, invoiceID)
		return
	}

	switch parts[1] {
	case "events":
		http.Error(w, "not implemented", http.StatusNotImplemented)
	case "stream":
		http.Error(w, "not implemented", http.StatusNotImplemented)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGetPublicInvoice(w http.ResponseWriter, r *http.Request, invoiceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	inv, ok, err := s.st.GetInvoice(ctx, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	wantToken, ok, err := s.st.GetInvoiceToken(ctx, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(wantToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   toInvoiceJSON(inv),
	})
}

func (s *Server) authMerchant(ctx context.Context, r *http.Request) (merchantID string, ok bool, err error) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return "", false, nil
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return "", false, nil
	}
	apiKey := strings.TrimSpace(parts[1])
	if apiKey == "" {
		return "", false, nil
	}
	return s.st.LookupMerchantIDByAPIKey(ctx, apiKey)
}

func toInvoiceJSON(inv domain.Invoice) map[string]any {
	var expiresAt any = nil
	if inv.ExpiresAt != nil {
		expiresAt = inv.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}

	return map[string]any{
		"invoice_id":             inv.InvoiceID,
		"merchant_id":            inv.MerchantID,
		"external_order_id":      inv.ExternalOrderID,
		"status":                 string(inv.Status),
		"address":                inv.Address,
		"amount_zat":             inv.AmountZat,
		"required_confirmations": inv.RequiredConfirmations,
		"received_zat_pending":   inv.ReceivedPendingZat,
		"received_zat_confirmed": inv.ReceivedConfirmedZat,
		"expires_at":             expiresAt,
		"created_at":             inv.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":             inv.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"status": "error",
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

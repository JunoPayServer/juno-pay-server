package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/outbox"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

type Option func(*Server) error

func WithAdminPassword(password string) Option {
	return func(s *Server) error {
		password = strings.TrimSpace(password)
		if password == "" {
			return errors.New("api: admin password is required")
		}
		passHash := sha256.Sum256([]byte(password))
		sessionKey := sha256.Sum256([]byte("juno-pay-server/session:" + password))

		s.adminEnabled = true
		s.adminPassHash = passHash
		s.adminSessionKey = sessionKey
		if s.adminSessionTTL == 0 {
			s.adminSessionTTL = 12 * time.Hour
		}
		return nil
	}
}

type Deriver interface {
	Derive(ufvk string, uaHRP string, index uint32) (string, error)
}

type TipSource interface {
	BestTip(ctx context.Context) (height int64, hash string, err error)
	UptimeSeconds(ctx context.Context) (seconds int64, err error)
}

var ErrUptimeUnsupported = errors.New("api: uptime unsupported")

type Clock interface {
	Now() time.Time
}

type TokenGenerator interface {
	NewInvoiceToken() (string, error)
}

type ScannerHealth interface {
	Healthy(ctx context.Context) (bool, error)
}

type Server struct {
	st store.Store

	deriver  Deriver
	tip      TipSource
	clock    Clock
	tokenGen TokenGenerator

	scanHealth ScannerHealth
	adminUI    http.Handler

	adminEnabled    bool
	adminPassHash   [32]byte
	adminSessionKey [32]byte
	adminSessionTTL time.Duration

	muRestart sync.Mutex

	lastUptimeKnown   bool
	lastUptimeSeconds int64

	junocashdRestartsDetected int64
	lastRestartAt             *time.Time
}

func New(st store.Store, deriver Deriver, tip TipSource, clock Clock, tokenGen TokenGenerator, opts ...Option) (*Server, error) {
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

	s := &Server{
		st:       st,
		deriver:  deriver,
		tip:      tip,
		clock:    clock,
		tokenGen: tokenGen,
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func WithScannerHealth(h ScannerHealth) Option {
	return func(s *Server) error {
		s.scanHealth = h
		return nil
	}
}

func WithAdminUI(dir string) Option {
	return func(s *Server) error {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			if _, ok := embeddedAdminUI(); !ok {
				return nil
			}
			s.adminUI = newAdminUIHandler("")
			return nil
		}
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("admin ui dir is not a directory: %s", dir)
		}
		s.adminUI = newAdminUIHandler(dir)
		return nil
	}
}

func (s *Server) observeUptime(now time.Time, uptimeSeconds int64) (restartsDetected int64, lastRestartAt *time.Time) {
	if s == nil {
		return 0, nil
	}
	now = now.UTC()

	s.muRestart.Lock()
	defer s.muRestart.Unlock()

	if s.lastUptimeKnown && uptimeSeconds < s.lastUptimeSeconds {
		s.junocashdRestartsDetected++
		t := now
		s.lastRestartAt = &t
	}
	s.lastUptimeKnown = true
	s.lastUptimeSeconds = uptimeSeconds

	var last *time.Time
	if s.lastRestartAt != nil {
		t := *s.lastRestartAt
		last = &t
	}
	return s.junocashdRestartsDetected, last
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/login", s.handleAdminLogin)
	mux.HandleFunc("/admin/logout", s.handleAdminLogout)
	if s.adminUI != nil {
		mux.Handle("/admin/", s.adminUI)
		mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
		})
	}
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/status", s.handleStatus)
	mux.HandleFunc("/v1/invoices", s.handleInvoices)
	mux.HandleFunc("/v1/public/invoices/", s.handlePublicInvoices)
	mux.HandleFunc("/v1/admin/status", s.handleAdminStatus)
	mux.HandleFunc("/v1/admin/merchants", s.handleAdminMerchants)
	mux.HandleFunc("/v1/admin/merchants/", s.handleAdminMerchantSubroutes)
	mux.HandleFunc("/v1/admin/api-keys/", s.handleAdminAPIKeys)
	mux.HandleFunc("/v1/admin/event-sinks", s.handleAdminEventSinks)
	mux.HandleFunc("/v1/admin/event-sinks/", s.handleAdminEventSinkSubroutes)
	mux.HandleFunc("/v1/admin/events", s.handleAdminEvents)
	mux.HandleFunc("/v1/admin/event-deliveries", s.handleAdminEventDeliveries)
	mux.HandleFunc("/v1/admin/review-cases", s.handleAdminReviewCases)
	mux.HandleFunc("/v1/admin/review-cases/", s.handleAdminReviewCaseSubroutes)
	mux.HandleFunc("/v1/admin/invoices", s.handleAdminInvoices)
	mux.HandleFunc("/v1/admin/invoices/", s.handleAdminInvoiceSubroutes)
	mux.HandleFunc("/v1/admin/deposits", s.handleAdminDeposits)
	mux.HandleFunc("/v1/admin/refunds", s.handleAdminRefunds)
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	bestHeight, bestHash, err := s.tip.BestTip(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "tip error")
		return
	}
	uptimeSeconds, err := s.tip.UptimeSeconds(ctx)
	uptimeAny := any(uptimeSeconds)
	if err != nil {
		if errors.Is(err, ErrUptimeUnsupported) {
			uptimeAny = nil
		} else {
			writeError(w, http.StatusInternalServerError, "internal", "uptime error")
			return
		}
	} else {
		s.observeUptime(s.clock.Now().UTC(), uptimeSeconds)
	}

	scannerConnected := false
	if s.scanHealth != nil {
		ok, err := s.scanHealth.Healthy(ctx)
		if err == nil && ok {
			scannerConnected = true
		}
	}

	lastCursor, lastEventAt, err := s.st.ScannerStatus(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	pendingDeliveries, err := s.st.PendingDeliveries(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	var lastEventAtAny any = nil
	if lastEventAt != nil && !lastEventAt.IsZero() {
		lastEventAtAny = lastEventAt.UTC().Format(time.RFC3339Nano)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"chain": map[string]any{
				"best_height":    bestHeight,
				"best_hash":      bestHash,
				"uptime_seconds": uptimeAny,
			},
			"scanner": map[string]any{
				"connected":           scannerConnected,
				"last_cursor_applied": lastCursor,
				"last_event_at":       lastEventAtAny,
			},
			"event_delivery": map[string]any{
				"pending_deliveries": pendingDeliveries,
			},
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

	addr, err := s.deriver.Derive(wallet.UFVK, wallet.UAHRP, addrIndex)
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
		s.handleListPublicInvoiceEvents(w, r, invoiceID)
	case "stream":
		s.handleStreamPublicInvoiceEvents(w, r, invoiceID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleGetPublicInvoice(w http.ResponseWriter, r *http.Request, invoiceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	inv, ok := s.authorizeInvoiceToken(w, ctx, r, invoiceID)
	if !ok {
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   toInvoiceJSON(inv),
	})
}

func (s *Server) handleListPublicInvoiceEvents(w http.ResponseWriter, r *http.Request, invoiceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := s.authorizeInvoiceToken(w, ctx, r, invoiceID); !ok {
		return
	}

	afterID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid cursor")
			return
		}
		afterID = n
	}

	events, nextCursor, err := s.st.ListInvoiceEvents(ctx, invoiceID, afterID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(events))
	for _, e := range events {
		out = append(out, toInvoiceEventJSON(e))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"events":      out,
			"next_cursor": strconv.FormatInt(nextCursor, 10),
		},
	})
}

func (s *Server) handleStreamPublicInvoiceEvents(w http.ResponseWriter, r *http.Request, invoiceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	if _, ok := s.authorizeInvoiceToken(w, ctx, r, invoiceID); !ok {
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "stream unsupported")
		return
	}

	afterID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil && n >= 0 {
			afterID = n
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evs, nextCursor, err := s.st.ListInvoiceEvents(ctx, invoiceID, afterID, 100)
			if err != nil {
				return
			}
			if len(evs) == 0 {
				continue
			}
			for _, e := range evs {
				msg, _ := json.Marshal(toInvoiceEventJSON(e))
				_, _ = w.Write([]byte("id: " + e.EventID + "\n"))
				_, _ = w.Write([]byte("event: invoice_event\n"))
				_, _ = w.Write([]byte("data: " + string(msg) + "\n\n"))
				flusher.Flush()
			}
			afterID = nextCursor
		}
	}
}

func (s *Server) authorizeInvoiceToken(w http.ResponseWriter, ctx context.Context, r *http.Request, invoiceID string) (domain.Invoice, bool) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
		return domain.Invoice{}, false
	}

	inv, ok, err := s.st.GetInvoice(ctx, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return domain.Invoice{}, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "invoice not found")
		return domain.Invoice{}, false
	}

	wantToken, ok, err := s.st.GetInvoiceToken(ctx, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return domain.Invoice{}, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "invoice not found")
		return domain.Invoice{}, false
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(wantToken)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
		return domain.Invoice{}, false
	}
	return inv, true
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

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && s.adminUI != nil {
		http.Redirect(w, r, "/admin/login/", http.StatusTemporaryRedirect)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.adminEnabled {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	pw := strings.TrimSpace(req.Password)
	if pw == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "password is required")
		return
	}
	if !s.checkAdminPassword(pw) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	if err := s.setAdminSessionCookie(w); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "session error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && s.adminUI != nil {
		http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}
	s.clearAdminSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	bestHeight, bestHash, err := s.tip.BestTip(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "tip error")
		return
	}
	uptimeSeconds, err := s.tip.UptimeSeconds(ctx)
	uptimeAny := any(uptimeSeconds)
	var restartsDetected int64
	var lastRestartAt *time.Time
	if err != nil {
		if errors.Is(err, ErrUptimeUnsupported) {
			uptimeAny = nil
		} else {
			writeError(w, http.StatusInternalServerError, "internal", "uptime error")
			return
		}
	} else {
		restartsDetected, lastRestartAt = s.observeUptime(s.clock.Now().UTC(), uptimeSeconds)
	}

	scannerConnected := false
	if s.scanHealth != nil {
		ok, err := s.scanHealth.Healthy(ctx)
		if err == nil && ok {
			scannerConnected = true
		}
	}

	lastCursor, lastEventAt, err := s.st.ScannerStatus(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	pendingDeliveries, err := s.st.PendingDeliveries(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	var lastEventAtAny any = nil
	if lastEventAt != nil && !lastEventAt.IsZero() {
		lastEventAtAny = lastEventAt.UTC().Format(time.RFC3339Nano)
	}
	var lastRestartAtAny any = nil
	if lastRestartAt != nil && !lastRestartAt.IsZero() {
		lastRestartAtAny = lastRestartAt.UTC().Format(time.RFC3339Nano)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"chain": map[string]any{
				"best_height":    bestHeight,
				"best_hash":      bestHash,
				"uptime_seconds": uptimeAny,
			},
			"scanner": map[string]any{
				"connected":           scannerConnected,
				"last_cursor_applied": lastCursor,
				"last_event_at":       lastEventAtAny,
			},
			"event_delivery": map[string]any{
				"pending_deliveries": pendingDeliveries,
			},
			"restarts": map[string]any{
				"junocashd_restarts_detected": restartsDetected,
				"last_restart_at":             lastRestartAtAny,
			},
		},
	})
}

func (s *Server) handleAdminMerchants(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleAdminCreateMerchant(w, r)
	case http.MethodGet:
		s.handleAdminListMerchants(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type merchantCreateRequest struct {
	Name     string                 `json:"name"`
	Settings *merchantSettingsInput `json:"settings,omitempty"`
}

type merchantSettingsInput struct {
	InvoiceTTLSeconds     int64                `json:"invoice_ttl_seconds"`
	RequiredConfirmations int32                `json:"required_confirmations"`
	Policies              invoicePoliciesInput `json:"policies"`
}

type invoicePoliciesInput struct {
	LatePaymentPolicy    string `json:"late_payment_policy"`
	PartialPaymentPolicy string `json:"partial_payment_policy"`
	OverpaymentPolicy    string `json:"overpayment_policy"`
}

func defaultMerchantSettings() domain.MerchantSettings {
	return domain.MerchantSettings{
		InvoiceTTLSeconds:     900,
		RequiredConfirmations: 100,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	}
}

func (in merchantSettingsInput) toDomain() domain.MerchantSettings {
	return domain.MerchantSettings{
		InvoiceTTLSeconds:     in.InvoiceTTLSeconds,
		RequiredConfirmations: in.RequiredConfirmations,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentPolicy(in.Policies.LatePaymentPolicy),
			PartialPayment: domain.PartialPaymentPolicy(in.Policies.PartialPaymentPolicy),
			Overpayment:    domain.OverpaymentPolicy(in.Policies.OverpaymentPolicy),
		},
	}
}

func (s *Server) handleAdminCreateMerchant(w http.ResponseWriter, r *http.Request) {
	var req merchantCreateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "name is required")
		return
	}

	settings := defaultMerchantSettings()
	if req.Settings != nil {
		settings = req.Settings.toDomain()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	m, err := s.st.CreateMerchant(ctx, req.Name, settings)
	if err != nil {
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"data":   toMerchantJSON(m),
	})
}

func (s *Server) handleAdminListMerchants(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ms, err := s.st.ListMerchants(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	out := make([]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, toMerchantJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   out,
	})
}

func (s *Server) handleAdminMerchantSubroutes(w http.ResponseWriter, r *http.Request) {
	// /v1/admin/merchants/{merchant_id}/...
	if !s.requireAdmin(w, r) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/admin/merchants/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	merchantID := parts[0]
	if len(parts) == 1 {
		s.handleAdminGetMerchant(w, r, merchantID)
		return
	}
	switch parts[1] {
	case "wallet":
		s.handleAdminSetMerchantWallet(w, r, merchantID)
	case "settings":
		s.handleAdminSetMerchantSettings(w, r, merchantID)
	case "api-keys":
		s.handleAdminCreateMerchantAPIKey(w, r, merchantID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAdminGetMerchant(w http.ResponseWriter, r *http.Request, merchantID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	m, ok, err := s.st.GetMerchant(ctx, merchantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "merchant not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   toMerchantJSON(m),
	})
}

type merchantWalletSetRequest struct {
	WalletID string `json:"wallet_id,omitempty"`
	UFVK     string `json:"ufvk"`
	Chain    string `json:"chain"`
	UAHRP    string `json:"ua_hrp"`
	CoinType int32  `json:"coin_type"`
}

func (s *Server) handleAdminSetMerchantWallet(w http.ResponseWriter, r *http.Request, merchantID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req merchantWalletSetRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.UFVK = strings.TrimSpace(req.UFVK)
	req.Chain = strings.TrimSpace(req.Chain)
	req.UAHRP = strings.TrimSpace(req.UAHRP)
	if req.WalletID == "" {
		req.WalletID = "wallet_" + merchantID
	}
	if req.UFVK == "" || req.Chain == "" || req.UAHRP == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "ufvk, chain, and ua_hrp are required")
		return
	}
	if _, err := s.deriver.Derive(req.UFVK, req.UAHRP, 0); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid ufvk")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	w0, err := s.st.SetMerchantWallet(ctx, merchantID, store.MerchantWallet{
		WalletID: req.WalletID,
		UFVK:     req.UFVK,
		Chain:    req.Chain,
		UAHRP:    req.UAHRP,
		CoinType: req.CoinType,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "wallet already set")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "merchant not found")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   toMerchantWalletJSON(w0),
	})
}

func (s *Server) handleAdminSetMerchantSettings(w http.ResponseWriter, r *http.Request, merchantID string) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var in merchantSettingsInput
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	m, err := s.st.UpdateMerchantSettings(ctx, merchantID, in.toDomain())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "merchant not found")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   toMerchantJSON(m),
	})
}

func (s *Server) handleAdminCreateMerchantAPIKey(w http.ResponseWriter, r *http.Request, merchantID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Label string `json:"label,omitempty"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	label := strings.TrimSpace(req.Label)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	keyID, apiKey, err := s.st.CreateMerchantAPIKey(ctx, merchantID, label)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "merchant not found")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	createdAt := s.clock.Now().UTC().Format(time.RFC3339Nano)
	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"api_key": apiKey,
			"key": map[string]any{
				"key_id":      keyID,
				"merchant_id": merchantID,
				"label":       label,
				"created_at":  createdAt,
				"revoked_at":  nil,
			},
		},
	})
}

func (s *Server) handleAdminAPIKeys(w http.ResponseWriter, r *http.Request) {
	// /v1/admin/api-keys/{key_id}
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keyID := strings.TrimPrefix(r.URL.Path, "/v1/admin/api-keys/")
	keyID = strings.TrimSpace(keyID)
	if keyID == "" || strings.Contains(keyID, "/") {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := s.st.RevokeMerchantAPIKey(ctx, keyID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleAdminEventSinks(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleAdminCreateEventSink(w, r)
	case http.MethodGet:
		s.handleAdminListEventSinks(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type eventSinkCreateRequest struct {
	MerchantID string         `json:"merchant_id"`
	Kind       string         `json:"kind"`
	Config     map[string]any `json:"config"`
}

func validateEventSinkConfig(kind domain.EventSinkKind, cfg map[string]any) error {
	switch kind {
	case domain.EventSinkWebhook:
		rawURL, _ := cfg["url"].(string)
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return errors.New("config.url is required")
		}
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return errors.New("config.url invalid")
		}
		if v, ok := cfg["secret"]; ok {
			if _, ok := v.(string); !ok {
				return errors.New("config.secret must be a string")
			}
		}
		if v, ok := cfg["timeout_ms"]; ok {
			switch n := v.(type) {
			case float64:
				if n < 0 {
					return errors.New("config.timeout_ms must be >= 0")
				}
			case int:
				if n < 0 {
					return errors.New("config.timeout_ms must be >= 0")
				}
			case int64:
				if n < 0 {
					return errors.New("config.timeout_ms must be >= 0")
				}
			default:
				return errors.New("config.timeout_ms must be a number")
			}
		}
		return nil

	case domain.EventSinkKafka:
		brokers, _ := cfg["brokers"].(string)
		brokers = strings.TrimSpace(brokers)
		topic, _ := cfg["topic"].(string)
		topic = strings.TrimSpace(topic)
		if brokers == "" {
			return errors.New("config.brokers is required")
		}
		if topic == "" {
			return errors.New("config.topic is required")
		}
		return nil

	case domain.EventSinkNATS:
		rawURL, _ := cfg["url"].(string)
		rawURL = strings.TrimSpace(rawURL)
		subject, _ := cfg["subject"].(string)
		subject = strings.TrimSpace(subject)
		if rawURL == "" {
			return errors.New("config.url is required")
		}
		if subject == "" {
			return errors.New("config.subject is required")
		}
		return nil

	case domain.EventSinkRabbitMQ:
		rawURL, _ := cfg["url"].(string)
		rawURL = strings.TrimSpace(rawURL)
		queue, _ := cfg["queue"].(string)
		queue = strings.TrimSpace(queue)
		if rawURL == "" {
			return errors.New("config.url is required")
		}
		if queue == "" {
			return errors.New("config.queue is required")
		}
		return nil

	default:
		return errors.New("kind invalid")
	}
}

func (s *Server) handleAdminCreateEventSink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req eventSinkCreateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	if req.MerchantID == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "merchant_id is required")
		return
	}

	kind := domain.EventSinkKind(strings.TrimSpace(req.Kind))
	if err := validateEventSinkConfig(kind, req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	cfgBytes, err := json.Marshal(req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", "config invalid")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sink, err := s.st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: req.MerchantID,
		Kind:       kind,
		Config:     cfgBytes,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "merchant not found")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"data":   toEventSinkJSON(sink),
	})
}

func (s *Server) handleAdminListEventSinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	sinks, err := s.st.ListEventSinks(ctx, merchantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(sinks))
	for _, sink := range sinks {
		out = append(out, toEventSinkJSON(sink))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   out,
	})
}

func (s *Server) handleAdminEventSinkSubroutes(w http.ResponseWriter, r *http.Request) {
	// /v1/admin/event-sinks/{sink_id}/test
	if !s.requireAdmin(w, r) {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/admin/event-sinks/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "test" {
		http.NotFound(w, r)
		return
	}
	sinkID := parts[0]
	s.handleAdminTestEventSink(w, r, sinkID)
}

func (s *Server) handleAdminTestEventSink(w http.ResponseWriter, r *http.Request, sinkID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	sink, found, err := s.st.GetEventSink(ctx, sinkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "sink not found")
		return
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "random error")
		return
	}
	eventID := "test_" + hex.EncodeToString(nonce[:])
	dataBytes, _ := json.Marshal(map[string]any{
		"merchant_id": sink.MerchantID,
		"sink_id":     sink.SinkID,
	})
	ev := domain.CloudEvent{
		SpecVersion:     "1.0",
		ID:              eventID,
		Source:          "juno-pay-server",
		Type:            "event-sink.test",
		Subject:         "event-sink/" + sink.SinkID,
		Time:            s.clock.Now().UTC(),
		DataContentType: "application/json",
		Data:            dataBytes,
	}

	worker, err := outbox.New(s.st)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "outbox error")
		return
	}

	if err := worker.Deliver(ctx, sink, ev); err != nil {
		writeError(w, http.StatusBadRequest, "delivery_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))

	afterID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid cursor")
			return
		}
		afterID = n
	}

	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid limit")
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	events, nextCursor, err := s.st.ListOutboundEvents(ctx, merchantID, afterID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if events == nil {
		events = []domain.CloudEvent{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"events":      events,
			"next_cursor": strconv.FormatInt(nextCursor, 10),
		},
	})
}

func (s *Server) handleAdminEventDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	sinkID := strings.TrimSpace(r.URL.Query().Get("sink_id"))
	status := domain.EventDeliveryStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", domain.EventDeliveryPending, domain.EventDeliveryDelivered, domain.EventDeliveryFailed:
	default:
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid status")
		return
	}

	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid limit")
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ds, err := s.st.ListEventDeliveries(ctx, store.EventDeliveryFilter{
		MerchantID: merchantID,
		SinkID:     sinkID,
		Status:     status,
		Limit:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(ds))
	for _, d := range ds {
		out = append(out, toEventDeliveryJSON(d))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   out,
	})
}

func (s *Server) handleAdminReviewCases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	status := domain.ReviewStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", domain.ReviewOpen, domain.ReviewResolved, domain.ReviewRejected:
	default:
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid status")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cases, err := s.st.ListReviewCases(ctx, store.ReviewCaseFilter{
		MerchantID: merchantID,
		Status:     status,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(cases))
	for _, c := range cases {
		out = append(out, toReviewCaseJSON(c))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   out,
	})
}

func (s *Server) handleAdminReviewCaseSubroutes(w http.ResponseWriter, r *http.Request) {
	// /v1/admin/review-cases/{review_id}/(resolve|reject)
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/admin/review-cases/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	reviewID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if reviewID == "" {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Notes string `json:"notes"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	req.Notes = strings.TrimSpace(req.Notes)
	if req.Notes == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "notes is required")
		return
	}
	if len(req.Notes) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid_argument", "notes too long")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var err error
	switch action {
	case "resolve":
		err = s.st.ResolveReviewCase(ctx, reviewID, req.Notes)
	case "reject":
		err = s.st.RejectReviewCase(ctx, reviewID, req.Notes)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "review case not found")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func (s *Server) handleAdminInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	externalOrderID := strings.TrimSpace(r.URL.Query().Get("external_order_id"))
	status := domain.InvoiceStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", domain.InvoiceOpen, domain.InvoicePartial, domain.InvoicePaid, domain.InvoiceOverpaid, domain.InvoiceExpired, domain.InvoicePaidLate, domain.InvoiceCanceled:
	default:
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid status")
		return
	}

	afterID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid cursor")
			return
		}
		afterID = n
	}

	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid limit")
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	invoices, nextCursor, err := s.st.ListInvoices(ctx, store.InvoiceFilter{
		MerchantID:      merchantID,
		Status:          status,
		ExternalOrderID: externalOrderID,
		AfterID:         afterID,
		Limit:           limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(invoices))
	for _, inv := range invoices {
		out = append(out, toInvoiceJSON(inv))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"invoices":    out,
			"next_cursor": strconv.FormatInt(nextCursor, 10),
		},
	})
}

func (s *Server) handleAdminInvoiceSubroutes(w http.ResponseWriter, r *http.Request) {
	// /v1/admin/invoices/{invoice_id}
	if !s.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	invoiceID := strings.TrimPrefix(r.URL.Path, "/v1/admin/invoices/")
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" || strings.Contains(invoiceID, "/") {
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	inv, found, err := s.st.GetInvoice(ctx, invoiceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "invoice not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data":   toInvoiceJSON(inv),
	})
}

func (s *Server) handleAdminDeposits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	invoiceID := strings.TrimSpace(r.URL.Query().Get("invoice_id"))
	txid := strings.TrimSpace(r.URL.Query().Get("txid"))

	afterID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid cursor")
			return
		}
		afterID = n
	}

	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid limit")
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	deps, nextCursor, err := s.st.ListDeposits(ctx, store.DepositFilter{
		MerchantID: merchantID,
		InvoiceID:  invoiceID,
		TxID:       txid,
		AfterID:    afterID,
		Limit:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(deps))
	for _, d := range deps {
		out = append(out, toDepositJSON(d))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"deposits":    out,
			"next_cursor": strconv.FormatInt(nextCursor, 10),
		},
	})
}

func (s *Server) handleAdminRefunds(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleAdminCreateRefund(w, r)
	case http.MethodGet:
		s.handleAdminListRefunds(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type refundCreateRequest struct {
	MerchantID       string `json:"merchant_id"`
	InvoiceID        string `json:"invoice_id,omitempty"`
	ExternalRefundID string `json:"external_refund_id,omitempty"`
	ToAddress        string `json:"to_address"`
	AmountZat        int64  `json:"amount_zat"`
	SentTxID         string `json:"sent_txid,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

func (s *Server) handleAdminCreateRefund(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req refundCreateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json")
		return
	}
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.InvoiceID = strings.TrimSpace(req.InvoiceID)
	req.ExternalRefundID = strings.TrimSpace(req.ExternalRefundID)
	req.ToAddress = strings.TrimSpace(req.ToAddress)
	req.SentTxID = strings.TrimSpace(req.SentTxID)
	req.Notes = strings.TrimSpace(req.Notes)
	if req.MerchantID == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "merchant_id is required")
		return
	}
	if req.ToAddress == "" {
		writeError(w, http.StatusBadRequest, "invalid_argument", "to_address is required")
		return
	}
	if req.AmountZat <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_argument", "amount_zat must be > 0")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	refund, err := s.st.CreateRefund(ctx, store.RefundCreate{
		MerchantID:       req.MerchantID,
		InvoiceID:        req.InvoiceID,
		ExternalRefundID: req.ExternalRefundID,
		ToAddress:        req.ToAddress,
		AmountZat:        req.AmountZat,
		SentTxID:         req.SentTxID,
		Notes:            req.Notes,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "not found")
			return
		case errors.Is(err, store.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "forbidden")
			return
		}
		if de, ok := domain.AsError(err); ok && de.Code == domain.ErrInvalidArgument {
			writeError(w, http.StatusBadRequest, string(de.Code), de.Message)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"data":   toRefundJSON(refund),
	})
}

func (s *Server) handleAdminListRefunds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	merchantID := strings.TrimSpace(r.URL.Query().Get("merchant_id"))
	invoiceID := strings.TrimSpace(r.URL.Query().Get("invoice_id"))
	status := domain.RefundStatus(strings.TrimSpace(r.URL.Query().Get("status")))
	switch status {
	case "", domain.RefundRequested, domain.RefundSent, domain.RefundCanceled:
	default:
		writeError(w, http.StatusBadRequest, "invalid_argument", "invalid status")
		return
	}

	afterID := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("cursor")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid cursor")
			return
		}
		afterID = n
	}

	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_argument", "invalid limit")
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	refunds, nextCursor, err := s.st.ListRefunds(ctx, store.RefundFilter{
		MerchantID: merchantID,
		InvoiceID:  invoiceID,
		Status:     status,
		AfterID:    afterID,
		Limit:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "db error")
		return
	}

	out := make([]any, 0, len(refunds))
	for _, refund := range refunds {
		out = append(out, toRefundJSON(refund))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"data": map[string]any{
			"refunds":     out,
			"next_cursor": strconv.FormatInt(nextCursor, 10),
		},
	})
}

func toMerchantJSON(m domain.Merchant) map[string]any {
	return map[string]any{
		"merchant_id": m.MerchantID,
		"name":        m.Name,
		"status":      string(m.Status),
		"settings": map[string]any{
			"invoice_ttl_seconds":    m.Settings.InvoiceTTLSeconds,
			"required_confirmations": m.Settings.RequiredConfirmations,
			"policies": map[string]any{
				"late_payment_policy":    string(m.Settings.Policies.LatePayment),
				"partial_payment_policy": string(m.Settings.Policies.PartialPayment),
				"overpayment_policy":     string(m.Settings.Policies.Overpayment),
			},
		},
		"created_at": m.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": m.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toMerchantWalletJSON(w0 store.MerchantWallet) map[string]any {
	return map[string]any{
		"merchant_id": w0.MerchantID,
		"wallet_id":   w0.WalletID,
		"ufvk":        w0.UFVK,
		"chain":       w0.Chain,
		"ua_hrp":      w0.UAHRP,
		"coin_type":   w0.CoinType,
		"created_at":  w0.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toEventSinkJSON(sink domain.EventSink) map[string]any {
	var cfg any = map[string]any{}
	var tmp any
	if err := json.Unmarshal(sink.Config, &tmp); err == nil {
		if m, ok := tmp.(map[string]any); ok {
			cfg = m
		}
	}

	return map[string]any{
		"sink_id":     sink.SinkID,
		"merchant_id": sink.MerchantID,
		"kind":        string(sink.Kind),
		"status":      string(sink.Status),
		"config":      cfg,
		"created_at":  sink.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":  sink.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toEventDeliveryJSON(d domain.EventDelivery) map[string]any {
	var nextRetryAt any = nil
	if d.NextRetryAt != nil {
		nextRetryAt = d.NextRetryAt.UTC().Format(time.RFC3339Nano)
	}
	var lastError any = nil
	if d.LastError != nil && strings.TrimSpace(*d.LastError) != "" {
		lastError = *d.LastError
	}

	return map[string]any{
		"delivery_id":   d.DeliveryID,
		"sink_id":       d.SinkID,
		"event_id":      d.EventID,
		"status":        string(d.Status),
		"attempt":       d.Attempt,
		"next_retry_at": nextRetryAt,
		"last_error":    lastError,
		"created_at":    d.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":    d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toDepositJSON(d domain.Deposit) map[string]any {
	var confirmedHeight any = nil
	if d.ConfirmedHeight != nil {
		confirmedHeight = *d.ConfirmedHeight
	}
	var invoiceID any = nil
	if d.InvoiceID != nil && strings.TrimSpace(*d.InvoiceID) != "" {
		invoiceID = *d.InvoiceID
	}

	return map[string]any{
		"wallet_id":         d.WalletID,
		"txid":              d.TxID,
		"action_index":      d.ActionIndex,
		"recipient_address": d.RecipientAddress,
		"amount_zat":        d.AmountZat,
		"height":            d.Height,
		"status":            string(d.Status),
		"confirmed_height":  confirmedHeight,
		"invoice_id":        invoiceID,
		"detected_at":       d.DetectedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":        d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toRefundJSON(r domain.Refund) map[string]any {
	var invoiceID any = nil
	if r.InvoiceID != nil && strings.TrimSpace(*r.InvoiceID) != "" {
		invoiceID = *r.InvoiceID
	}
	var externalRefundID any = nil
	if r.ExternalRefundID != nil && strings.TrimSpace(*r.ExternalRefundID) != "" {
		externalRefundID = *r.ExternalRefundID
	}
	var sentTxID any = nil
	if r.SentTxID != nil && strings.TrimSpace(*r.SentTxID) != "" {
		sentTxID = *r.SentTxID
	}

	return map[string]any{
		"refund_id":          r.RefundID,
		"merchant_id":        r.MerchantID,
		"invoice_id":         invoiceID,
		"external_refund_id": externalRefundID,
		"to_address":         r.ToAddress,
		"amount_zat":         r.AmountZat,
		"status":             string(r.Status),
		"sent_txid":          sentTxID,
		"notes":              r.Notes,
		"created_at":         r.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":         r.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toReviewCaseJSON(c domain.ReviewCase) map[string]any {
	var invoiceID any = nil
	if c.InvoiceID != nil && strings.TrimSpace(*c.InvoiceID) != "" {
		invoiceID = *c.InvoiceID
	}

	return map[string]any{
		"review_id":   c.ReviewID,
		"merchant_id": c.MerchantID,
		"invoice_id":  invoiceID,
		"reason":      string(c.Reason),
		"status":      string(c.Status),
		"notes":       c.Notes,
		"created_at":  c.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":  c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func toInvoiceEventJSON(e domain.InvoiceEvent) map[string]any {
	var dep any = nil
	if e.Deposit != nil {
		dep = map[string]any{
			"wallet_id":    e.Deposit.WalletID,
			"txid":         e.Deposit.TxID,
			"action_index": e.Deposit.ActionIndex,
			"amount_zat":   e.Deposit.AmountZat,
			"height":       e.Deposit.Height,
		}
	}
	var refund any = nil
	if e.Refund != nil {
		refund = toRefundJSON(*e.Refund)
	}

	return map[string]any{
		"event_id":    e.EventID,
		"type":        string(e.Type),
		"occurred_at": e.OccurredAt.UTC().Format(time.RFC3339Nano),
		"invoice_id":  e.InvoiceID,
		"deposit":     dep,
		"refund":      refund,
	}
}

func (s *Server) checkAdminPassword(password string) bool {
	sum := sha256.Sum256([]byte(password))
	return subtle.ConstantTimeCompare(sum[:], s.adminPassHash[:]) == 1
}

func (s *Server) setAdminSessionCookie(w http.ResponseWriter) error {
	ttl := s.adminSessionTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	exp := s.clock.Now().UTC().Add(ttl).Unix()

	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}

	payload := "v1." + strconv.FormatInt(exp, 10) + "." + hex.EncodeToString(nonce[:])
	sig := s.adminSign(payload)
	c := &http.Cookie{
		Name:     "juno_admin_session",
		Value:    payload + "." + hex.EncodeToString(sig),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
	return nil
}

func (s *Server) clearAdminSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "juno_admin_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) isAdminAuthenticated(r *http.Request) bool {
	c, err := r.Cookie("juno_admin_session")
	if err != nil || c == nil {
		return false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 4 {
		return false
	}
	if parts[0] != "v1" {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || exp <= 0 {
		return false
	}
	if s.clock.Now().UTC().Unix() > exp {
		return false
	}

	payload := strings.Join(parts[:3], ".")
	sig, err := hex.DecodeString(parts[3])
	if err != nil || len(sig) != sha256.Size {
		return false
	}
	want := s.adminSign(payload)
	return subtle.ConstantTimeCompare(sig, want) == 1
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.adminEnabled || !s.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return false
	}
	return true
}

func (s *Server) adminSign(payload string) []byte {
	mac := hmac.New(sha256.New, s.adminSessionKey[:])
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
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

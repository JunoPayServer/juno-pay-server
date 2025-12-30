package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-sdk-go/types"
)

type alwaysHealthy struct{ ok bool }

func (h alwaysHealthy) Healthy(_ context.Context) (bool, error) { return h.ok, nil }

type mutableTip struct {
	height  int64
	hash    string
	uptimeS int64
}

func (t *mutableTip) BestTip(_ context.Context) (int64, string, error) { return t.height, t.hash, nil }
func (t *mutableTip) UptimeSeconds(_ context.Context) (int64, error)   { return t.uptimeS, nil }

func TestStatus_Public(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", defaultMerchantSettings())
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}
	if _, err := st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkWebhook,
		Config:     json.RawMessage(`{"url":"https://example.com/webhook"}`),
	}); err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}
	if _, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1a",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             1,
		RequiredConfirmations: 1,
		Policies:              defaultMerchantSettings().Policies,
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	// Apply one scan event to advance cursor / last_event_at.
	p := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx1",
			Height:         1,
			ActionIndex:    0,
			AmountZatoshis: 1,
		},
		RecipientAddress: "j1unknown",
	}
	b, _ := json.Marshal(p)
	if err := st.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     7,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	tip := &mutableTip{height: 100, hash: "h100", uptimeS: 123}
	s, err := New(st, fakeDeriver{}, tip, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithScannerHealth(alwaysHealthy{ok: true}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Chain struct {
				BestHeight    int64  `json:"best_height"`
				BestHash      string `json:"best_hash"`
				UptimeSeconds int64  `json:"uptime_seconds"`
			} `json:"chain"`
			Scanner struct {
				Connected       bool    `json:"connected"`
				LastCursor      int64   `json:"last_cursor_applied"`
				LastEventAt     *string `json:"last_event_at"`
				LastEventAtNull any     `json:"-"`
			} `json:"scanner"`
			EventDelivery struct {
				PendingDeliveries int64 `json:"pending_deliveries"`
			} `json:"event_delivery"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if resp.Data.Chain.BestHeight != 100 || resp.Data.Chain.BestHash != "h100" || resp.Data.Chain.UptimeSeconds != 123 {
		t.Fatalf("unexpected chain status: %+v", resp.Data.Chain)
	}
	if !resp.Data.Scanner.Connected {
		t.Fatalf("expected scanner connected")
	}
	if resp.Data.Scanner.LastCursor != 7 {
		t.Fatalf("expected last_cursor_applied=7, got %d", resp.Data.Scanner.LastCursor)
	}
	if resp.Data.Scanner.LastEventAt == nil || *resp.Data.Scanner.LastEventAt == "" {
		t.Fatalf("expected last_event_at")
	}
	if resp.Data.EventDelivery.PendingDeliveries != 1 {
		t.Fatalf("expected pending_deliveries=1, got %d", resp.Data.EventDelivery.PendingDeliveries)
	}
}

func TestStatus_AdminDetectsJunocashdRestarts(t *testing.T) {
	st := store.NewMem()

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	tip := &mutableTip{height: 100, hash: "h100", uptimeS: 1000}
	s, err := New(st, fakeDeriver{}, tip, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, s, "pw")

	req1 := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
	req1.AddCookie(adminCookie)
	rec1 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("status1: expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	var resp1 struct {
		Data struct {
			Restarts struct {
				Detected      int64   `json:"junocashd_restarts_detected"`
				LastRestartAt *string `json:"last_restart_at"`
			} `json:"restarts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal status1: %v", err)
	}
	if resp1.Data.Restarts.Detected != 0 {
		t.Fatalf("expected 0 restarts, got %d", resp1.Data.Restarts.Detected)
	}
	if resp1.Data.Restarts.LastRestartAt != nil {
		t.Fatalf("expected last_restart_at null on first call")
	}

	// Uptime reset indicates restart.
	tip.uptimeS = 5
	req2 := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
	req2.AddCookie(adminCookie)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status2: expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp2 struct {
		Data struct {
			Restarts struct {
				Detected      int64   `json:"junocashd_restarts_detected"`
				LastRestartAt *string `json:"last_restart_at"`
			} `json:"restarts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal status2: %v", err)
	}
	if resp2.Data.Restarts.Detected != 1 {
		t.Fatalf("expected 1 restart, got %d", resp2.Data.Restarts.Detected)
	}
	if resp2.Data.Restarts.LastRestartAt == nil || *resp2.Data.Restarts.LastRestartAt == "" {
		t.Fatalf("expected last_restart_at to be set")
	}
}

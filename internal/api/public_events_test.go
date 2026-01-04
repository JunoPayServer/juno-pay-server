package api

import (
	"bytes"
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

func TestPublicInvoice_EventsAndAccounting(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	settings := domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	}

	m, err := st.CreateMerchant(ctx, "acme", settings)
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
	_, apiKey, err := st.CreateMerchantAPIKey(ctx, m.MerchantID, "default")
	if err != nil {
		t.Fatalf("CreateMerchantAPIKey: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	s, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create invoice.
	reqBody := []byte(`{"external_order_id":"order-1","amount_zat":100}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var createResp struct {
		Data struct {
			Invoice struct {
				InvoiceID string `json:"invoice_id"`
				Address   string `json:"address"`
			} `json:"invoice"`
			InvoiceToken string `json:"invoice_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	invoiceID := createResp.Data.Invoice.InvoiceID
	addr := createResp.Data.Invoice.Address
	token := createResp.Data.InvoiceToken
	if invoiceID == "" || addr == "" || token == "" {
		t.Fatalf("expected invoice_id/address/token")
	}

	// Apply a pending deposit (DepositEvent).
	depPayload := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx1",
			Height:         101,
			ActionIndex:    0,
			AmountZatoshis: 100,
		},
		RecipientAddress: addr,
	}
	depRaw, _ := json.Marshal(depPayload)
	if err := st.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       "DepositEvent",
		Height:     101,
		Payload:    depRaw,
		OccurredAt: now,
	}); err != nil {
		t.Fatalf("ApplyScanEvent: %v", err)
	}

	// Events should include deposit.detected.
	evReq := httptest.NewRequest(http.MethodGet, "/v1/public/invoices/"+invoiceID+"/events?token="+token, nil)
	evRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(evRec, evReq)
	if evRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", evRec.Code, evRec.Body.String())
	}
	var evResp struct {
		Status string `json:"status"`
		Data   struct {
			Events []struct {
				Type string `json:"type"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evRec.Body.Bytes(), &evResp); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}
	if evResp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	hasDetected := false
	for _, e := range evResp.Data.Events {
		if e.Type == "deposit.detected" {
			hasDetected = true
		}
	}
	if !hasDetected {
		t.Fatalf("expected deposit.detected event")
	}

	// Invoice should show pending status immediately (do not stay "open").
	invReq0 := httptest.NewRequest(http.MethodGet, "/v1/public/invoices/"+invoiceID+"?token="+token, nil)
	invRec0 := httptest.NewRecorder()
	s.Handler().ServeHTTP(invRec0, invReq0)
	if invRec0.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", invRec0.Code, invRec0.Body.String())
	}
	var invResp0 struct {
		Data struct {
			Status             string `json:"status"`
			ReceivedPendingZat int64  `json:"received_zat_pending"`
		} `json:"data"`
	}
	if err := json.Unmarshal(invRec0.Body.Bytes(), &invResp0); err != nil {
		t.Fatalf("unmarshal invoice: %v", err)
	}
	if invResp0.Data.Status != "pending" {
		t.Fatalf("expected status pending, got %q", invResp0.Data.Status)
	}
	if invResp0.Data.ReceivedPendingZat != 100 {
		t.Fatalf("expected received_zat_pending=100, got %d", invResp0.Data.ReceivedPendingZat)
	}

	// Confirm the deposit.
	confirmPayload := types.DepositConfirmedPayload{
		DepositEventPayload: depPayload,
		ConfirmedHeight:     102,
	}
	confirmRaw, _ := json.Marshal(confirmPayload)
	if err := st.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       "DepositConfirmed",
		Height:     101,
		Payload:    confirmRaw,
		OccurredAt: now,
	}); err != nil {
		t.Fatalf("ApplyScanEvent confirm: %v", err)
	}

	// Invoice should now be paid.
	invReq := httptest.NewRequest(http.MethodGet, "/v1/public/invoices/"+invoiceID+"?token="+token, nil)
	invRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(invRec, invReq)
	if invRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", invRec.Code, invRec.Body.String())
	}
	var invResp struct {
		Data struct {
			Status               string `json:"status"`
			ReceivedConfirmedZat int64  `json:"received_zat_confirmed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(invRec.Body.Bytes(), &invResp); err != nil {
		t.Fatalf("unmarshal invoice: %v", err)
	}
	if invResp.Data.Status != "confirmed" {
		t.Fatalf("expected status confirmed, got %q", invResp.Data.Status)
	}
	if invResp.Data.ReceivedConfirmedZat != 100 {
		t.Fatalf("expected received_zat_confirmed=100, got %d", invResp.Data.ReceivedConfirmedZat)
	}
}

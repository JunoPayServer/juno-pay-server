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

func TestAdmin_DepositsList(t *testing.T) {
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

	inv, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1a",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	p1 := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx1",
			Height:         1,
			ActionIndex:    0,
			AmountZatoshis: 10,
		},
		RecipientAddress: "j1a",
	}
	b1, _ := json.Marshal(p1)
	if err := st.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent 1: %v", err)
	}

	p2 := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx2",
			Height:         2,
			ActionIndex:    0,
			AmountZatoshis: 1,
		},
		RecipientAddress: "j1unknown",
	}
	b2, _ := json.Marshal(p2)
	if err := st.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b2,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent 2: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	srv, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, srv, "pw")

	// Unauthorized without cookie.
	unauthReq := httptest.NewRequest(http.MethodGet, "/v1/admin/deposits", nil)
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", unauthRec.Code, unauthRec.Body.String())
	}

	// Page 1
	req1 := httptest.NewRequest(http.MethodGet, "/v1/admin/deposits?merchant_id="+m.MerchantID+"&limit=1", nil)
	req1.AddCookie(adminCookie)
	rec1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("page1: expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	var resp1 struct {
		Status string `json:"status"`
		Data   struct {
			Deposits []struct {
				TxID      string  `json:"txid"`
				InvoiceID *string `json:"invoice_id"`
			} `json:"deposits"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal page1: %v", err)
	}
	if resp1.Status != "ok" {
		t.Fatalf("page1: expected status ok")
	}
	if len(resp1.Data.Deposits) != 1 || resp1.Data.Deposits[0].TxID != "tx1" {
		t.Fatalf("page1: expected tx1")
	}
	if resp1.Data.Deposits[0].InvoiceID == nil || *resp1.Data.Deposits[0].InvoiceID != inv.InvoiceID {
		t.Fatalf("page1: expected tx1 invoice_id")
	}
	if resp1.Data.NextCursor == "" {
		t.Fatalf("page1: expected next_cursor")
	}

	// Page 2
	req2 := httptest.NewRequest(http.MethodGet, "/v1/admin/deposits?merchant_id="+m.MerchantID+"&cursor="+resp1.Data.NextCursor+"&limit=10", nil)
	req2.AddCookie(adminCookie)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("page2: expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp2 struct {
		Data struct {
			Deposits []struct {
				TxID      string  `json:"txid"`
				InvoiceID *string `json:"invoice_id"`
			} `json:"deposits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal page2: %v", err)
	}
	if len(resp2.Data.Deposits) != 1 || resp2.Data.Deposits[0].TxID != "tx2" {
		t.Fatalf("page2: expected tx2")
	}
	if resp2.Data.Deposits[0].InvoiceID != nil {
		t.Fatalf("page2: expected tx2 invoice_id nil")
	}

	// Filter by invoice_id.
	fReq := httptest.NewRequest(http.MethodGet, "/v1/admin/deposits?merchant_id="+m.MerchantID+"&invoice_id="+inv.InvoiceID, nil)
	fReq.AddCookie(adminCookie)
	fRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(fRec, fReq)
	if fRec.Code != http.StatusOK {
		t.Fatalf("filter: expected 200, got %d: %s", fRec.Code, fRec.Body.String())
	}
	var fResp struct {
		Data struct {
			Deposits []struct {
				TxID string `json:"txid"`
			} `json:"deposits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(fRec.Body.Bytes(), &fResp); err != nil {
		t.Fatalf("unmarshal filter: %v", err)
	}
	if len(fResp.Data.Deposits) != 1 || fResp.Data.Deposits[0].TxID != "tx1" {
		t.Fatalf("filter: expected only tx1")
	}
}

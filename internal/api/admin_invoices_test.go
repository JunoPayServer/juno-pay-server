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
)

func TestAdmin_InvoicesListAndGet(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", defaultMerchantSettings())
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	inv1, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1a",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             1,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice inv1: %v", err)
	}
	inv2, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-2",
		WalletID:              "w1",
		AddressIndex:          1,
		Address:               "j1b",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             1,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice inv2: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	srv, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, srv, "pw")

	// Unauthorized without cookie.
	unauthReq := httptest.NewRequest(http.MethodGet, "/v1/admin/invoices", nil)
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", unauthRec.Code, unauthRec.Body.String())
	}

	// Page 1
	req1 := httptest.NewRequest(http.MethodGet, "/v1/admin/invoices?merchant_id="+m.MerchantID+"&limit=1", nil)
	req1.AddCookie(adminCookie)
	rec1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("page1: expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}
	var resp1 struct {
		Status string `json:"status"`
		Data   struct {
			Invoices []struct {
				InvoiceID string `json:"invoice_id"`
			} `json:"invoices"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal page1: %v", err)
	}
	if resp1.Status != "ok" {
		t.Fatalf("page1: expected status ok")
	}
	if len(resp1.Data.Invoices) != 1 || resp1.Data.Invoices[0].InvoiceID != inv1.InvoiceID {
		t.Fatalf("page1: expected inv1")
	}
	if resp1.Data.NextCursor == "" {
		t.Fatalf("page1: expected next_cursor")
	}

	// Page 2
	req2 := httptest.NewRequest(http.MethodGet, "/v1/admin/invoices?merchant_id="+m.MerchantID+"&cursor="+resp1.Data.NextCursor+"&limit=10", nil)
	req2.AddCookie(adminCookie)
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("page2: expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp2 struct {
		Data struct {
			Invoices []struct {
				InvoiceID string `json:"invoice_id"`
			} `json:"invoices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal page2: %v", err)
	}
	if len(resp2.Data.Invoices) != 1 || resp2.Data.Invoices[0].InvoiceID != inv2.InvoiceID {
		t.Fatalf("page2: expected inv2")
	}

	// Get invoice
	getReq := httptest.NewRequest(http.MethodGet, "/v1/admin/invoices/"+inv1.InvoiceID, nil)
	getReq.AddCookie(adminCookie)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	var getResp struct {
		Status string `json:"status"`
		Data   struct {
			InvoiceID string `json:"invoice_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if getResp.Status != "ok" {
		t.Fatalf("get: expected status ok")
	}
	if getResp.Data.InvoiceID != inv1.InvoiceID {
		t.Fatalf("get: expected invoice_id %q got %q", inv1.InvoiceID, getResp.Data.InvoiceID)
	}
}

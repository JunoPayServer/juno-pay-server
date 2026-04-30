package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/domain"
	"github.com/JunoPayServer/juno-pay-server/internal/store"
)

func TestAdmin_Refunds_CreateListAndInvoiceEvents(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", defaultMerchantSettings())
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
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
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if err := st.PutInvoiceToken(ctx, inv.InvoiceID, "tok"); err != nil {
		t.Fatalf("PutInvoiceToken: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	srv, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, srv, "pw")

	// Create refund.
	body, _ := json.Marshal(map[string]any{
		"merchant_id": m.MerchantID,
		"invoice_id":  inv.InvoiceID,
		"to_address":  "j1dest",
		"amount_zat":  1,
		"notes":       "n1",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/admin/refunds", bytes.NewReader(body))
	createReq.AddCookie(adminCookie)
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Status string `json:"status"`
		Data   struct {
			RefundID  string  `json:"refund_id"`
			Status    string  `json:"status"`
			InvoiceID *string `json:"invoice_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if createResp.Status != "ok" {
		t.Fatalf("create: expected status ok")
	}
	if createResp.Data.RefundID == "" {
		t.Fatalf("create: expected refund_id")
	}
	if createResp.Data.Status != "requested" {
		t.Fatalf("create: expected status requested")
	}
	if createResp.Data.InvoiceID == nil || *createResp.Data.InvoiceID != inv.InvoiceID {
		t.Fatalf("create: expected invoice_id")
	}

	// List refunds.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/admin/refunds?merchant_id="+m.MerchantID, nil)
	listReq.AddCookie(adminCookie)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Status string `json:"status"`
		Data   struct {
			Refunds []struct {
				RefundID string `json:"refund_id"`
			} `json:"refunds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Status != "ok" {
		t.Fatalf("list: expected status ok")
	}
	if len(listResp.Data.Refunds) != 1 || listResp.Data.Refunds[0].RefundID != createResp.Data.RefundID {
		t.Fatalf("list: expected refund")
	}

	// Public invoice events should include refund.requested with refund object.
	evReq := httptest.NewRequest(http.MethodGet, "/v1/public/invoices/"+inv.InvoiceID+"/events?token=tok", nil)
	evRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(evRec, evReq)
	if evRec.Code != http.StatusOK {
		t.Fatalf("events: expected 200, got %d: %s", evRec.Code, evRec.Body.String())
	}
	var evResp struct {
		Status string `json:"status"`
		Data   struct {
			Events []struct {
				Type   string          `json:"type"`
				Refund json.RawMessage `json:"refund"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evRec.Body.Bytes(), &evResp); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}
	if evResp.Status != "ok" {
		t.Fatalf("events: expected status ok")
	}
	found := false
	for _, e := range evResp.Data.Events {
		if e.Type != "refund.requested" {
			continue
		}
		found = true
		var r struct {
			RefundID string `json:"refund_id"`
		}
		if err := json.Unmarshal(e.Refund, &r); err != nil {
			t.Fatalf("unmarshal refund: %v", err)
		}
		if r.RefundID != createResp.Data.RefundID {
			t.Fatalf("refund_id mismatch")
		}
	}
	if !found {
		t.Fatalf("expected refund.requested event")
	}
}

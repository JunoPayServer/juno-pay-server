package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

func TestDemoAirPurchase_CreatesInvoice(t *testing.T) {
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

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	srv, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"buyer_id": "user-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/demo/air/purchase", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Invoice struct {
				InvoiceID       string `json:"invoice_id"`
				ExternalOrderID string `json:"external_order_id"`
				AmountZat       int64  `json:"amount_zat"`
				Address         string `json:"address"`
				Status          string `json:"status"`
				Policies        struct {
					LatePaymentPolicy    string `json:"late_payment_policy"`
					PartialPaymentPolicy string `json:"partial_payment_policy"`
					OverpaymentPolicy    string `json:"overpayment_policy"`
				} `json:"policies"`
			} `json:"invoice"`
			InvoiceToken string `json:"invoice_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if resp.Data.Invoice.InvoiceID == "" {
		t.Fatalf("expected invoice_id")
	}
	if !strings.HasPrefix(resp.Data.Invoice.ExternalOrderID, "demo-air-") {
		t.Fatalf("expected demo external_order_id prefix, got %q", resp.Data.Invoice.ExternalOrderID)
	}
	if resp.Data.Invoice.AmountZat != 100_000_000 {
		t.Fatalf("expected amount_zat=100000000 (1 JUNO), got %d", resp.Data.Invoice.AmountZat)
	}
	if resp.Data.Invoice.Address != "j1addr0" {
		t.Fatalf("expected derived address j1addr0, got %q", resp.Data.Invoice.Address)
	}
	if resp.Data.Invoice.Status != string(domain.InvoiceOpen) {
		t.Fatalf("expected status open, got %q", resp.Data.Invoice.Status)
	}
	if resp.Data.InvoiceToken != "tok" {
		t.Fatalf("expected invoice_token tok")
	}
}

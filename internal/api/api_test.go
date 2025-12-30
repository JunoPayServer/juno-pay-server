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
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type fixedTip struct {
	height int64
	hash   string
}

func (t fixedTip) BestTip(_ context.Context) (int64, string, error) { return t.height, t.hash, nil }

type fakeDeriver struct{}

func (fakeDeriver) Derive(_ string, _ string, index uint32) (string, error) {
	return "j1addr" + itoa(uint64(index)), nil
}

type fixedTokenGen struct{ token string }

func (g fixedTokenGen) NewInvoiceToken() (string, error) { return g.token, nil }

func TestCreateInvoice_SuccessAndIdempotent(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	settings := domain.MerchantSettings{
		InvoiceTTLSeconds:     600,
		RequiredConfirmations: 10,
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

	body := map[string]any{
		"external_order_id": "order-1",
		"amount_zat":        1234,
		"metadata": map[string]any{
			"k": "v",
		},
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Invoice struct {
				InvoiceID       string  `json:"invoice_id"`
				MerchantID      string  `json:"merchant_id"`
				ExternalOrderID string  `json:"external_order_id"`
				Status          string  `json:"status"`
				Address         string  `json:"address"`
				AmountZat       int64   `json:"amount_zat"`
				RequiredConfs   int32   `json:"required_confirmations"`
				ExpiresAt       *string `json:"expires_at"`
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
	if resp.Data.Invoice.MerchantID != m.MerchantID {
		t.Fatalf("merchant_id mismatch")
	}
	if resp.Data.Invoice.ExternalOrderID != "order-1" {
		t.Fatalf("external_order_id mismatch")
	}
	if resp.Data.Invoice.AmountZat != 1234 {
		t.Fatalf("amount_zat mismatch")
	}
	if resp.Data.Invoice.RequiredConfs != 10 {
		t.Fatalf("required confirmations mismatch")
	}
	if resp.Data.Invoice.Address != "j1addr0" {
		t.Fatalf("expected derived address j1addr0, got %q", resp.Data.Invoice.Address)
	}
	if resp.Data.Invoice.ExpiresAt == nil {
		t.Fatalf("expected expires_at")
	}
	exp, err := time.Parse(time.RFC3339Nano, *resp.Data.Invoice.ExpiresAt)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}
	wantExp := now.Add(600 * time.Second)
	if !exp.Equal(wantExp) {
		t.Fatalf("expires_at mismatch: got %s want %s", exp.Format(time.RFC3339Nano), wantExp.Format(time.RFC3339Nano))
	}
	if resp.Data.Invoice.Status != "open" {
		t.Fatalf("expected status open")
	}
	if resp.Data.InvoiceToken != "tok" {
		t.Fatalf("expected invoice_token")
	}

	// Retry (idempotent)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(b))
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var resp2 struct {
		Data struct {
			Invoice struct {
				InvoiceID string `json:"invoice_id"`
			} `json:"invoice"`
			InvoiceToken string `json:"invoice_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal2: %v", err)
	}
	if resp2.Data.Invoice.InvoiceID != resp.Data.Invoice.InvoiceID {
		t.Fatalf("expected same invoice_id on retry")
	}
	if resp2.Data.InvoiceToken != resp.Data.InvoiceToken {
		t.Fatalf("expected same invoice_token on retry")
	}
}

func TestCreateInvoice_Conflict(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
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

	s, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: time.Now().UTC()}, fixedTokenGen{token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	mkreq := func(amount int) *http.Request {
		body := map[string]any{
			"external_order_id": "order-1",
			"amount_zat":        amount,
		}
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+apiKey)
		return r
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, mkreq(123))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, mkreq(124))
	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestCreateInvoice_Unauthorized(t *testing.T) {
	st := store.NewMem()
	s, err := New(st, fakeDeriver{}, fixedTip{height: 0, hash: ""}, fixedClock{t: time.Now().UTC()}, fixedTokenGen{token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	body := []byte(`{"external_order_id":"o","amount_zat":1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestPublicInvoice_TokenRequired(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
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

	// Create invoice directly in store (simulating previously created invoice).
	inv, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:      m.MerchantID,
		ExternalOrderID: "order-1",
		WalletID:        "w1",
		AddressIndex:    0,
		Address:         "j1addr0",
		AmountZat:       1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if err := st.PutInvoiceToken(ctx, inv.InvoiceID, "tok"); err != nil {
		t.Fatalf("PutInvoiceToken: %v", err)
	}

	s, err := New(st, fakeDeriver{}, fixedTip{height: 0, hash: ""}, fixedClock{t: time.Now().UTC()}, fixedTokenGen{token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/public/invoices/"+inv.InvoiceID+"?token=tok", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/public/invoices/"+inv.InvoiceID+"?token=bad", nil)
	rec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec2.Code)
	}
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

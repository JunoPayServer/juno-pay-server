package outbox

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/domain"
	"github.com/JunoPayServer/juno-pay-server/internal/store"
)

func TestWorker_WebhookDelivery_Success(t *testing.T) {
	ctx := context.Background()

	var gotBody []byte
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Juno-Signature")
		b, _ := io.ReadAll(r.Body)
		gotBody = b
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	st := store.NewMem()
	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	if _, err := st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkWebhook,
		Config:     json.RawMessage(`{"url":"` + srv.URL + `","secret":"s"}`),
	}); err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}

	_, _, err = st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
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
		t.Fatalf("CreateInvoice: %v", err)
	}

	w, err := New(st, WithNowFunc(func() time.Time { return time.Unix(100, 0).UTC() }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(gotBody) == 0 {
		t.Fatalf("expected webhook body")
	}
	var ce domain.CloudEvent
	if err := json.Unmarshal(gotBody, &ce); err != nil {
		t.Fatalf("decode cloudevent: %v", err)
	}
	if ce.SpecVersion != "1.0" || ce.Type == "" || ce.ID == "" {
		t.Fatalf("unexpected cloudevent: %+v", ce)
	}

	mac := hmac.New(sha256.New, []byte("s"))
	_, _ = mac.Write(gotBody)
	wantSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != wantSig {
		t.Fatalf("signature mismatch: got %q want %q", gotSig, wantSig)
	}

	ds, err := st.ListEventDeliveries(ctx, store.EventDeliveryFilter{MerchantID: m.MerchantID, Limit: 10})
	if err != nil {
		t.Fatalf("ListEventDeliveries: %v", err)
	}
	if len(ds) == 0 {
		t.Fatalf("expected at least 1 delivery")
	}
	if ds[0].Status != domain.EventDeliveryDelivered {
		t.Fatalf("expected delivered, got %q", ds[0].Status)
	}
	if ds[0].Attempt != 1 {
		t.Fatalf("expected attempt=1, got %d", ds[0].Attempt)
	}
}

func TestWorker_WebhookDelivery_Retry(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	st := store.NewMem()
	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	if _, err := st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkWebhook,
		Config:     json.RawMessage(`{"url":"` + srv.URL + `"}`),
	}); err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}

	_, _, err = st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
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
		t.Fatalf("CreateInvoice: %v", err)
	}

	now := time.Unix(100, 0).UTC()
	w, err := New(st, WithNowFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	ds, err := st.ListEventDeliveries(ctx, store.EventDeliveryFilter{MerchantID: m.MerchantID, Limit: 10})
	if err != nil {
		t.Fatalf("ListEventDeliveries: %v", err)
	}
	if len(ds) == 0 {
		t.Fatalf("expected at least 1 delivery")
	}
	if ds[0].Status != domain.EventDeliveryPending {
		t.Fatalf("expected pending, got %q", ds[0].Status)
	}
	if ds[0].Attempt != 1 {
		t.Fatalf("expected attempt=1, got %d", ds[0].Attempt)
	}
	if ds[0].NextRetryAt == nil || !ds[0].NextRetryAt.After(now) {
		t.Fatalf("expected next_retry_at after now")
	}
	if ds[0].LastError == nil || *ds[0].LastError == "" {
		t.Fatalf("expected last_error")
	}
}


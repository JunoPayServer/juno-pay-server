package api

import (
	"bytes"
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

func adminLogin(t *testing.T, s *Server, password string) *http.Cookie {
	t.Helper()

	loginBody, _ := json.Marshal(map[string]any{"password": password})
	req := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(loginBody))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("admin login: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	cs := rec.Result().Cookies()
	if len(cs) != 1 {
		t.Fatalf("admin login: expected 1 cookie, got %d", len(cs))
	}
	return cs[0]
}

type nilOutboundEventsStore struct {
	store.Store
}

func (s nilOutboundEventsStore) ListOutboundEvents(ctx context.Context, merchantID string, afterID int64, limit int) ([]domain.CloudEvent, int64, error) {
	_, next, err := s.Store.ListOutboundEvents(ctx, merchantID, afterID, limit)
	return nil, next, err
}

func TestAdmin_OutboundEvents_EmptyIsJSONArray(t *testing.T) {
	base := store.NewMem()
	ctx := context.Background()

	m, err := base.CreateMerchant(ctx, "acme", defaultMerchantSettings())
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	s, err := New(nilOutboundEventsStore{Store: base}, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, s, "pw")

	evReq := httptest.NewRequest(http.MethodGet, "/v1/admin/events?merchant_id="+m.MerchantID, nil)
	evReq.AddCookie(adminCookie)
	evRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(evRec, evReq)
	if evRec.Code != http.StatusOK {
		t.Fatalf("list events: expected 200, got %d: %s", evRec.Code, evRec.Body.String())
	}

	var evResp struct {
		Status string `json:"status"`
		Data   struct {
			Events []domain.CloudEvent `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evRec.Body.Bytes(), &evResp); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}
	if evResp.Status != "ok" {
		t.Fatalf("events: expected status ok")
	}
	if evResp.Data.Events == nil {
		t.Fatalf("events: expected JSON array, got null")
	}
	if len(evResp.Data.Events) != 0 {
		t.Fatalf("events: expected 0 events, got %d", len(evResp.Data.Events))
	}
}

func TestAdmin_EventSinksAndOutboundEvents(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", defaultMerchantSettings())
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	s, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, s, "pw")

	// Create a sink.
	createBody, _ := json.Marshal(map[string]any{
		"merchant_id": m.MerchantID,
		"kind":        "webhook",
		"config": map[string]any{
			"url": "https://example.com/webhook",
		},
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/admin/event-sinks", bytes.NewReader(createBody))
	createReq.AddCookie(adminCookie)
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create sink: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Status string `json:"status"`
		Data   struct {
			SinkID     string `json:"sink_id"`
			MerchantID string `json:"merchant_id"`
			Kind       string `json:"kind"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal create sink: %v", err)
	}
	if createResp.Status != "ok" {
		t.Fatalf("create sink: expected status ok, got %q", createResp.Status)
	}
	if createResp.Data.SinkID == "" {
		t.Fatalf("create sink: expected sink_id")
	}
	if createResp.Data.MerchantID != m.MerchantID {
		t.Fatalf("create sink: merchant_id mismatch")
	}
	if createResp.Data.Kind != "webhook" {
		t.Fatalf("create sink: kind mismatch")
	}
	if createResp.Data.Status != "active" {
		t.Fatalf("create sink: expected active status, got %q", createResp.Data.Status)
	}

	// List sinks.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/admin/event-sinks?merchant_id="+m.MerchantID, nil)
	listReq.AddCookie(adminCookie)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list sinks: expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Status string `json:"status"`
		Data   []struct {
			SinkID string `json:"sink_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Status != "ok" {
		t.Fatalf("list: expected status ok")
	}
	if len(listResp.Data) != 1 || listResp.Data[0].SinkID != createResp.Data.SinkID {
		t.Fatalf("list: expected 1 sink")
	}

	// Create an invoice directly to generate an outbox event + delivery.
	if _, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
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
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	// List outbox events.
	evReq := httptest.NewRequest(http.MethodGet, "/v1/admin/events?merchant_id="+m.MerchantID, nil)
	evReq.AddCookie(adminCookie)
	evRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(evRec, evReq)
	if evRec.Code != http.StatusOK {
		t.Fatalf("list events: expected 200, got %d: %s", evRec.Code, evRec.Body.String())
	}
	var evResp struct {
		Status string `json:"status"`
		Data   struct {
			Events []domain.CloudEvent `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(evRec.Body.Bytes(), &evResp); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}
	if evResp.Status != "ok" {
		t.Fatalf("events: expected status ok")
	}
	if len(evResp.Data.Events) == 0 {
		t.Fatalf("events: expected at least 1 event")
	}
	if evResp.Data.Events[0].Type != "invoice.created" {
		t.Fatalf("events: expected invoice.created, got %q", evResp.Data.Events[0].Type)
	}

	// List deliveries.
	dReq := httptest.NewRequest(http.MethodGet, "/v1/admin/event-deliveries?merchant_id="+m.MerchantID, nil)
	dReq.AddCookie(adminCookie)
	dRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(dRec, dReq)
	if dRec.Code != http.StatusOK {
		t.Fatalf("list deliveries: expected 200, got %d: %s", dRec.Code, dRec.Body.String())
	}
	var dResp struct {
		Status string `json:"status"`
		Data   []struct {
			DeliveryID string `json:"delivery_id"`
			SinkID     string `json:"sink_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(dRec.Body.Bytes(), &dResp); err != nil {
		t.Fatalf("unmarshal deliveries: %v", err)
	}
	if dResp.Status != "ok" {
		t.Fatalf("deliveries: expected status ok")
	}
	if len(dResp.Data) == 0 {
		t.Fatalf("deliveries: expected at least 1 delivery")
	}
	if dResp.Data[0].SinkID != createResp.Data.SinkID {
		t.Fatalf("deliveries: expected sink_id=%q got %q", createResp.Data.SinkID, dResp.Data[0].SinkID)
	}
}

func TestAdmin_TestEventSink_Webhook(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", defaultMerchantSettings())
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	type received struct {
		Signature   string
		ContentType string
		Body        []byte
	}
	gotCh := make(chan received, 1)

	const secret = "s3cr3t"
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotCh <- received{
			Signature:   r.Header.Get("X-Juno-Signature"),
			ContentType: r.Header.Get("Content-Type"),
			Body:        b,
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer h.Close()

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	s, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, s, "pw")

	createBody, _ := json.Marshal(map[string]any{
		"merchant_id": m.MerchantID,
		"kind":        "webhook",
		"config": map[string]any{
			"url":    h.URL,
			"secret": secret,
		},
	})
	createReq := httptest.NewRequest(http.MethodPost, "/v1/admin/event-sinks", bytes.NewReader(createBody))
	createReq.AddCookie(adminCookie)
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create sink: expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			SinkID string `json:"sink_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}

	testReq := httptest.NewRequest(http.MethodPost, "/v1/admin/event-sinks/"+createResp.Data.SinkID+"/test", nil)
	testReq.AddCookie(adminCookie)
	testRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("test sink: expected 200, got %d: %s", testRec.Code, testRec.Body.String())
	}

	select {
	case got := <-gotCh:
		if got.ContentType != "application/cloudevents+json" {
			t.Fatalf("expected cloudevents content-type, got %q", got.ContentType)
		}

		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(got.Body)
		wantSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if got.Signature != wantSig {
			t.Fatalf("signature mismatch: got %q want %q", got.Signature, wantSig)
		}

		var ce domain.CloudEvent
		if err := json.Unmarshal(got.Body, &ce); err != nil {
			t.Fatalf("unmarshal cloudevent: %v", err)
		}
		if ce.SpecVersion != "1.0" {
			t.Fatalf("expected specversion 1.0, got %q", ce.SpecVersion)
		}
		if ce.Type == "" {
			t.Fatalf("expected ce.type")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for webhook")
	}
}

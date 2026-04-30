package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/store"
	"github.com/JunoPayServer/juno-sdk-go/types"
)

func TestAdmin_ReviewCases_ListResolveReject(t *testing.T) {
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

	// Create 2 unknown address deposits -> 2 review cases.
	for i := 1; i <= 2; i++ {
		p := types.DepositEventPayload{
			DepositEvent: types.DepositEvent{
				WalletID:       "w1",
				TxID:           fmt.Sprintf("tx%d", i),
				Height:         int64(i),
				ActionIndex:    0,
				AmountZatoshis: 1,
			},
			RecipientAddress: fmt.Sprintf("j1unknown%d", i),
		}
		b, _ := json.Marshal(p)
		if err := st.ApplyScanEvent(ctx, store.ScanEvent{
			WalletID:   "w1",
			Cursor:     int64(i),
			Kind:       string(types.WalletEventKindDepositEvent),
			Payload:    b,
			OccurredAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("ApplyScanEvent %d: %v", i, err)
		}
	}

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	srv, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, srv, "pw")

	// Unauthorized without cookie.
	unauthReq := httptest.NewRequest(http.MethodGet, "/v1/admin/review-cases", nil)
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", unauthRec.Code, unauthRec.Body.String())
	}

	// List open cases.
	listReq := httptest.NewRequest(http.MethodGet, "/v1/admin/review-cases?merchant_id="+m.MerchantID+"&status=open", nil)
	listReq.AddCookie(adminCookie)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Status string `json:"status"`
		Data   []struct {
			ReviewID string `json:"review_id"`
			Reason   string `json:"reason"`
			Status   string `json:"status"`
			Notes    string `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if len(listResp.Data) != 2 {
		t.Fatalf("expected 2 review cases, got %d", len(listResp.Data))
	}
	if listResp.Data[0].ReviewID == "" || listResp.Data[1].ReviewID == "" {
		t.Fatalf("expected review_id fields")
	}

	rid1 := listResp.Data[0].ReviewID
	rid2 := listResp.Data[1].ReviewID

	// Resolve first.
	resolveBody, _ := json.Marshal(map[string]any{"notes": "resolved"})
	resolveReq := httptest.NewRequest(http.MethodPost, "/v1/admin/review-cases/"+rid1+"/resolve", bytes.NewReader(resolveBody))
	resolveReq.AddCookie(adminCookie)
	resolveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("resolve: expected 200, got %d: %s", resolveRec.Code, resolveRec.Body.String())
	}
	var okResp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resolveRec.Body.Bytes(), &okResp); err != nil {
		t.Fatalf("unmarshal resolve: %v", err)
	}
	if okResp.Status != "ok" {
		t.Fatalf("expected status ok")
	}

	// Reject second.
	rejectBody, _ := json.Marshal(map[string]any{"notes": "rejected"})
	rejectReq := httptest.NewRequest(http.MethodPost, "/v1/admin/review-cases/"+rid2+"/reject", bytes.NewReader(rejectBody))
	rejectReq.AddCookie(adminCookie)
	rejectRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusOK {
		t.Fatalf("reject: expected 200, got %d: %s", rejectRec.Code, rejectRec.Body.String())
	}

	// List resolved.
	resolvedReq := httptest.NewRequest(http.MethodGet, "/v1/admin/review-cases?merchant_id="+m.MerchantID+"&status=resolved", nil)
	resolvedReq.AddCookie(adminCookie)
	resolvedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resolvedRec, resolvedReq)
	if resolvedRec.Code != http.StatusOK {
		t.Fatalf("resolved list: expected 200, got %d: %s", resolvedRec.Code, resolvedRec.Body.String())
	}
	var resolvedResp struct {
		Data []struct {
			ReviewID string `json:"review_id"`
			Status   string `json:"status"`
			Notes    string `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resolvedRec.Body.Bytes(), &resolvedResp); err != nil {
		t.Fatalf("unmarshal resolved: %v", err)
	}
	if len(resolvedResp.Data) != 1 || resolvedResp.Data[0].ReviewID != rid1 {
		t.Fatalf("expected 1 resolved case with review_id=%q", rid1)
	}
	if resolvedResp.Data[0].Status != "resolved" {
		t.Fatalf("expected status resolved, got %q", resolvedResp.Data[0].Status)
	}
	if resolvedResp.Data[0].Notes != "resolved" {
		t.Fatalf("expected notes to be updated")
	}

	// List rejected.
	rejectedReq := httptest.NewRequest(http.MethodGet, "/v1/admin/review-cases?merchant_id="+m.MerchantID+"&status=rejected", nil)
	rejectedReq.AddCookie(adminCookie)
	rejectedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rejectedRec, rejectedReq)
	if rejectedRec.Code != http.StatusOK {
		t.Fatalf("rejected list: expected 200, got %d: %s", rejectedRec.Code, rejectedRec.Body.String())
	}
	var rejectedResp struct {
		Data []struct {
			ReviewID string `json:"review_id"`
			Status   string `json:"status"`
			Notes    string `json:"notes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rejectedRec.Body.Bytes(), &rejectedResp); err != nil {
		t.Fatalf("unmarshal rejected: %v", err)
	}
	if len(rejectedResp.Data) != 1 || rejectedResp.Data[0].ReviewID != rid2 {
		t.Fatalf("expected 1 rejected case with review_id=%q", rid2)
	}
	if rejectedResp.Data[0].Status != "rejected" {
		t.Fatalf("expected status rejected, got %q", rejectedResp.Data[0].Status)
	}
	if rejectedResp.Data[0].Notes != "rejected" {
		t.Fatalf("expected notes to be updated")
	}
}

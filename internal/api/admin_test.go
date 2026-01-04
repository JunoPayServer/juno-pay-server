package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

func TestAdmin_MerchantLifecycleAndAPIKey(t *testing.T) {
	st := store.NewMem()
	ctx := context.Background()

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)
	s, err := New(st, fakeDeriver{}, fixedTip{height: 100, hash: "h100"}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Login
	loginBody := []byte(`{"password":"pw"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/login", bytes.NewReader(loginBody))
	loginRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", loginRec.Code, loginRec.Body.String())
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	adminCookie := cookies[0]

	// Create merchant (with explicit settings)
	reqBody := map[string]any{
		"name": "acme",
		"settings": map[string]any{
			"invoice_ttl_seconds":     600,
			"required_confirmations": 10,
			"policies": map[string]any{
				"late_payment_policy":    "mark_paid_late",
				"partial_payment_policy": "accept_partial",
				"overpayment_policy":     "mark_overpaid",
			},
		},
	}
	b, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/v1/admin/merchants", bytes.NewReader(b))
	createReq = createReq.WithContext(ctx)
	createReq.AddCookie(adminCookie)
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Status string `json:"status"`
		Data   struct {
			MerchantID string `json:"merchant_id"`
			Name       string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if createResp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if createResp.Data.MerchantID == "" {
		t.Fatalf("expected merchant_id")
	}
	if createResp.Data.Name != "acme" {
		t.Fatalf("expected name acme")
	}
	merchantID := createResp.Data.MerchantID

	// Set merchant wallet (immutable)
	walletBody := map[string]any{
		"ufvk":     "jview1js32zyfmmd4yzqy04pf9qwqrj47w3uvekjzs7pzfh2ars2v0ggzg74cd39lw9px0tr0nq7e86xevgx7fqxzslmlfqcaw28wj75prfgd0xdae7fywxl99n035kejzpj9upard7kegh3epjna7efmzy392cyr7a2hs4khc00zq0j2jqnnnz0usmuc92r5un",
		"chain":    "mainnet",
		"ua_hrp":   "j",
		"coin_type": 8133,
	}
	wb, _ := json.Marshal(walletBody)
	wReq := httptest.NewRequest(http.MethodPost, "/v1/admin/merchants/"+merchantID+"/wallet", bytes.NewReader(wb))
	wReq.AddCookie(adminCookie)
	wRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(wRec, wReq)
	if wRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", wRec.Code, wRec.Body.String())
	}

	// Create merchant API key
	keyBody := []byte(`{"label":"default"}`)
	kReq := httptest.NewRequest(http.MethodPost, "/v1/admin/merchants/"+merchantID+"/api-keys", bytes.NewReader(keyBody))
	kReq.AddCookie(adminCookie)
	kRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(kRec, kReq)
	if kRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", kRec.Code, kRec.Body.String())
	}
	var keyResp struct {
		Status string `json:"status"`
		Data   struct {
			APIKey string `json:"api_key"`
			Key    struct {
				KeyID string `json:"key_id"`
			} `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(kRec.Body.Bytes(), &keyResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if keyResp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if keyResp.Data.APIKey == "" {
		t.Fatalf("expected api_key")
	}
		if keyResp.Data.Key.KeyID == "" {
			t.Fatalf("expected key_id")
		}

		// Merchant detail should include wallet + api keys.
		mReq := httptest.NewRequest(http.MethodGet, "/v1/admin/merchants/"+merchantID, nil)
		mReq.AddCookie(adminCookie)
		mRec := httptest.NewRecorder()
		s.Handler().ServeHTTP(mRec, mReq)
		if mRec.Code != http.StatusOK {
			t.Fatalf("get merchant: expected 200, got %d: %s", mRec.Code, mRec.Body.String())
		}
		var mResp struct {
			Status string `json:"status"`
			Data   struct {
				MerchantID string `json:"merchant_id"`
				Wallet     *struct {
					WalletID string `json:"wallet_id"`
					UFVK     string `json:"ufvk"`
				} `json:"wallet"`
				APIKeys []struct {
					KeyID     string  `json:"key_id"`
					Label     string  `json:"label"`
					RevokedAt *string `json:"revoked_at"`
				} `json:"api_keys"`
			} `json:"data"`
		}
		if err := json.Unmarshal(mRec.Body.Bytes(), &mResp); err != nil {
			t.Fatalf("unmarshal merchant: %v", err)
		}
		if mResp.Status != "ok" {
			t.Fatalf("merchant: expected status ok")
		}
		if mResp.Data.MerchantID != merchantID {
			t.Fatalf("merchant_id mismatch")
		}
		if mResp.Data.Wallet == nil || mResp.Data.Wallet.WalletID == "" || mResp.Data.Wallet.UFVK == "" {
			t.Fatalf("expected wallet to be present in merchant detail")
		}
		if len(mResp.Data.APIKeys) == 0 {
			t.Fatalf("expected at least 1 api key in merchant detail")
		}
		if mResp.Data.APIKeys[0].KeyID != keyResp.Data.Key.KeyID {
			t.Fatalf("api key id mismatch")
		}
		if mResp.Data.APIKeys[0].Label != "default" {
			t.Fatalf("api key label mismatch")
		}
		if mResp.Data.APIKeys[0].RevokedAt != nil {
			t.Fatalf("expected key to be unrevoked")
		}

		// Invoice creation with merchant API key should work.
		invReqBody := []byte(`{"external_order_id":"order-1","amount_zat":1}`)
		invReq := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(invReqBody))
		invReq.Header.Set("Authorization", "Bearer "+keyResp.Data.APIKey)
	invRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(invRec, invReq)
	if invRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", invRec.Code, invRec.Body.String())
	}

	// Revoke API key.
	revReq := httptest.NewRequest(http.MethodDelete, "/v1/admin/api-keys/"+keyResp.Data.Key.KeyID, nil)
	revReq.AddCookie(adminCookie)
	revRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(revRec, revReq)
		if revRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", revRec.Code, revRec.Body.String())
		}

		// Merchant detail should show revoked_at after revocation.
		mReq2 := httptest.NewRequest(http.MethodGet, "/v1/admin/merchants/"+merchantID, nil)
		mReq2.AddCookie(adminCookie)
		mRec2 := httptest.NewRecorder()
		s.Handler().ServeHTTP(mRec2, mReq2)
		if mRec2.Code != http.StatusOK {
			t.Fatalf("get merchant after revoke: expected 200, got %d: %s", mRec2.Code, mRec2.Body.String())
		}
		var mResp2 struct {
			Data struct {
				APIKeys []struct {
					KeyID     string  `json:"key_id"`
					RevokedAt *string `json:"revoked_at"`
				} `json:"api_keys"`
			} `json:"data"`
		}
		if err := json.Unmarshal(mRec2.Body.Bytes(), &mResp2); err != nil {
			t.Fatalf("unmarshal merchant2: %v", err)
		}
		if len(mResp2.Data.APIKeys) == 0 || mResp2.Data.APIKeys[0].KeyID != keyResp.Data.Key.KeyID || mResp2.Data.APIKeys[0].RevokedAt == nil {
			t.Fatalf("expected revoked_at to be set after revocation")
		}

		// Using the key after revocation should be unauthorized.
		invReq2 := httptest.NewRequest(http.MethodPost, "/v1/invoices", bytes.NewReader(invReqBody))
		invReq2.Header.Set("Authorization", "Bearer "+keyResp.Data.APIKey)
		invRec2 := httptest.NewRecorder()
	s.Handler().ServeHTTP(invRec2, invReq2)
	if invRec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", invRec2.Code, invRec2.Body.String())
	}
}

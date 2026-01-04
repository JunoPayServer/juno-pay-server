package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

type uptimeUnsupportedTip struct{}

func (uptimeUnsupportedTip) BestTip(_ context.Context) (int64, string, error) {
	return 100, "h100", nil
}
func (uptimeUnsupportedTip) UptimeSeconds(_ context.Context) (int64, error) {
	return 0, ErrUptimeUnsupported
}

func TestStatus_UptimeUnsupported_Public(t *testing.T) {
	st := store.NewMem()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := New(st, fakeDeriver{}, uptimeUnsupportedTip{}, fixedClock{t: now}, fixedTokenGen{token: "tok"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Chain struct {
				BestHeight    int64  `json:"best_height"`
				BestHash      string `json:"best_hash"`
				UptimeSeconds *int64 `json:"uptime_seconds"`
			} `json:"chain"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if resp.Data.Chain.BestHeight != 100 || resp.Data.Chain.BestHash != "h100" {
		t.Fatalf("unexpected chain: %+v", resp.Data.Chain)
	}
	if resp.Data.Chain.UptimeSeconds != nil {
		t.Fatalf("expected uptime_seconds null, got %v", *resp.Data.Chain.UptimeSeconds)
	}
}

func TestStatus_UptimeUnsupported_Admin(t *testing.T) {
	st := store.NewMem()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := New(st, fakeDeriver{}, uptimeUnsupportedTip{}, fixedClock{t: now}, fixedTokenGen{token: "tok"}, WithAdminPassword("pw"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	adminCookie := adminLogin(t, s, "pw")

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
	req.AddCookie(adminCookie)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   struct {
			Chain struct {
				BestHeight    int64  `json:"best_height"`
				BestHash      string `json:"best_hash"`
				UptimeSeconds *int64 `json:"uptime_seconds"`
			} `json:"chain"`
			Restarts struct {
				Detected      int64   `json:"junocashd_restarts_detected"`
				LastRestartAt *string `json:"last_restart_at"`
			} `json:"restarts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("expected status ok")
	}
	if resp.Data.Chain.UptimeSeconds != nil {
		t.Fatalf("expected uptime_seconds null, got %v", *resp.Data.Chain.UptimeSeconds)
	}
	if resp.Data.Restarts.Detected != 0 || resp.Data.Restarts.LastRestartAt != nil {
		t.Fatalf("unexpected restarts: %+v", resp.Data.Restarts)
	}
}

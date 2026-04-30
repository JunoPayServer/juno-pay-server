package scanclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_UpsertWallet(t *testing.T) {
	var got walletUpsertRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/wallets" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := c.UpsertWallet(context.Background(), "w1", "jview1test"); err != nil {
		t.Fatalf("UpsertWallet: %v", err)
	}
	if got.WalletID != "w1" || got.UFVK != "jview1test" {
		t.Fatalf("request mismatch: %+v", got)
	}
}

func TestClient_ListWalletEvents(t *testing.T) {
	wantTime := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/v1/wallets/w1/events" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("cursor") != "5" || r.URL.Query().Get("limit") != "2" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": []any{
				map[string]any{
					"id":         6,
					"kind":       "DepositEvent",
					"wallet_id":  "w1",
					"height":     10,
					"payload":    map[string]any{"k": "v"},
					"created_at": wantTime.Format(time.RFC3339Nano),
				},
			},
			"next_cursor": 6,
		})
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	evs, next, err := c.ListWalletEvents(context.Background(), "w1", 5, 2)
	if err != nil {
		t.Fatalf("ListWalletEvents: %v", err)
	}
	if next != 6 {
		t.Fatalf("expected next_cursor 6, got %d", next)
	}
	if len(evs) != 1 || evs[0].ID != 6 || evs[0].Kind != "DepositEvent" || evs[0].WalletID != "w1" {
		t.Fatalf("events mismatch: %+v", evs)
	}
	if !evs[0].CreatedAt.Equal(wantTime) {
		t.Fatalf("created_at mismatch: got %s want %s", evs[0].CreatedAt, wantTime)
	}
}

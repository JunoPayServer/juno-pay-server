//go:build adminui

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/store"
)

func TestWithAdminUI_UsesEmbeddedWhenDirEmpty(t *testing.T) {
	st := store.NewMem()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := New(
		st,
		fakeDeriver{},
		fixedTip{height: 1, hash: "h1"},
		fixedClock{t: now},
		fixedTokenGen{token: "tok"},
		WithAdminPassword("pw"),
		WithAdminUI(""),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}


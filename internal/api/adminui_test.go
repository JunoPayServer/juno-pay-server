package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAdminUI_RedirectKeepsPrefix(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "status"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status", "index.html"), []byte("status"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newAdminUIHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusMovedPermanently)
	}
	if got := res.Header.Get("Location"); got != "/admin/status/" {
		t.Fatalf("Location = %q, want %q", got, "/admin/status/")
	}
}

func TestAdminUI_ServesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("root"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := newAdminUIHandler(dir)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/admin/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	b, _ := io.ReadAll(res.Body)
	if string(b) != "root" {
		t.Fatalf("body = %q, want %q", string(b), "root")
	}
}


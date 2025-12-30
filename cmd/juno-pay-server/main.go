package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/api"
	"github.com/Abdullah1738/juno-pay-server/internal/ingest"
	"github.com/Abdullah1738/juno-pay-server/internal/keys"
	"github.com/Abdullah1738/juno-pay-server/internal/keys/ffi"
	"github.com/Abdullah1738/juno-pay-server/internal/outbox"
	"github.com/Abdullah1738/juno-pay-server/internal/scanclient"
	"github.com/Abdullah1738/juno-pay-server/internal/store/sqlite"
	"github.com/Abdullah1738/juno-sdk-go/junocashd"
)

func main() {
	addr := getenv("JUNO_PAY_ADDR", "127.0.0.1:8080")
	adminPassword := getenv("JUNO_PAY_ADMIN_PASSWORD", "")
	if adminPassword == "" {
		log.Fatalf("missing env: JUNO_PAY_ADMIN_PASSWORD")
	}

	dataDir := getenv("JUNO_PAY_DATA_DIR", defaultDataDir())
	tokenKeyHex := getenv("JUNO_PAY_TOKEN_KEY_HEX", "")
	if tokenKeyHex == "" {
		log.Fatalf("missing env: JUNO_PAY_TOKEN_KEY_HEX")
	}
	tokenKey, err := hex.DecodeString(tokenKeyHex)
	if err != nil || len(tokenKey) != 32 {
		log.Fatalf("invalid JUNO_PAY_TOKEN_KEY_HEX (expected 32-byte hex)")
	}

	st, err := sqlite.Open(dataDir, tokenKey)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := st.Init(ctx); err != nil {
		log.Fatalf("init store: %v", err)
	}

	rpcURL := getenv("JUNO_CASHD_RPC_URL", "http://127.0.0.1:8232")
	rpcUser := getenv("JUNO_CASHD_RPC_USER", "")
	rpcPass := getenv("JUNO_CASHD_RPC_PASS", "")
	jcd := junocashd.New(rpcURL, rpcUser, rpcPass)

	s, err := api.New(st, keysDeriver{d: ffi.New()}, junocashdTip{cli: jcd}, realClock{}, randTokenGen{}, api.WithAdminPassword(adminPassword))
	if err != nil {
		log.Fatalf("init error: %v", err)
	}

	scanURL := getenv("JUNO_SCAN_URL", "")
	if scanURL == "" {
		log.Fatalf("missing env: JUNO_SCAN_URL")
	}
	sc, err := scanclient.New(scanURL)
	if err != nil {
		log.Fatalf("init scan client: %v", err)
	}
	pollInterval := parsePollInterval(getenv("JUNO_PAY_SCAN_POLL_MS", "1000"))
	ing, err := ingest.New(st, sc, pollInterval)
	if err != nil {
		log.Fatalf("init ingestor: %v", err)
	}
	go func() {
		for {
			if err := ing.Sync(context.Background()); err != nil {
				log.Printf("scan sync error: %v", err)
			}
			time.Sleep(pollInterval)
		}
	}()

	outboxPoll := parsePollInterval(getenv("JUNO_PAY_OUTBOX_POLL_MS", "500"))
	outboxBatchSize := parseIntClamp(getenv("JUNO_PAY_OUTBOX_BATCH_SIZE", "100"), 1, 1000)
	outboxMaxAttempts := int32(parseIntClamp(getenv("JUNO_PAY_OUTBOX_MAX_ATTEMPTS", "25"), 1, 1000))
	ob, err := outbox.New(st, outbox.WithBatchSize(outboxBatchSize), outbox.WithMaxAttempts(outboxMaxAttempts))
	if err != nil {
		log.Fatalf("init outbox: %v", err)
	}
	go func() {
		for {
			if err := ob.Sync(context.Background()); err != nil {
				log.Printf("outbox sync error: %v", err)
			}
			time.Sleep(outboxPoll)
		}
	}()

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func defaultDataDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".juno-pay-server")
	}
	return ".juno-pay-server"
}

func parsePollInterval(ms string) time.Duration {
	n, err := strconv.Atoi(ms)
	if err != nil || n <= 0 {
		return time.Second
	}
	if n < 100 {
		n = 100
	}
	if n > 60000 {
		n = 60000
	}
	return time.Duration(n) * time.Millisecond
}

func parseIntClamp(v string, min, max int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return min
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

type keysDeriver struct{ d keys.Deriver }

func (kd keysDeriver) Derive(ufvk string, uaHRP string, index uint32) (string, error) {
	return kd.d.AddressFromUFVK(ufvk, uaHRP, keys.ScopeExternal, index)
}

type junocashdTip struct{ cli *junocashd.Client }

func (t junocashdTip) BestTip(ctx context.Context) (int64, string, error) {
	if t.cli == nil {
		return 0, "", nil
	}
	height, err := t.cli.GetBlockCount(ctx)
	if err != nil {
		return 0, "", err
	}
	hash, err := t.cli.GetBlockHash(ctx, height)
	if err != nil {
		return 0, "", err
	}
	return height, hash, nil
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

type randTokenGen struct{}

func (randTokenGen) NewInvoiceToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "inv_tok_" + hex.EncodeToString(raw[:]), nil
}

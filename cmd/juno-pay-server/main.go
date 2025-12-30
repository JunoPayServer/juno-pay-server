package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Abdullah1738/juno-addrgen/pkg/addrgen"
	"github.com/Abdullah1738/juno-pay-server/internal/api"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-sdk-go/junocashd"
)

func main() {
	addr := getenv("JUNO_PAY_ADDR", "127.0.0.1:8080")

	st := store.NewMem()

	rpcURL := getenv("JUNO_CASHD_RPC_URL", "http://127.0.0.1:8232")
	rpcUser := getenv("JUNO_CASHD_RPC_USER", "")
	rpcPass := getenv("JUNO_CASHD_RPC_PASS", "")
	jcd := junocashd.New(rpcURL, rpcUser, rpcPass)

	s, err := api.New(st, addrgenDeriver{}, junocashdTip{cli: jcd}, realClock{}, randTokenGen{})
	if err != nil {
		log.Fatalf("init error: %v", err)
	}

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

type addrgenDeriver struct{}

func (addrgenDeriver) Derive(ufvk string, index uint32) (string, error) {
	return addrgen.Derive(ufvk, index)
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

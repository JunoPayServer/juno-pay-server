package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := getenv("JUNO_PAY_ADDR", "127.0.0.1:8080")

	srv := &http.Server{
		Addr:              addr,
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

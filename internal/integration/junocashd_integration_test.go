//go:build integration

package integration

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/testutil/containers"
)

func TestJunocashdContainer_Starts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	jd, err := containers.StartJunocashd(ctx)
	if err != nil {
		t.Fatalf("StartJunocashd: %v", err)
	}
	defer func() { _ = jd.Terminate(context.Background()) }()

	stdout, _, err := jd.CLI(ctx, "getblockcount")
	if err != nil {
		t.Fatalf("getblockcount: %v", err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(stdout)), 10, 64)
	if err != nil {
		t.Fatalf("parse: %v (%q)", err, string(stdout))
	}
	if n < 0 {
		t.Fatalf("unexpected blockcount: %d", n)
	}
}

//go:build integration

package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JunoPayServer/juno-pay-server/internal/api"
	"github.com/JunoPayServer/juno-pay-server/internal/testutil/containers"
	"github.com/JunoPayServer/juno-sdk-go/junocashd"
)

func TestJunocashdTip_UptimeUnsupported(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	jd, err := containers.StartJunocashd(ctx)
	if err != nil {
		t.Fatalf("StartJunocashd: %v", err)
	}
	defer func() { _ = jd.Terminate(context.Background()) }()

	cli := junocashd.New(jd.RPCURL, jd.RPCUser, jd.RPCPassword)
	tip := junocashdTip{cli: cli}

	_, err = tip.UptimeSeconds(ctx)
	if !errors.Is(err, api.ErrUptimeUnsupported) {
		t.Fatalf("expected ErrUptimeUnsupported, got %v", err)
	}
}

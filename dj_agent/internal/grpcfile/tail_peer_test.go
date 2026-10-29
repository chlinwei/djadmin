//go:build linux

package grpcfile

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTailExternalPeer(t *testing.T) {
	coordinate := os.Getenv("DJADMIN_LIVE_TAIL_E2E_COORD")
	if coordinate == "" {
		t.Skip("opt-in peer for autoadmin TestLiveTailRealAgentEndToEnd")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	address := ""
	for address == "" {
		payload, err := os.ReadFile(coordinate)
		if err == nil {
			address = strings.TrimSpace(string(payload))
		}
		if ctx.Err() != nil {
			t.Fatal("API integration peer did not publish an address")
		}
		if address == "" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	established, _ := runOnce(ctx, address, "host-agent", "e2e-agent-secret", nil, func(bool) {})
	if !established || ctx.Err() != nil {
		t.Fatal("real agent did not complete the integration session")
	}
}

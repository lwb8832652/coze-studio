// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

func TestSafeStdioTransportInboundRequestFloodHasFixedInflightBound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &safeStdioTransport{
		ctx:       ctx,
		cancel:    cancel,
		responses: make(map[string]chan stdioResponseDelivery),
		done:      make(chan struct{}),
		inbound:   newStdioInboundLimiter(),
	}
	pending := make(chan stdioResponseDelivery, 1)
	transport.responses["pending"] = pending
	release := make(chan struct{})
	var active atomic.Int32
	var maximum atomic.Int32
	transport.SetRequestHandler(func(context.Context, mcptransport.JSONRPCRequest) (*mcptransport.JSONRPCResponse, error) {
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		<-release
		active.Add(-1)
		return nil, nil
	})

	for index := 0; index < defaultStdioInboundMaxInflight+128; index++ {
		transport.handleIncomingRequest(mcptransport.JSONRPCRequest{
			JSONRPC: mcpsdk.JSONRPC_VERSION,
			ID:      mcpsdk.NewRequestId(int64(index + 1)),
			Method:  "roots/list",
		})
	}
	select {
	case delivery := <-pending:
		if !errors.Is(delivery.err, ErrStdioInboundRequestLimitExceeded) {
			t.Fatalf("pending request received %v", delivery.err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request was not woken after inbound flood")
	}
	if got := transport.inbound.inflightCount(); got > defaultStdioInboundMaxInflight {
		t.Fatalf("inflight handlers = %d", got)
	}
	close(release)
	if !transport.inbound.wait(time.Second) {
		t.Fatal("inbound handlers did not release")
	}
	if got := maximum.Load(); got > int32(defaultStdioInboundMaxInflight) {
		t.Fatalf("maximum concurrent handlers = %d", got)
	}
}

func TestStdioInboundLimiterEnforcesSessionTotalAndRateBudgets(t *testing.T) {
	now := time.Unix(100, 0)
	t.Run("rate", func(t *testing.T) {
		limiter := newStdioInboundLimiter()
		for index := 0; index < defaultStdioInboundMaxPerWindow; index++ {
			if !limiter.tryAcquire(now) {
				t.Fatalf("rate budget rejected request %d", index)
			}
			limiter.release()
		}
		if limiter.tryAcquire(now) {
			t.Fatal("rate budget accepted one request too many")
		}
	})

	t.Run("session total", func(t *testing.T) {
		limiter := newStdioInboundLimiter()
		for index := 0; index < defaultStdioInboundMaxTotal; index++ {
			requestTime := now.Add(time.Duration(index) * (defaultStdioInboundRateWindow + time.Nanosecond))
			if !limiter.tryAcquire(requestTime) {
				t.Fatalf("session budget rejected request %d", index)
			}
			limiter.release()
		}
		if limiter.tryAcquire(now.Add(time.Duration(defaultStdioInboundMaxTotal+1) * defaultStdioInboundRateWindow)) {
			t.Fatal("session total accepted one request too many")
		}
	})
}

func TestSafeStdioTransportCloseWaitsForInboundHandlersOnlyWithinBound(t *testing.T) {
	transport := &safeStdioTransport{
		responses:        make(map[string]chan stdioResponseDelivery),
		done:             make(chan struct{}),
		readDone:         make(chan struct{}),
		drainDone:        make(chan struct{}),
		inbound:          newStdioInboundLimiter(),
		inboundCloseWait: 20 * time.Millisecond,
	}
	if !transport.inbound.tryAcquire(time.Now()) {
		t.Fatal("failed to acquire inbound test slot")
	}
	started := time.Now()
	done := make(chan struct{})
	for index := 0; index < 8; index++ {
		go func() {
			_ = transport.Close()
			done <- struct{}{}
		}()
	}
	for index := 0; index < 8; index++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("concurrent Close exceeded bound")
		}
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Close waited too long: %v", elapsed)
	}
	transport.inbound.release()
}

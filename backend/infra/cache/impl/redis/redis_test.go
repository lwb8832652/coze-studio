// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package redis

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func cleanupTestClient(t *testing.T, client Cmdable) {
	t.Helper()
	impl, ok := client.(*redisImpl)
	if !ok {
		t.Fatalf("redis client has type %T, want *redisImpl", client)
	}
	t.Cleanup(func() {
		if err := impl.client.Close(); err != nil {
			t.Errorf("close redis client: %v", err)
		}
	})
}

type silentTCPServer struct {
	listener net.Listener
	done     chan struct{}

	mu          sync.Mutex
	connections []net.Conn
	accepted    int
	closed      bool
	closeOnce   sync.Once
}

func newSilentTCPServer(t *testing.T) *silentTCPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start silent TCP server: %v", err)
	}
	server := &silentTCPServer{
		listener: listener,
		done:     make(chan struct{}),
	}
	go server.acceptConnections()
	t.Cleanup(server.Close)
	return server
}

func (s *silentTCPServer) acceptConnections() {
	defer close(s.done)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.accepted++
		if s.closed {
			_ = conn.Close()
		} else {
			s.connections = append(s.connections, conn)
		}
		s.mu.Unlock()
	}
}

func (s *silentTCPServer) Addr() string {
	return s.listener.Addr().String()
}

func (s *silentTCPServer) AcceptedConnections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted
}

func (s *silentTCPServer) Close() {
	s.closeOnce.Do(func() {
		_ = s.listener.Close()
		s.mu.Lock()
		s.closed = true
		connections := append([]net.Conn(nil), s.connections...)
		s.connections = nil
		s.mu.Unlock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		<-s.done
	})
}

func TestRedisRunScriptUsesKeysAndArguments(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	client := NewWithAddrAndPassword(server.Addr(), "")
	cleanupTestClient(t, client)
	script := `return {KEYS[1], ARGV[1]}`

	for _, value := range []string{"first", "second"} {
		result, err := client.RunScript(context.Background(), script, []string{"script-key"}, value).Result()
		if err != nil {
			t.Fatalf("run script for %q: %v", value, err)
		}
		items, ok := result.([]interface{})
		if !ok || len(items) != 2 {
			t.Fatalf("unexpected script result %#v", result)
		}
		if items[0] != "script-key" || items[1] != value {
			t.Fatalf("unexpected keys/arguments result %#v", items)
		}
	}
}

func TestRedisRunScriptPropagatesCancellation(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	client := NewWithAddrAndPassword(server.Addr(), "")
	cleanupTestClient(t, client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.RunScript(ctx, `return 1`, nil).Result()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestRedisRunScriptPipelineExecutesWithEmptyScriptCache(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	client := NewWithAddrAndPassword(server.Addr(), "")
	cleanupTestClient(t, client)
	pipeline := client.Pipeline()
	command := pipeline.RunScript(
		context.Background(),
		`return {KEYS[1], ARGV[1]}`,
		[]string{"pipeline-key"},
		"pipeline-value",
	)
	if _, err := pipeline.Exec(context.Background()); err != nil {
		t.Fatalf("execute pipeline against empty script cache: %v", err)
	}
	result, err := command.Result()
	if err != nil {
		t.Fatalf("read pipelined script result: %v", err)
	}
	items, ok := result.([]interface{})
	if !ok || len(items) != 2 || items[0] != "pipeline-key" || items[1] != "pipeline-value" {
		t.Fatalf("unexpected pipeline script result %#v", result)
	}
}

func TestParseRedisDB(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "missing", raw: "", want: 0},
		{name: "blank", raw: "   ", want: 0},
		{name: "zero", raw: "0", want: 0},
		{name: "selected", raw: " 2 ", want: 2},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "plus sign", raw: "+1", wantErr: true},
		{name: "decimal", raw: "1.5", wantErr: true},
		{name: "non digit", raw: "1a", wantErr: true},
		{name: "overflow", raw: strings.Repeat("9", 100), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRedisDB(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRedisDB(%q) unexpectedly succeeded", tt.raw)
				}
				if !strings.Contains(err.Error(), "REDIS_DB") {
					t.Fatalf("parseRedisDB(%q) error = %v, want REDIS_DB", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRedisDB(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseRedisDB(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNewUsesConfiguredRedisDB(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	t.Setenv("REDIS_ADDR", server.Addr())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "2")

	client, err := New(context.Background())
	if err != nil {
		t.Fatalf("initialize redis: %v", err)
	}
	cleanupTestClient(t, client)
	if err := client.Set(context.Background(), "selected-db", "ok", 0).Err(); err != nil {
		t.Fatalf("write selected database: %v", err)
	}
	if _, err := server.DB(0).Get("selected-db"); !errors.Is(err, miniredis.ErrKeyNotFound) {
		t.Fatalf("DB 0 unexpectedly contains selected-db: %v", err)
	}
	got, err := server.DB(2).Get("selected-db")
	if err != nil || got != "ok" {
		t.Fatalf("DB 2 selected-db = %q, %v", got, err)
	}
}

func TestNewPropagatesReadinessFailure(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	server.SetError("ERR selected database unavailable")
	t.Setenv("REDIS_ADDR", server.Addr())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "3")

	_, err = New(context.Background())
	if err == nil || !strings.Contains(err.Error(), "redis readiness check failed") {
		t.Fatalf("New() readiness error = %v", err)
	}
}

func TestNewHonorsParentDeadlineDuringReadiness(t *testing.T) {
	server := newSilentTCPServer(t)
	t.Setenv("REDIS_ADDR", server.Addr())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "0")

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	type newResult struct {
		client Cmdable
		err    error
	}
	resultCh := make(chan newResult, 1)
	go func() {
		client, err := New(ctx)
		resultCh <- newResult{client: client, err: err}
	}()

	const maxWait = time.Second
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	var result newResult
	select {
	case result = <-resultCh:
	case <-timer.C:
		elapsed := time.Since(started)
		server.Close()
		result = <-resultCh
		if result.client != nil {
			cleanupTestClient(t, result.client)
		}
		t.Fatalf("New did not honor the parent deadline; still blocked after %s", elapsed)
	}

	elapsed := time.Since(started)
	if result.client != nil {
		cleanupTestClient(t, result.client)
		t.Fatalf("New returned an unexpected client after %s", elapsed)
	}
	if result.err == nil {
		t.Fatalf("New unexpectedly succeeded after %s", elapsed)
	}
	if !strings.Contains(result.err.Error(), "redis readiness check failed") {
		t.Fatalf("New readiness error = %v", result.err)
	}
	if !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("New readiness error = %v, want context deadline exceeded", result.err)
	}
	if server.AcceptedConnections() == 0 {
		t.Fatal("silent TCP server accepted no connections")
	}
	if elapsed >= maxWait {
		t.Fatalf("New returned after %s, want less than %s", elapsed, maxWait)
	}
	t.Logf("New honored the parent deadline in %s", elapsed)
}

func TestNewRejectsNilContext(t *testing.T) {
	_, err := New(nil)
	if err == nil || !strings.Contains(err.Error(), "redis initialization context is nil") {
		t.Fatalf("New(nil) error = %v", err)
	}
}

func TestNewRejectsInvalidRedisDBBeforeDial(t *testing.T) {
	t.Setenv("REDIS_ADDR", "unused.invalid:6379")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "not-a-number")

	_, err := New(context.Background())
	if err == nil || !strings.Contains(err.Error(), "REDIS_DB") {
		t.Fatalf("New() invalid REDIS_DB error = %v", err)
	}
}

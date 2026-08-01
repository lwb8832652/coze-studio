// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package redis

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestRedisRunScriptUsesKeysAndArguments(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)

	client := NewWithAddrAndPassword(server.Addr(), "")
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

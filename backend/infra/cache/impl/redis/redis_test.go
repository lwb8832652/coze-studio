// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package redis

import (
	"context"
	"errors"
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

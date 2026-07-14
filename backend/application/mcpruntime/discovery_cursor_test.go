// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDiscoveryRejectsOversizedCursorForEveryCapability(t *testing.T) {
	largeCursor := strings.Repeat("c", 33)
	limits := DiscoveryLimits{
		MaxItems: 10, MaxBytes: 4096, MaxPageBytes: 4096, MaxPages: 4, MaxCursorBytes: 32,
	}
	tests := []struct {
		name    string
		session *discoverySession
	}{
		{
			name: "tools",
			session: &discoverySession{
				capabilities: CapabilityFlags{Tools: true},
				toolPages:    map[string]ToolPage{"": {NextCursor: largeCursor}},
			},
		},
		{
			name: "resources",
			session: &discoverySession{
				capabilities:  CapabilityFlags{Resources: true},
				resourcePages: map[string]ResourcePage{"": {NextCursor: largeCursor}},
			},
		},
		{
			name: "prompts",
			session: &discoverySession{
				capabilities: CapabilityFlags{Prompts: true},
				promptPages:  map[string]PromptPage{"": {NextCursor: largeCursor}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Discover(context.Background(), test.session, limits)
			if !errors.Is(err, ErrDiscoveryLimitExceeded) {
				t.Fatalf("oversized cursor must fail closed, got %v", err)
			}
		})
	}
}

func TestDiscoveryCountsCursorInPageAndTotalByteBudgets(t *testing.T) {
	t.Run("page bytes", func(t *testing.T) {
		session := &discoverySession{
			capabilities: CapabilityFlags{Tools: true},
			toolPages:    map[string]ToolPage{"": {NextCursor: strings.Repeat("p", 17)}},
		}
		_, err := Discover(context.Background(), session, DiscoveryLimits{
			MaxItems: 10, MaxBytes: 4096, MaxPageBytes: 16, MaxPages: 4, MaxCursorBytes: 64,
		})
		if !errors.Is(err, ErrDiscoveryLimitExceeded) {
			t.Fatalf("cursor must count toward page bytes, got %v", err)
		}
	})

	t.Run("128 unique cursors", func(t *testing.T) {
		pages := make(map[string]ToolPage, 129)
		cursor := ""
		for index := 0; index < 128; index++ {
			next := fmt.Sprintf("cursor-%03d", index)
			pages[cursor] = ToolPage{NextCursor: next}
			cursor = next
		}
		pages[cursor] = ToolPage{}
		session := &discoverySession{
			capabilities: CapabilityFlags{Tools: true},
			toolPages:    pages,
		}
		_, err := Discover(context.Background(), session, DiscoveryLimits{
			MaxItems: 10, MaxBytes: 512, MaxPageBytes: 64, MaxPages: 256, MaxCursorBytes: 32,
		})
		if !errors.Is(err, ErrDiscoveryLimitExceeded) {
			t.Fatalf("unique cursor bytes must exhaust total budget, got %v", err)
		}
		if len(session.toolCursors) >= 128 {
			t.Fatalf("cursor budget did not stop pagination early: calls=%d", len(session.toolCursors))
		}
	})
}

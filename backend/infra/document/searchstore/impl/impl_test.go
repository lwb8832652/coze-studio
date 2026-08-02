// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package impl

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestGetVectorStoreUsesNoopWhenDisabled(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		missing bool
	}{
		{name: "missing", missing: true},
		{name: "blank", value: "   "},
		{name: "none", value: "none"},
		{name: "noop", value: "noop"},
		{name: "disabled", value: "disabled"},
		{name: "normalized disabled", value: "  DiSaBlEd\t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COZE_DEBUG_SKIP_VECTOR_STORE", "false")
			t.Setenv("VECTOR_STORE_TYPE", tt.value)
			if tt.missing {
				if err := os.Unsetenv("VECTOR_STORE_TYPE"); err != nil {
					t.Fatalf("unset VECTOR_STORE_TYPE: %v", err)
				}
			}

			// A nil config ensures disabled modes return before provider initialization.
			manager, err := getVectorStore(context.Background(), nil)
			if err != nil {
				t.Fatalf("getVectorStore(%q): %v", tt.value, err)
			}
			if _, ok := manager.(noopVectorManager); !ok {
				t.Fatalf("getVectorStore(%q) returned %T, want noopVectorManager", tt.value, manager)
			}
		})
	}
}

func TestGetVectorStoreRejectsUnknownType(t *testing.T) {
	t.Setenv("COZE_DEBUG_SKIP_VECTOR_STORE", "false")
	t.Setenv("VECTOR_STORE_TYPE", "unknown-provider")
	_, err := getVectorStore(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected vector store type") {
		t.Fatalf("unknown vector store error = %v", err)
	}
}

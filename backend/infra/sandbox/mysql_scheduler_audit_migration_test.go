// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMySQLSchedulerAuditMigrationDefinesGlobalSafeTable(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docker", "atlas", "migrations", "20260811000200_sandbox_scheduler_audit.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scheduler audit migration: %v", err)
	}
	sql := string(raw)
	match := regexp.MustCompile("(?s)CREATE TABLE `sandbox_scheduler_audit_events` \\(.*?\\) ENGINE=").FindString(sql)
	if match == "" {
		t.Fatal("scheduler audit migration does not define the independent table")
	}
	columns := regexp.MustCompile("(?m)^  `([^`]+)` ").FindAllStringSubmatch(match, -1)
	want := map[string]bool{"event_id": true, "actor_user_id": true, "request_id": true, "action": true, "metadata_json": true, "created_at": true}
	if len(columns) != len(want) {
		t.Fatal("scheduler audit migration column count is not the safe global contract")
	}
	for _, column := range columns {
		if !want[column[1]] {
			t.Fatal("scheduler audit migration includes a forbidden column")
		}
	}
	if strings.Contains(match, "provider_id") || !strings.Contains(match, "`action` IN ('scheduler_settings.update', 'scheduler_settings.update_failed')") {
		t.Fatal("scheduler audit migration is not global or has an unsafe action contract")
	}
}

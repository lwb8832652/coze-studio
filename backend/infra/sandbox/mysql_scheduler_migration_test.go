// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSandboxRunnerSchedulerMigrationInitializesExactContracts(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "docker", "atlas", "migrations", "20260811000100_sandbox_runner_scheduler.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scheduler migration: %v", err)
	}
	statements := splitSchedulerMigrationStatements(string(content))
	if len(statements) != 5 {
		t.Fatalf("scheduler migration statements = %d, want 5", len(statements))
	}
	assertSchedulerProviderFeatureMigration(t, statements[:3])
	assertSchedulerSingletonMigration(t, statements[3], statements[4])
}

func splitSchedulerMigrationStatements(sql string) []string {
	parts := strings.Split(sql, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		if statement := strings.TrimSpace(part); statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}

func assertSchedulerProviderFeatureMigration(t *testing.T, statements []string) {
	t.Helper()
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+` + "`sandbox_providers`" + `\s+ADD\s+COLUMN\s+` + "`last_health_features_json`" + `\s+JSON\s+NULL\s+AFTER\s+` + "`last_health_capabilities_json`" + `$`),
		regexp.MustCompile(`(?is)^UPDATE\s+` + "`sandbox_providers`" + `\s+SET\s+` + "`last_health_features_json`" + `\s*=\s*JSON_ARRAY\(\)\s+WHERE\s+` + "`last_health_features_json`" + `\s+IS\s+NULL$`),
		regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+` + "`sandbox_providers`" + `\s+MODIFY\s+COLUMN\s+` + "`last_health_features_json`" + `\s+JSON\s+NOT\s+NULL$`),
	}
	for index, pattern := range patterns {
		if !pattern.MatchString(statements[index]) {
			t.Fatalf("provider feature migration statement %d has wrong schema transition", index+1)
		}
	}
}

func assertSchedulerSingletonMigration(t *testing.T, createStatement, insertStatement string) {
	t.Helper()
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+` + "`sandbox_scheduler_settings`" + `\s*\(`),
		regexp.MustCompile("(?is)`id`\\s+TINYINT\\s+UNSIGNED\\s+NOT\\s+NULL"),
		regexp.MustCompile("(?is)`settings_json`\\s+JSON\\s+NOT\\s+NULL"),
		regexp.MustCompile("(?is)`version`\\s+BIGINT\\s+UNSIGNED\\s+NOT\\s+NULL"),
		regexp.MustCompile("(?is)`updated_by`\\s+BIGINT\\s+UNSIGNED\\s+NOT\\s+NULL"),
		regexp.MustCompile("(?is)PRIMARY\\s+KEY\\s*\\(`id`\\)"),
		regexp.MustCompile("(?is)CHECK\\s*\\(`id`\\s*=\\s*1\\)"),
	} {
		if !pattern.MatchString(createStatement) {
			t.Fatal("scheduler singleton CREATE TABLE contract is incomplete")
		}
	}
	match := regexp.MustCompile(`(?is)^INSERT\s+INTO\s+` + "`sandbox_scheduler_settings`" + `\s*\([^)]*\)\s*VALUES\s*\(\s*1\s*,\s*'([^']*)'\s*,\s*1\s*,\s*0\s*,\s*UTC_TIMESTAMP\(3\)\s*,\s*UTC_TIMESTAMP\(3\)\s*\)$`).FindStringSubmatch(insertStatement)
	if len(match) != 2 {
		t.Fatal("scheduler singleton INSERT does not use id=1/version=1/updated_by=0 with JSON settings")
	}
	decoded, err := domainsandbox.DecodeSchedulerSettingsJSON([]byte(match[1]))
	if err != nil {
		t.Fatalf("decode migration scheduler settings JSON: %v", err)
	}
	if !reflect.DeepEqual(decoded, domainsandbox.DefaultSchedulerSettings()) {
		t.Fatal("migration scheduler settings JSON is not the exact 2C4G default")
	}
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const sharedAIOMigrationName = "20260814000100_expand_sandbox_shared_aio_core.sql"

func TestSharedAIOMigrationIsAdditiveAndDefinesExactRuntimeContract(t *testing.T) {
	repoRoot := sharedAIORepoRoot(t)
	migration := readSharedAIOFile(t, filepath.Join(repoRoot, "docker", "atlas", "migrations", sharedAIOMigrationName))
	schema := readSharedAIOFile(t, filepath.Join(repoRoot, "docker", "atlas", "opencoze_latest_schema.hcl"))

	assertSharedAIOMigrationSafety(t, migration)
	assertSharedAIOSchedulerColumns(t, migration, schema)
	assertSharedAIORuntimeSessionTable(t, migration, schema)
}

func sharedAIORepoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve shared AIO migration test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}

func readSharedAIOFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	return string(content)
}

func assertSharedAIOMigrationSafety(t *testing.T, migration string) {
	t.Helper()
	upper := strings.ToUpper(migration)
	for _, forbidden := range []string{"DROP ", "TRUNCATE ", "RENAME ", "DELETE ", "MODIFY ", "AUTO_INCREMENT ="} {
		if strings.Contains(upper, forbidden) {
			t.Fatalf("shared AIO migration contains destructive operation %q", strings.TrimSpace(forbidden))
		}
	}
	if count := regexp.MustCompile(`(?i)\bCREATE\s+TABLE\b`).FindAllStringIndex(migration, -1); len(count) != 1 {
		t.Fatalf("shared AIO migration creates %d tables, want exactly one", len(count))
	}
	if !regexp.MustCompile("(?is)CREATE\\s+TABLE\\s+`sandbox_runtime_sessions`\\s*\\(").MatchString(migration) {
		t.Fatal("shared AIO migration does not create sandbox_runtime_sessions")
	}
	for _, forbidden := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bsandbox_runtime_service_leases\b`),
		regexp.MustCompile(`(?i)\bthread_key\b`),
		regexp.MustCompile(`(?i)\bworkspace_key\b`),
		regexp.MustCompile(`(?i)\bidentity_id\b`),
		regexp.MustCompile("(?i)`(?:uid|gid|path|physical_path|workspace_path)`"),
	} {
		if forbidden.MatchString(migration) {
			t.Fatalf("shared AIO migration contains forbidden Phase 1 contract %q", forbidden.String())
		}
	}
}

func assertSharedAIOSchedulerColumns(t *testing.T, migration, schema string) {
	t.Helper()
	alterMatch := regexp.MustCompile("(?is)ALTER\\s+TABLE\\s+`sandbox_scheduler_settings`(?P<body>.*?);").FindStringSubmatch(migration)
	if len(alterMatch) != 2 {
		t.Fatal("shared AIO migration must add scheduler columns in one ALTER TABLE")
	}
	alter := alterMatch[1]
	wantedColumns := []string{
		"session_settings_json",
		"session_settings_version",
		"session_settings_updated_by",
		"session_settings_updated_at",
		"aio_runtime_generation",
		"aio_runtime_deployment_id",
		"aio_runtime_sentinel_id",
	}
	if got := len(regexp.MustCompile(`(?i)\bADD\s+COLUMN\b`).FindAllStringIndex(alter, -1)); got != len(wantedColumns) {
		t.Fatalf("scheduler ALTER adds %d columns, want %d", got, len(wantedColumns))
	}
	for _, column := range wantedColumns {
		if !regexp.MustCompile("(?i)ADD\\s+COLUMN\\s+`" + regexp.QuoteMeta(column) + "`").MatchString(alter) {
			t.Fatalf("scheduler ALTER is missing %s", column)
		}
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile("(?is)`session_settings_json`\\s+JSON\\s+NULL"),
		regexp.MustCompile("(?is)`session_settings_version`\\s+BIGINT\\s+UNSIGNED\\s+NOT\\s+NULL\\s+DEFAULT\\s+1"),
		regexp.MustCompile("(?is)`session_settings_updated_by`\\s+BIGINT\\s+UNSIGNED\\s+NOT\\s+NULL\\s+DEFAULT\\s+0"),
		regexp.MustCompile("(?is)`session_settings_updated_at`\\s+DATETIME\\(3\\)\\s+NULL"),
		regexp.MustCompile("(?is)`aio_runtime_generation`\\s+BIGINT\\s+UNSIGNED\\s+NOT\\s+NULL\\s+DEFAULT\\s+0"),
		regexp.MustCompile("(?is)`aio_runtime_deployment_id`\\s+VARCHAR\\(128\\).*?NOT\\s+NULL\\s+DEFAULT\\s+''"),
		regexp.MustCompile("(?is)`aio_runtime_sentinel_id`\\s+VARCHAR\\(128\\).*?NOT\\s+NULL\\s+DEFAULT\\s+''"),
		regexp.MustCompile("(?is)UPDATE\\s+`sandbox_scheduler_settings`\\s+SET\\s+`session_settings_json`\\s*=\\s*'[^']+'\\s*,\\s*`session_settings_updated_at`\\s*=\\s*UTC_TIMESTAMP\\(3\\)\\s+WHERE\\s+`id`\\s*=\\s*1"),
	} {
		if !pattern.MatchString(migration) {
			t.Fatalf("shared AIO scheduler migration contract is missing %q", pattern.String())
		}
	}

	jsonMatch := regexp.MustCompile("(?is)SET\\s+`session_settings_json`\\s*=\\s*'([^']+)'").FindStringSubmatch(migration)
	if len(jsonMatch) != 2 {
		t.Fatal("shared AIO migration does not backfill canonical session settings JSON")
	}
	var actual map[string]any
	if err := json.Unmarshal([]byte(jsonMatch[1]), &actual); err != nil {
		t.Fatalf("decode shared AIO session settings JSON: %v", err)
	}
	want := map[string]any{
		"core_enabled": false, "interactive_enabled": false, "host_shell_enabled": false,
		"core_weight": float64(1), "heavy_weight": float64(2), "per_user_active_limit": float64(1),
		"idle_session_limit": float64(20), "idle_shell_limit": float64(4),
		"session_idle_ttl_seconds": float64(1200), "shell_idle_ttl_seconds": float64(300),
		"command_timeout_seconds": float64(600), "cancel_grace_seconds": float64(5),
		"workspace_quota_mb": float64(2048),
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("shared AIO migration session settings = %#v, want exact Phase 1 defaults", actual)
	}

	schedulerBlock := sharedAIOHCLTableBlock(t, schema, "sandbox_scheduler_settings")
	if !regexp.MustCompile(`(?s)column "session_settings_json"\s*\{.*?null\s*=\s*true.*?type\s*=\s*json.*?\}`).MatchString(schedulerBlock) {
		t.Fatal("Atlas scheduler schema must keep session_settings_json nullable during the expand phase")
	}
	for _, column := range wantedColumns {
		if !strings.Contains(schedulerBlock, `column "`+column+`"`) {
			t.Fatalf("Atlas scheduler schema is missing %s", column)
		}
	}
	for _, column := range []string{"aio_runtime_deployment_id", "aio_runtime_sentinel_id"} {
		assertSharedAIOHCLASCIIColumn(t, schedulerBlock, column)
	}
}

func assertSharedAIORuntimeSessionTable(t *testing.T, migration, schema string) {
	t.Helper()
	createMatch := regexp.MustCompile("(?is)CREATE\\s+TABLE\\s+`sandbox_runtime_sessions`\\s*\\((?P<body>.*?)\\)\\s*ENGINE=InnoDB").FindStringSubmatch(migration)
	if len(createMatch) != 2 {
		t.Fatal("shared AIO runtime session CREATE TABLE is missing or malformed")
	}
	create := createMatch[1]
	patterns := []*regexp.Regexp{
		regexp.MustCompile("(?i)`session_id`\\s+CHAR\\(36\\).*?NOT\\s+NULL"),
		regexp.MustCompile("(?i)PRIMARY\\s+KEY\\s*\\(`session_id`\\)"),
		regexp.MustCompile("(?i)UNIQUE\\s+KEY\\s+`uk_sandbox_runtime_session_business`\\s*\\(`deployment_id`,\\s*`provider_id`,\\s*`space_id`,\\s*`user_id`,\\s*`thread_id`,\\s*`profile`\\)"),
		regexp.MustCompile("(?i)KEY\\s+`idx_sandbox_runtime_session_provider`\\s*\\(`provider_id`\\)"),
		regexp.MustCompile("(?i)KEY\\s+`idx_sandbox_runtime_session_recovery`\\s*\\(`deployment_id`,\\s*`runtime_generation`,\\s*`state`,\\s*`session_id`\\)"),
		regexp.MustCompile("(?i)KEY\\s+`idx_sandbox_runtime_session_expiry`\\s*\\(`deployment_id`,\\s*`state`,\\s*`expires_at`,\\s*`session_id`\\)"),
		regexp.MustCompile("(?i)FOREIGN\\s+KEY\\s*\\(`provider_id`\\)\\s+REFERENCES\\s+`sandbox_providers`\\s*\\(`id`\\)"),
		regexp.MustCompile("(?i)CHECK\\s*\\(`profile`\\s+IN\\s*\\('core',\\s*'interactive'\\)\\)"),
		regexp.MustCompile("(?i)CHECK\\s*\\(`state`\\s+IN\\s*\\('active',\\s*'recovering',\\s*'released',\\s*'destroyed'\\)\\)"),
		regexp.MustCompile("(?i)CHECK\\s*\\(`runtime_generation`\\s*>\\s*0\\)"),
		regexp.MustCompile("(?i)CHECK\\s*\\(`version`\\s*>\\s*0\\)"),
	}
	for _, pattern := range patterns {
		if !pattern.MatchString(create) {
			t.Fatalf("shared AIO runtime session table is missing %q", pattern.String())
		}
	}
	for _, column := range []string{
		"deployment_id", "provider_id", "space_id", "user_id", "thread_id", "profile", "state",
		"runtime_generation", "upstream_shell_id", "recovery_reason", "version", "last_activity_at",
		"expires_at", "created_at", "updated_at",
	} {
		if !strings.Contains(create, "`"+column+"`") {
			t.Fatalf("shared AIO runtime session table is missing %s", column)
		}
	}

	block := sharedAIOHCLTableBlock(t, schema, "sandbox_runtime_sessions")
	for _, fragment := range []string{
		`column "session_id"`, `char(36)`, `column "runtime_generation"`,
		`index "uk_sandbox_runtime_session_business"`, `index "idx_sandbox_runtime_session_provider"`,
		`index "idx_sandbox_runtime_session_recovery"`, `index "idx_sandbox_runtime_session_expiry"`,
		`foreign_key "fk_sandbox_runtime_session_provider"`,
		`check "chk_sandbox_runtime_session_profile"`, `check "chk_sandbox_runtime_session_state"`,
		`check "chk_sandbox_runtime_session_generation"`, `check "chk_sandbox_runtime_session_version"`,
	} {
		if !strings.Contains(block, fragment) {
			t.Fatalf("Atlas runtime session schema is missing %q", fragment)
		}
	}
	for _, column := range []string{
		"session_id", "deployment_id", "thread_id", "profile", "state", "upstream_shell_id", "recovery_reason",
	} {
		assertSharedAIOHCLASCIIColumn(t, block, column)
	}
}

func assertSharedAIOHCLASCIIColumn(t *testing.T, tableBlock, column string) {
	t.Helper()
	pattern := regexp.MustCompile(`(?s)column "` + regexp.QuoteMeta(column) + `"\s*\{.*?charset\s*=\s*"ascii".*?collate\s*=\s*"ascii_bin".*?\}`)
	if !pattern.MatchString(tableBlock) {
		t.Fatalf("Atlas schema column %s does not preserve ascii/ascii_bin semantics", column)
	}
}

func sharedAIOHCLTableBlock(t *testing.T, schema, table string) string {
	t.Helper()
	start := strings.Index(schema, `table "`+table+`" {`)
	if start < 0 {
		t.Fatalf("Atlas schema is missing table %s", table)
	}
	depth := 0
	for index := start; index < len(schema); index++ {
		switch schema[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return schema[start : index+1]
			}
		}
	}
	t.Fatalf("Atlas table %s has an unclosed block", table)
	return ""
}

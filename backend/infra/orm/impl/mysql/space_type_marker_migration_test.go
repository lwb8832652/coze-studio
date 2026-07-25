/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mysql

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSpaceTypeMarkerMigrationUsesCurrentSchema(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	migrationPath := filepath.Join(
		filepath.Dir(filename),
		"..", "..", "..", "..", "..",
		"docker", "atlas", "migrations", "20260724000200_space_type_marker.sql",
	)
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)

	if strings.Contains(strings.ToLower(sql), "`opencoze`") {
		t.Fatal("migration must not bind the space table to a deployment-specific schema")
	}
	for _, required := range []string{
		"`information_schema`.`COLUMNS`",
		"`TABLE_SCHEMA` = DATABASE()",
		"`TABLE_NAME` = 'space'",
		"`COLUMN_NAME` = 'space_type'",
		"ALTER TABLE `space`",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration is missing current-schema guard %q", required)
		}
	}
}

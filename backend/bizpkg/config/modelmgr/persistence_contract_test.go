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

package modelmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelManagementMigrationDeclaresProductionSchema(t *testing.T) {
	migrationPath := filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260722000100_system_model_management.sql",
	)
	migration, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	source := string(migration)
	for _, required := range []string{
		"ALTER TABLE `model_instance`",
		"ADD COLUMN `status`",
		"ADD COLUMN `sort_order`",
		"ADD COLUMN `creator_id`",
		"ADD COLUMN `protocol`",
		"ADD COLUMN `routing_strategy`",
		"ADD COLUMN `access_mode`",
		"ADD COLUMN `scenario_json`",
		"CREATE TABLE `model_instance_endpoint`",
		"`api_key_envelope`",
		"`api_key_fingerprint`",
		"CREATE TABLE `model_instance_grant`",
		"UNIQUE KEY `uniq_model_subject`",
	} {
		require.Contains(t, source, required)
	}

	require.NotContains(t, strings.ToLower(source), "json_extract(`connection`")
	require.NotContains(t, strings.ToLower(source), "$.base_conn_info.api_key")
	require.Equal(t, 2, strings.Count(source, "`id` BIGINT NOT NULL AUTO_INCREMENT"))
}

func TestModelManagementMigrationKeepsHistoricalRowsAvailable(t *testing.T) {
	migrationPath := filepath.Join(
		"..", "..", "..", "..", "docker", "atlas", "migrations",
		"20260722000100_system_model_management.sql",
	)
	migration, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	source := string(migration)
	require.Contains(t, source, "DEFAULT 1 COMMENT '1 enabled, 0 disabled'")
	require.Contains(t, source, "DEFAULT 'all'")
	require.Contains(t, source, "DEFAULT 'round_robin'")
	require.Contains(t, source, "UPDATE `model_instance` SET `sort_order` = `id`")
}

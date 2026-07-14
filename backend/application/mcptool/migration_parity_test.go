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

package mcptool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mcpMigrationDuplicateFixture struct {
	ServerID  int64  `gorm:"column:server_id;primaryKey"`
	SpaceID   int64  `gorm:"column:space_id"`
	Name      string `gorm:"column:name"`
	DeletedAt int64  `gorm:"column:deleted_at"`
}

func (mcpMigrationDuplicateFixture) TableName() string {
	return "mcp_migration_duplicate_fixture"
}

func TestMCPManagementMigrationRejectsDuplicateLiveNamesWithoutMutation(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docker", "atlas", "migrations", "20260714000100_mcp_management_parity.sql")
	migration, err := os.ReadFile(path)
	require.NoError(t, err)
	sql := string(migration)
	uniqueAt := strings.Index(sql, "uk_mcp_tool_servers_space_name_deleted")
	require.NotEqual(t, -1, uniqueAt)
	require.NotContains(t, strings.ToUpper(sql), "UPDATE `MCP_TOOL_SERVERS`")
	require.NotContains(t, strings.ToUpper(sql), "DELETE FROM `MCP_TOOL_SERVERS`")
	require.Less(t, uniqueAt, strings.Index(sql, "ADD COLUMN `creator_id`"))

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpMigrationDuplicateFixture{}))
	fixtures := []mcpMigrationDuplicateFixture{
		{ServerID: 10, SpaceID: 1, Name: "duplicate", DeletedAt: 0},
		{ServerID: 11, SpaceID: 1, Name: "duplicate", DeletedAt: 0},
		{ServerID: 12, SpaceID: 1, Name: "duplicate", DeletedAt: 0},
		{ServerID: 20, SpaceID: 2, Name: "duplicate", DeletedAt: 0},
	}
	require.NoError(t, db.Create(&fixtures).Error)
	require.Error(t, db.Exec(`CREATE UNIQUE INDEX live_name_fixture_unique ON mcp_migration_duplicate_fixture(space_id, name, deleted_at)`).Error)

	var live []mcpMigrationDuplicateFixture
	require.NoError(t, db.Where("deleted_at = 0").Order("server_id").Find(&live).Error)
	require.Len(t, live, 4)
	var deleted []mcpMigrationDuplicateFixture
	require.NoError(t, db.Where("deleted_at <> 0").Order("server_id").Find(&deleted).Error)
	require.Empty(t, deleted)
}

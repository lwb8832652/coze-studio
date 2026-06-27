/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestMCPRuntimeAuditRepositoryCreatesAndListsEvents(t *testing.T) {
	repo := newTestMCPRuntimeAuditRepository(t)
	first := sampleMCPRuntimeAuditEvent(5001)
	second := sampleMCPRuntimeAuditEvent(5002)
	second.EventType = "mcp.tool.completed"
	second.OutputBytes = 128
	second.CreatedAt = 200

	require.NoError(t, repo.CreateMCPRuntimeAuditEvent(context.Background(), first))
	require.NoError(t, repo.CreateMCPRuntimeAuditEvent(context.Background(), second))

	got, err := repo.ListMCPRuntimeAuditEvents(
		context.Background(),
		ListMCPRuntimeAuditEventsRequest{
			RunID: 20,
			Limit: 10,
		},
	)

	require.NoError(t, err)
	require.Equal(t, []*entity.MCPRuntimeAuditEvent{first, second}, got)
}

func TestMCPRuntimeAuditRepositoryBoundsSanitizedFields(t *testing.T) {
	repo := newTestMCPRuntimeAuditRepository(t)
	event := sampleMCPRuntimeAuditEvent(5001)
	event.RuntimeToolName = "mcp_100_search_docs_with_a_very_long_suffix_that_should_not_keep_growing_forever"
	event.ErrorCode = "transport_failed_with_secret_path"

	require.NoError(t, repo.CreateMCPRuntimeAuditEvent(context.Background(), event))

	got, err := repo.ListMCPRuntimeAuditEvents(
		context.Background(),
		ListMCPRuntimeAuditEventsRequest{RunID: 20, Limit: 1},
	)

	require.NoError(t, err)
	require.Len(t, got, 1)
	require.LessOrEqual(t, len(got[0].RuntimeToolName), 128)
	require.LessOrEqual(t, len(got[0].ErrorCode), 64)
	require.NotContains(t, got[0].RuntimeToolName, "secret docs")
	require.NotContains(t, got[0].ErrorCode, "/mnt/coze/mcp")
}

func newTestMCPRuntimeAuditRepository(t *testing.T) MCPRuntimeAuditRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpRuntimeAuditEventPO{}))

	return NewMCPRuntimeAuditRepository(db)
}

func sampleMCPRuntimeAuditEvent(id int64) *entity.MCPRuntimeAuditEvent {
	return &entity.MCPRuntimeAuditEvent{
		ID:              id,
		SpaceID:         30,
		ThreadID:        10,
		RunID:           20,
		ServerID:        100,
		RuntimeToolName: "mcp_100_search_docs",
		EventType:       "mcp.tool.started",
		ErrorCode:       "",
		ElapsedMillis:   3,
		OutputBytes:     0,
		CreatedAt:       100,
	}
}

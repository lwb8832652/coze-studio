// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMySQLManagementAuditRepositoryPersistsSafeLifecycleAndStableOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpManagementAuditEventPO{}))
	repo := NewMySQLManagementAuditRepository(db)

	first := &ManagementAuditEvent{
		SpaceID: 10, ServerID: 41, ActorID: 7, ToolName: "search",
		Status: ManagementAuditStatusPending, CreatedAt: 1000,
	}
	second := &ManagementAuditEvent{
		SpaceID: 10, ServerID: 41, ActorID: 8, ToolName: "search",
		Status: ManagementAuditStatusPending, CreatedAt: 1000,
	}
	require.NoError(t, repo.CreatePending(context.Background(), first))
	require.NoError(t, repo.CreatePending(context.Background(), second))
	require.Positive(t, first.EventID)
	require.Greater(t, second.EventID, first.EventID)

	require.NoError(t, repo.Complete(context.Background(), first.EventID, ManagementAuditCompletion{
		Status:      ManagementAuditStatusFailed,
		LatencyMs:   17,
		ErrorCode:   ManagementAuditErrorRuntimeFailed,
		CompletedAt: 1017,
	}))

	events, err := repo.List(context.Background(), 10, 41, nil, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, second.EventID, events[0].EventID)
	require.Equal(t, ManagementAuditStatusPending, events[0].Status)
	require.Equal(t, first.EventID, events[1].EventID)
	require.Equal(t, ManagementAuditStatusFailed, events[1].Status)
	require.Equal(t, ManagementAuditErrorRuntimeFailed, events[1].ErrorCode)
	require.Equal(t, "runtime call failed", events[1].ErrorSummary)
	require.Equal(t, int64(1017), events[1].CompletedAt)

	cursor := &ManagementAuditCursor{CreatedAt: events[0].CreatedAt, EventID: events[0].EventID}
	after, err := repo.List(context.Background(), 10, 41, cursor, 10)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Equal(t, first.EventID, after[0].EventID)
}

func TestMySQLManagementAuditSchemaHasNoSensitivePayloadColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&mcpManagementAuditEventPO{}))

	columnTypes, err := db.Migrator().ColumnTypes(&mcpManagementAuditEventPO{})
	require.NoError(t, err)
	columns := make(map[string]struct{}, len(columnTypes))
	for _, column := range columnTypes {
		columns[strings.ToLower(column.Name())] = struct{}{}
	}
	for _, forbidden := range []string{
		"arguments", "result", "config", "auth", "provider_body", "raw_payload",
	} {
		_, exists := columns[forbidden]
		require.False(t, exists, "sensitive column %q must not exist", forbidden)
	}
	for _, required := range []string{
		"event_id", "space_id", "server_id", "actor_id", "tool_name", "status",
		"latency_ms", "error_code", "error_summary", "created_at", "completed_at",
	} {
		_, exists := columns[required]
		require.True(t, exists, "required audit column %q is missing", required)
	}
}

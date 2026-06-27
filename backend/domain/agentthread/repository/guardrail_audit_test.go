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

package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestGuardrailAuditRepositoryCreatesAndListsEvents(t *testing.T) {
	repo := newTestGuardrailAuditRepository(t)
	first := sampleGuardrailAuditEvent(6001)
	second := sampleGuardrailAuditEvent(6002)
	second.Action = "deny"
	second.EventType = "guardrail.decision.deny"
	second.ReasonCode = "runtime_policy_deny"
	second.CreatedAt = 200

	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), first))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), second))

	got, total, err := repo.ListGuardrailAuditEvents(
		context.Background(),
		ListGuardrailAuditEventsRequest{
			RunID: 20,
			Limit: 10,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []*entity.GuardrailAuditEvent{first, second}, got)
}

func TestGuardrailAuditRepositoryBoundsSanitizedFields(t *testing.T) {
	repo := newTestGuardrailAuditRepository(t)
	event := sampleGuardrailAuditEvent(6001)
	event.TargetID = "runtime_tool:" + strings.Repeat("x", 200)
	event.Source = "adk_tool_wrapper_" + strings.Repeat("x", 200)
	event.RuleIDs = strings.Repeat("rule,", 160)

	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), event))

	got, total, err := repo.ListGuardrailAuditEvents(
		context.Background(),
		ListGuardrailAuditEventsRequest{RunID: 20, Limit: 1},
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, got, 1)
	require.LessOrEqual(t, len([]byte(got[0].TargetID)), 128)
	require.LessOrEqual(t, len([]byte(got[0].Source)), 64)
	require.LessOrEqual(t, len([]byte(got[0].RuleIDs)), 512)
}

func TestGuardrailAuditRepositoryReturnsTotalBeforePagination(t *testing.T) {
	repo := newTestGuardrailAuditRepository(t)
	first := sampleGuardrailAuditEvent(6001)
	first.CreatedAt = 100
	second := sampleGuardrailAuditEvent(6002)
	second.CreatedAt = 200
	otherThread := sampleGuardrailAuditEvent(6003)
	otherThread.ThreadID = 11
	otherThread.RunID = 21
	otherThread.CreatedAt = 300

	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), first))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), second))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), otherThread))

	got, total, err := repo.ListGuardrailAuditEvents(
		context.Background(),
		ListGuardrailAuditEventsRequest{
			ThreadID: 10,
			Limit:    1,
			Offset:   1,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []*entity.GuardrailAuditEvent{second}, got)
}

func TestGuardrailAuditRepositoryDeletesExpiredEventsInBatches(t *testing.T) {
	repo := newTestGuardrailAuditRepository(t)
	oldest := sampleGuardrailAuditEvent(6001)
	oldest.CreatedAt = 100
	secondOldest := sampleGuardrailAuditEvent(6002)
	secondOldest.CreatedAt = 200
	fresh := sampleGuardrailAuditEvent(6003)
	fresh.CreatedAt = 300

	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), oldest))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), secondOldest))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), fresh))

	deleted, err := repo.DeleteGuardrailAuditEventsBefore(
		context.Background(),
		DeleteGuardrailAuditEventsBeforeRequest{
			CutoffCreatedAt: 250,
			Limit:           1,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	got, total, err := repo.ListGuardrailAuditEvents(
		context.Background(),
		ListGuardrailAuditEventsRequest{ThreadID: 10, Limit: 10},
	)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []*entity.GuardrailAuditEvent{secondOldest, fresh}, got)
}

func TestGuardrailAuditRepositoryDeletesSpecificArchivedEventIDs(t *testing.T) {
	repo := newTestGuardrailAuditRepository(t)
	oldest := sampleGuardrailAuditEvent(6001)
	oldest.CreatedAt = 100
	secondOldest := sampleGuardrailAuditEvent(6002)
	secondOldest.CreatedAt = 200
	fresh := sampleGuardrailAuditEvent(6003)
	fresh.CreatedAt = 300

	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), oldest))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), secondOldest))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), fresh))

	deleted, err := repo.DeleteGuardrailAuditEventsByIDs(
		context.Background(),
		DeleteGuardrailAuditEventsByIDsRequest{
			EventIDs: []int64{6002},
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	got, total, err := repo.ListGuardrailAuditEvents(
		context.Background(),
		ListGuardrailAuditEventsRequest{ThreadID: 10, Limit: 10},
	)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []*entity.GuardrailAuditEvent{oldest, fresh}, got)
}

func TestGuardrailAuditRepositoryListsEventsBeforeCutoff(t *testing.T) {
	repo := newTestGuardrailAuditRepository(t)
	oldest := sampleGuardrailAuditEvent(6001)
	oldest.CreatedAt = 100
	secondOldest := sampleGuardrailAuditEvent(6002)
	secondOldest.CreatedAt = 200
	fresh := sampleGuardrailAuditEvent(6003)
	fresh.CreatedAt = 300

	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), oldest))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), secondOldest))
	require.NoError(t, repo.CreateGuardrailAuditEvent(context.Background(), fresh))

	got, total, err := repo.ListGuardrailAuditEventsBefore(
		context.Background(),
		ListGuardrailAuditEventsBeforeRequest{
			CutoffCreatedAt: 250,
			Limit:           1,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []*entity.GuardrailAuditEvent{oldest}, got)
}

func newTestGuardrailAuditRepository(t *testing.T) GuardrailAuditRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&guardrailAuditEventPO{}))

	return NewGuardrailAuditRepository(db)
}

func sampleGuardrailAuditEvent(id int64) *entity.GuardrailAuditEvent {
	return &entity.GuardrailAuditEvent{
		ID:         id,
		SpaceID:    30,
		ThreadID:   10,
		RunID:      20,
		ActorID:    40,
		EventType:  "guardrail.decision.confirm",
		TargetType: "tool_call",
		TargetID:   "runtime_tool:search_docs",
		Operation:  "invoke",
		Source:     "adk_tool_wrapper",
		Action:     "confirm",
		FailMode:   "fail_closed",
		Provider:   "scanner",
		ReasonCode: "high_risk",
		RuleIDs:    "rule:high_risk",
		CreatedAt:  100,
	}
}

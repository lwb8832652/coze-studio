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

package agentthread

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGuardrailAuditRetentionReaperDeletesEventsBeforeCutoff(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{deleteBeforeDeleted: 3}
	reaper := NewGuardrailAuditRetentionReaper(
		GuardrailAuditRetentionReaperOptions{
			Repository: repo,
			Retention:  24 * time.Hour,
			BatchSize:  7,
			NowMillis:  func() int64 { return 48 * int64(time.Hour/time.Millisecond) },
		},
	)

	result, err := reaper.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.NoError(t, err)
	require.Equal(t, GuardrailAuditRetentionReaperResult{
		CutoffCreatedAt: 24 * int64(time.Hour/time.Millisecond),
		Deleted:         3,
	}, result)
	require.Equal(t, 1, repo.deleteBeforeCalls)
	require.Equal(t, int64(24*time.Hour/time.Millisecond), repo.deleteBeforeReq.CutoffCreatedAt)
	require.Equal(t, int32(7), repo.deleteBeforeReq.Limit)
}

func TestGuardrailAuditRetentionReaperRequiresPositiveRetention(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{}
	reaper := NewGuardrailAuditRetentionReaper(
		GuardrailAuditRetentionReaperOptions{
			Repository: repo,
			Retention:  0,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := reaper.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, 0, repo.deleteBeforeCalls)
	require.Contains(t, err.Error(), "guardrail audit retention cleanup failed")
}

func TestGuardrailAuditRetentionReaperSanitizesRepositoryErrors(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{
		deleteBeforeErr: errors.New("delete audit row with prompt sk-secret and s3://raw"),
	}
	reaper := NewGuardrailAuditRetentionReaper(
		GuardrailAuditRetentionReaperOptions{
			Repository: repo,
			Retention:  time.Hour,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := reaper.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "guardrail audit retention cleanup failed")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "s3://")
}

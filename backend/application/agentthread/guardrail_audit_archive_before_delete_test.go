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

func TestGuardrailAuditArchiveBeforeDeleteCleanerArchivesBeforeDeletingSameCutoff(
	t *testing.T,
) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        2,
			ArchivedEventIDs: []int64{
				101,
				102,
			},
			Total:     5,
			ArchiveID: "archive_1",
		},
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{
		result: GuardrailAuditRetentionReaperResult{
			CutoffCreatedAt: 1000,
			Deleted:         2,
		},
	}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(
		GuardrailAuditArchiveBeforeDeleteCleanerOptions{
			Archiver:  archiver,
			Deleter:   deleter,
			Retention: 24 * time.Hour,
			BatchSize: 2,
			NowMillis: func() int64 { return 86401000 },
		},
	)

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.NoError(t, err)
	require.Equal(t, int64(1000), archiver.cutoffCreatedAt)
	require.Equal(t, int64(1000), deleter.cutoffCreatedAt)
	require.Equal(t, []int64{101, 102}, deleter.eventIDs)
	require.Equal(t, int64(2), result.Archived)
	require.Equal(t, int64(2), result.Deleted)
	require.Equal(t, "archive_1", result.ArchiveID)
}

func TestGuardrailAuditArchiveBeforeDeleteCleanerDoesNotDeleteWhenArchiveFails(
	t *testing.T,
) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		err: errors.New("write s3://bucket/raw with sk-secret prompt"),
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(
		GuardrailAuditArchiveBeforeDeleteCleanerOptions{
			Archiver:  archiver,
			Deleter:   deleter,
			Retention: 24 * time.Hour,
			BatchSize: 2,
			NowMillis: func() int64 { return 86401000 },
		},
	)

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, 1, archiver.calls)
	require.Equal(t, 0, deleter.calls)
	require.Contains(t, err.Error(), "guardrail audit retention cleanup failed")
	require.NotContains(t, err.Error(), "s3://")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "prompt")
}

func TestGuardrailAuditArchiveBeforeDeleteCleanerSkipsDeleteWhenNothingArchived(
	t *testing.T,
) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
		},
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{
		result: GuardrailAuditRetentionReaperResult{
			CutoffCreatedAt: 1000,
			Deleted:         99,
		},
	}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(
		GuardrailAuditArchiveBeforeDeleteCleanerOptions{
			Archiver:  archiver,
			Deleter:   deleter,
			Retention: 24 * time.Hour,
			BatchSize: 2,
			NowMillis: func() int64 { return 86401000 },
		},
	)

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.NoError(t, err)
	require.Equal(t, 0, deleter.calls)
	require.Equal(t, GuardrailAuditRetentionReaperResult{
		CutoffCreatedAt: 1000,
	}, result)
}

func TestGuardrailAuditArchiveBeforeDeleteCleanerFailsClosedOnCutoffMismatch(
	t *testing.T,
) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 999,
			Archived:        2,
			ArchivedEventIDs: []int64{
				101,
				102,
			},
			Total:     5,
			ArchiveID: "archive_1",
		},
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(
		GuardrailAuditArchiveBeforeDeleteCleanerOptions{
			Archiver:  archiver,
			Deleter:   deleter,
			Retention: 24 * time.Hour,
			BatchSize: 2,
			NowMillis: func() int64 { return 86401000 },
		},
	)

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, 0, deleter.calls)
	require.Contains(t, err.Error(), "guardrail audit retention cleanup failed")
	require.NotContains(t, err.Error(), "archive_1")
}

func TestGuardrailAuditArchiveBeforeDeleteCleanerDoesNotExposeDeleteErrors(
	t *testing.T,
) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        2,
			ArchivedEventIDs: []int64{
				101,
				102,
			},
			Total:     5,
			ArchiveID: "archive_1",
		},
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{
		err: errors.New("delete raw SQL with sk-secret and prompt"),
	}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(
		GuardrailAuditArchiveBeforeDeleteCleanerOptions{
			Archiver:  archiver,
			Deleter:   deleter,
			Retention: 24 * time.Hour,
			BatchSize: 2,
			NowMillis: func() int64 { return 86401000 },
		},
	)

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, 1, deleter.calls)
	require.Contains(t, err.Error(), "guardrail audit retention cleanup failed")
	require.NotContains(t, err.Error(), "raw SQL")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "prompt")
}

func TestGuardrailAuditArchiveBeforeDeleteCleanerFailsClosedOnMissingArchivedIDs(
	t *testing.T,
) {
	archiver := &recordingGuardrailAuditArchiveRunner{
		result: GuardrailAuditArchiveExportResult{
			CutoffCreatedAt: 1000,
			Archived:        2,
			Total:           5,
			ArchiveID:       "archive_1",
		},
	}
	deleter := &recordingGuardrailAuditRetentionDeleter{}
	cleaner := NewGuardrailAuditArchiveBeforeDeleteCleaner(
		GuardrailAuditArchiveBeforeDeleteCleanerOptions{
			Archiver:  archiver,
			Deleter:   deleter,
			Retention: 24 * time.Hour,
			BatchSize: 2,
			NowMillis: func() int64 { return 86401000 },
		},
	)

	result, err := cleaner.CleanupExpiredGuardrailAuditEvents(context.Background())

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, 0, deleter.calls)
	require.Contains(t, err.Error(), "guardrail audit retention cleanup failed")
	require.NotContains(t, err.Error(), "archive_1")
}

type recordingGuardrailAuditRetentionDeleter struct {
	result          GuardrailAuditRetentionReaperResult
	err             error
	cutoffCreatedAt int64
	eventIDs        []int64
	calls           int
}

func (d *recordingGuardrailAuditRetentionDeleter) CleanupGuardrailAuditEventsByIDs(
	ctx context.Context,
	cutoffCreatedAt int64,
	eventIDs []int64,
) (GuardrailAuditRetentionReaperResult, error) {
	d.calls++
	d.cutoffCreatedAt = cutoffCreatedAt
	d.eventIDs = append([]int64(nil), eventIDs...)
	if d.err != nil {
		return GuardrailAuditRetentionReaperResult{}, d.err
	}

	return d.result, nil
}

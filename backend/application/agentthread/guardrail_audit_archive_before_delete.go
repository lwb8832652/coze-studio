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
	"time"
)

var errGuardrailAuditRetentionCleanupFailed = errors.New("guardrail audit retention cleanup failed")

type GuardrailAuditRetentionDeleter interface {
	CleanupGuardrailAuditEventsByIDs(
		ctx context.Context,
		cutoffCreatedAt int64,
		eventIDs []int64,
	) (GuardrailAuditRetentionReaperResult, error)
}

type GuardrailAuditArchiveBeforeDeleteCleanerOptions struct {
	Archiver  GuardrailAuditArchiver
	Deleter   GuardrailAuditRetentionDeleter
	Retention time.Duration
	BatchSize int32
	NowMillis func() int64
}

type GuardrailAuditArchiveBeforeDeleteCleaner struct {
	archiver  GuardrailAuditArchiver
	deleter   GuardrailAuditRetentionDeleter
	retention time.Duration
	batchSize int32
	nowMillis func() int64
}

func NewGuardrailAuditArchiveBeforeDeleteCleaner(
	options GuardrailAuditArchiveBeforeDeleteCleanerOptions,
) *GuardrailAuditArchiveBeforeDeleteCleaner {
	batchSize := options.BatchSize
	if batchSize <= 0 || batchSize > defaultGuardrailAuditRetentionBatchSize {
		batchSize = defaultGuardrailAuditRetentionBatchSize
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &GuardrailAuditArchiveBeforeDeleteCleaner{
		archiver:  options.Archiver,
		deleter:   options.Deleter,
		retention: options.Retention,
		batchSize: batchSize,
		nowMillis: nowMillis,
	}
}

func (c *GuardrailAuditArchiveBeforeDeleteCleaner) CleanupExpiredGuardrailAuditEvents(
	ctx context.Context,
) (GuardrailAuditRetentionReaperResult, error) {
	if !c.validConfig() {
		return GuardrailAuditRetentionReaperResult{}, errGuardrailAuditRetentionCleanupFailed
	}

	cutoffCreatedAt := c.now() - c.retention.Milliseconds()
	if cutoffCreatedAt <= 0 {
		return GuardrailAuditRetentionReaperResult{}, errGuardrailAuditRetentionCleanupFailed
	}

	archiveResult, err := c.archiver.ArchiveExpiredGuardrailAuditEvents(
		ctx,
		cutoffCreatedAt,
	)
	if err != nil {
		return GuardrailAuditRetentionReaperResult{}, errGuardrailAuditRetentionCleanupFailed
	}
	if archiveResult.CutoffCreatedAt != cutoffCreatedAt {
		return GuardrailAuditRetentionReaperResult{}, errGuardrailAuditRetentionCleanupFailed
	}
	if archiveResult.Archived <= 0 {
		return GuardrailAuditRetentionReaperResult{
			CutoffCreatedAt: cutoffCreatedAt,
		}, nil
	}
	if int64(len(archiveResult.ArchivedEventIDs)) != archiveResult.Archived ||
		!validGuardrailAuditArchiveEventIDs(archiveResult.ArchivedEventIDs) {
		return GuardrailAuditRetentionReaperResult{}, errGuardrailAuditRetentionCleanupFailed
	}

	deleteResult, err := c.deleter.CleanupGuardrailAuditEventsByIDs(
		ctx,
		cutoffCreatedAt,
		archiveResult.ArchivedEventIDs,
	)
	if err != nil {
		return GuardrailAuditRetentionReaperResult{}, errGuardrailAuditRetentionCleanupFailed
	}
	deleteResult.CutoffCreatedAt = cutoffCreatedAt
	deleteResult.Archived = archiveResult.Archived
	deleteResult.ArchivedEventIDs = append([]int64(nil), archiveResult.ArchivedEventIDs...)
	deleteResult.ArchiveID = archiveResult.ArchiveID

	return deleteResult, nil
}

func (c *GuardrailAuditArchiveBeforeDeleteCleaner) validConfig() bool {
	return c != nil &&
		c.archiver != nil &&
		c.deleter != nil &&
		c.retention > 0 &&
		c.batchSize > 0
}

func (c *GuardrailAuditArchiveBeforeDeleteCleaner) now() int64 {
	if c == nil || c.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return c.nowMillis()
}

func validGuardrailAuditArchiveEventIDs(eventIDs []int64) bool {
	if len(eventIDs) == 0 || len(eventIDs) > int(defaultGuardrailAuditRetentionBatchSize) {
		return false
	}
	seen := make(map[int64]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID <= 0 {
			return false
		}
		if _, exists := seen[eventID]; exists {
			return false
		}
		seen[eventID] = struct{}{}
	}

	return true
}

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

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const defaultGuardrailAuditRetentionBatchSize = int32(1000)

type GuardrailAuditRetentionReaperOptions struct {
	Repository domainrepo.GuardrailAuditRepository
	Retention  time.Duration
	BatchSize  int32
	NowMillis  func() int64
}

type GuardrailAuditRetentionReaper struct {
	repository domainrepo.GuardrailAuditRepository
	retention  time.Duration
	batchSize  int32
	nowMillis  func() int64
}

type GuardrailAuditRetentionReaperResult struct {
	CutoffCreatedAt  int64
	Archived         int64
	ArchivedEventIDs []int64
	ArchiveID        string
	Deleted          int64
}

func NewGuardrailAuditRetentionReaper(
	options GuardrailAuditRetentionReaperOptions,
) *GuardrailAuditRetentionReaper {
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = defaultGuardrailAuditRetentionBatchSize
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &GuardrailAuditRetentionReaper{
		repository: options.Repository,
		retention:  options.Retention,
		batchSize:  batchSize,
		nowMillis:  nowMillis,
	}
}

func (r *GuardrailAuditRetentionReaper) CleanupExpiredGuardrailAuditEvents(
	ctx context.Context,
) (GuardrailAuditRetentionReaperResult, error) {
	if !r.validConfig() {
		return GuardrailAuditRetentionReaperResult{}, errors.New("guardrail audit retention cleanup failed")
	}

	cutoff := r.now() - r.retention.Milliseconds()
	return r.CleanupGuardrailAuditEventsBefore(ctx, cutoff)
}

func (r *GuardrailAuditRetentionReaper) CleanupGuardrailAuditEventsBefore(
	ctx context.Context,
	cutoffCreatedAt int64,
) (GuardrailAuditRetentionReaperResult, error) {
	if !r.validConfig() || cutoffCreatedAt <= 0 {
		return GuardrailAuditRetentionReaperResult{}, errors.New("guardrail audit retention cleanup failed")
	}

	deleted, err := r.repository.DeleteGuardrailAuditEventsBefore(
		ctx,
		domainrepo.DeleteGuardrailAuditEventsBeforeRequest{
			CutoffCreatedAt: cutoffCreatedAt,
			Limit:           r.batchSize,
		},
	)
	if err != nil {
		return GuardrailAuditRetentionReaperResult{}, errors.New("guardrail audit retention cleanup failed")
	}

	return GuardrailAuditRetentionReaperResult{
		CutoffCreatedAt: cutoffCreatedAt,
		Deleted:         deleted,
	}, nil
}

func (r *GuardrailAuditRetentionReaper) CleanupGuardrailAuditEventsByIDs(
	ctx context.Context,
	cutoffCreatedAt int64,
	eventIDs []int64,
) (GuardrailAuditRetentionReaperResult, error) {
	if !r.validConfig() || cutoffCreatedAt <= 0 || len(eventIDs) == 0 {
		return GuardrailAuditRetentionReaperResult{}, errors.New("guardrail audit retention cleanup failed")
	}

	deleted, err := r.repository.DeleteGuardrailAuditEventsByIDs(
		ctx,
		domainrepo.DeleteGuardrailAuditEventsByIDsRequest{
			EventIDs: eventIDs,
		},
	)
	if err != nil {
		return GuardrailAuditRetentionReaperResult{}, errors.New("guardrail audit retention cleanup failed")
	}

	return GuardrailAuditRetentionReaperResult{
		CutoffCreatedAt: cutoffCreatedAt,
		Deleted:         deleted,
	}, nil
}

func (r *GuardrailAuditRetentionReaper) validConfig() bool {
	return r != nil &&
		r.repository != nil &&
		r.retention > 0 &&
		r.batchSize > 0
}

func (r *GuardrailAuditRetentionReaper) now() int64 {
	if r == nil || r.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return r.nowMillis()
}

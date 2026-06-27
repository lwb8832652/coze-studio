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

const GuardrailAuditArchiveSchema = "coze.guardrail_audit.archive.v1"

type GuardrailAuditArchiveExporterOptions struct {
	Repository domainrepo.GuardrailAuditRepository
	Writer     GuardrailAuditArchiveWriter
	BatchSize  int32
	NowMillis  func() int64
}

type GuardrailAuditArchiveExporter struct {
	repository domainrepo.GuardrailAuditRepository
	writer     GuardrailAuditArchiveWriter
	batchSize  int32
	nowMillis  func() int64
}

type GuardrailAuditArchiveWriter interface {
	WriteGuardrailAuditArchive(
		ctx context.Context,
		payload GuardrailAuditArchivePayload,
	) (GuardrailAuditArchiveWriteResult, error)
}

type GuardrailAuditArchivePayload struct {
	Schema          string
	ExportedAt      int64
	CutoffCreatedAt int64
	Total           int64
	Events          []*GuardrailAuditEventSummary
}

type GuardrailAuditArchiveWriteResult struct {
	ArchiveID string
}

type GuardrailAuditArchiveExportResult struct {
	CutoffCreatedAt  int64
	Archived         int64
	ArchivedEventIDs []int64
	Total            int64
	ArchiveID        string
}

func NewGuardrailAuditArchiveExporter(
	options GuardrailAuditArchiveExporterOptions,
) *GuardrailAuditArchiveExporter {
	batchSize := options.BatchSize
	if batchSize <= 0 || batchSize > defaultGuardrailAuditRetentionBatchSize {
		batchSize = defaultGuardrailAuditRetentionBatchSize
	}
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &GuardrailAuditArchiveExporter{
		repository: options.Repository,
		writer:     options.Writer,
		batchSize:  batchSize,
		nowMillis:  nowMillis,
	}
}

func (e *GuardrailAuditArchiveExporter) ArchiveExpiredGuardrailAuditEvents(
	ctx context.Context,
	cutoffCreatedAt int64,
) (GuardrailAuditArchiveExportResult, error) {
	if e == nil || e.repository == nil || e.batchSize <= 0 {
		return GuardrailAuditArchiveExportResult{}, errors.New("guardrail audit archive export failed")
	}

	events, total, err := e.repository.ListGuardrailAuditEventsBefore(
		ctx,
		domainrepo.ListGuardrailAuditEventsBeforeRequest{
			CutoffCreatedAt: cutoffCreatedAt,
			Limit:           e.batchSize,
		},
	)
	if err != nil {
		return GuardrailAuditArchiveExportResult{}, errors.New("guardrail audit archive export failed")
	}
	if len(events) == 0 {
		return GuardrailAuditArchiveExportResult{CutoffCreatedAt: cutoffCreatedAt}, nil
	}
	if e.writer == nil {
		return GuardrailAuditArchiveExportResult{}, errors.New("guardrail audit archive export failed")
	}

	payload := GuardrailAuditArchivePayload{
		Schema:          GuardrailAuditArchiveSchema,
		ExportedAt:      e.now(),
		CutoffCreatedAt: cutoffCreatedAt,
		Total:           total,
		Events:          make([]*GuardrailAuditEventSummary, 0, len(events)),
	}
	archivedEventIDs := make([]int64, 0, len(events))
	for _, event := range events {
		payload.Events = append(payload.Events, DomainGuardrailAuditEventToSummary(event))
		archivedEventIDs = append(archivedEventIDs, event.ID)
	}

	writeResult, err := e.writer.WriteGuardrailAuditArchive(ctx, payload)
	if err != nil {
		return GuardrailAuditArchiveExportResult{}, errors.New("guardrail audit archive export failed")
	}

	return GuardrailAuditArchiveExportResult{
		CutoffCreatedAt:  cutoffCreatedAt,
		Archived:         int64(len(payload.Events)),
		ArchivedEventIDs: archivedEventIDs,
		Total:            total,
		ArchiveID:        sanitizeGuardrailIdentifier(writeResult.ArchiveID, 128),
	}, nil
}

func (e *GuardrailAuditArchiveExporter) now() int64 {
	if e == nil || e.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return e.nowMillis()
}

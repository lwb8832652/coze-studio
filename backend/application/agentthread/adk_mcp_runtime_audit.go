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

package agentthread

import (
	"context"
	"errors"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type ADKMCPRuntimeAuditRecord struct {
	SpaceID         int64
	ThreadID        int64
	RunID           int64
	ServerID        int64
	RuntimeToolName string
	Transport       string
	EventType       string
	ErrorCode       string
	ElapsedMillis   int64
	OutputBytes     int64
}

type ADKMCPRuntimeAuditRecorder interface {
	RecordADKMCPRuntimeAudit(
		ctx context.Context,
		record ADKMCPRuntimeAuditRecord,
	) error
}

type ApplicationADKMCPRuntimeAuditRecorderOptions struct {
	Repository       domainrepo.MCPRuntimeAuditRepository
	IDGen            idgen.IDGenerator
	MetricsCollector RuntimeMetricsCollector
	NowMillis        func() int64
}

type ApplicationADKMCPRuntimeAuditRecorder struct {
	repository       domainrepo.MCPRuntimeAuditRepository
	idGen            idgen.IDGenerator
	metricsCollector RuntimeMetricsCollector
	nowMillis        func() int64
}

func NewApplicationADKMCPRuntimeAuditRecorder(
	options ApplicationADKMCPRuntimeAuditRecorderOptions,
) *ApplicationADKMCPRuntimeAuditRecorder {
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &ApplicationADKMCPRuntimeAuditRecorder{
		repository:       options.Repository,
		idGen:            options.IDGen,
		metricsCollector: options.MetricsCollector,
		nowMillis:        nowMillis,
	}
}

func (r *ApplicationADKMCPRuntimeAuditRecorder) RecordADKMCPRuntimeAudit(
	ctx context.Context,
	record ADKMCPRuntimeAuditRecord,
) error {
	if !validADKMCPRuntimeAuditRecord(record) ||
		r == nil ||
		r.repository == nil ||
		r.idGen == nil {
		return errors.New("mcp runtime audit record failed")
	}
	id, err := r.idGen.GenID(ctx)
	if err != nil || id <= 0 {
		return errors.New("mcp runtime audit record failed")
	}
	event := &domainentity.MCPRuntimeAuditEvent{
		ID:              id,
		SpaceID:         record.SpaceID,
		ThreadID:        record.ThreadID,
		RunID:           record.RunID,
		ServerID:        record.ServerID,
		RuntimeToolName: strings.TrimSpace(record.RuntimeToolName),
		EventType:       strings.TrimSpace(record.EventType),
		ErrorCode:       strings.TrimSpace(record.ErrorCode),
		ElapsedMillis:   record.ElapsedMillis,
		OutputBytes:     record.OutputBytes,
		CreatedAt:       r.now(),
	}
	if err := r.repository.CreateMCPRuntimeAuditEvent(ctx, event); err != nil {
		return errors.New("mcp runtime audit record failed")
	}
	recordRuntimeMCPInvocation(ctx, r.metricsCollector, record)

	return nil
}

func (r *ApplicationADKMCPRuntimeAuditRecorder) now() int64 {
	if r == nil || r.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return r.nowMillis()
}

func validADKMCPRuntimeAuditRecord(record ADKMCPRuntimeAuditRecord) bool {
	return record.SpaceID > 0 &&
		record.ThreadID > 0 &&
		record.RunID > 0 &&
		record.ServerID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(record.RuntimeToolName)) &&
		strings.TrimSpace(record.EventType) != "" &&
		record.ElapsedMillis >= 0 &&
		record.OutputBytes >= 0
}

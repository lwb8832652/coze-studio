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
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestApplicationADKMCPRuntimeAuditRecorderPersistsContentFreeRecord(
	t *testing.T,
) {
	repo := &recordingMCPRuntimeAuditRepository{}
	recorder := NewApplicationADKMCPRuntimeAuditRecorder(
		ApplicationADKMCPRuntimeAuditRecorderOptions{
			Repository: repo,
			IDGen:      &mcpWorkdirLeaseSequenceIDGen{next: 8101},
			NowMillis:  func() int64 { return 2000 },
		},
	)

	err := recorder.RecordADKMCPRuntimeAudit(
		context.Background(),
		ADKMCPRuntimeAuditRecord{
			SpaceID:         30,
			ThreadID:        10,
			RunID:           20,
			ServerID:        100,
			RuntimeToolName: "mcp_100_search_docs",
			EventType:       "mcp.tool.completed",
			ErrorCode:       "",
			ElapsedMillis:   25,
			OutputBytes:     128,
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, &domainentity.MCPRuntimeAuditEvent{
		ID:              8101,
		SpaceID:         30,
		ThreadID:        10,
		RunID:           20,
		ServerID:        100,
		RuntimeToolName: "mcp_100_search_docs",
		EventType:       "mcp.tool.completed",
		ErrorCode:       "",
		ElapsedMillis:   25,
		OutputBytes:     128,
		CreatedAt:       2000,
	}, repo.createdEvent)
}

func TestApplicationADKMCPRuntimeAuditRecorderSanitizesErrors(t *testing.T) {
	recorder := NewApplicationADKMCPRuntimeAuditRecorder(
		ApplicationADKMCPRuntimeAuditRecorderOptions{},
	)

	err := recorder.RecordADKMCPRuntimeAudit(
		context.Background(),
		ADKMCPRuntimeAuditRecord{
			SpaceID:         30,
			ThreadID:        10,
			RunID:           20,
			ServerID:        100,
			RuntimeToolName: "mcp_100_search_docs",
			EventType:       "mcp.tool.failed",
			ErrorCode:       "transport_failed",
		},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp runtime audit record failed")
	require.NotContains(t, err.Error(), "mcp_100_search_docs")
}

type recordingMCPRuntimeAuditRepository struct {
	createdEvent *domainentity.MCPRuntimeAuditEvent
	createCalls  int
	createErr    error
}

func (r *recordingMCPRuntimeAuditRepository) CreateMCPRuntimeAuditEvent(
	ctx context.Context,
	event *domainentity.MCPRuntimeAuditEvent,
) error {
	r.createCalls++
	if event != nil {
		cloned := *event
		r.createdEvent = &cloned
	}
	return r.createErr
}

func (r *recordingMCPRuntimeAuditRepository) ListMCPRuntimeAuditEvents(
	ctx context.Context,
	req domainrepo.ListMCPRuntimeAuditEventsRequest,
) ([]*domainentity.MCPRuntimeAuditEvent, error) {
	return nil, nil
}

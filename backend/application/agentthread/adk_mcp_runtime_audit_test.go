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

func TestApplicationADKMCPRuntimeAuditRecorderRecordsRuntimeMetrics(t *testing.T) {
	repo := &recordingMCPRuntimeAuditRepository{}
	metrics := &recordingRuntimeMetricsCollector{}
	recorder := NewApplicationADKMCPRuntimeAuditRecorder(
		ApplicationADKMCPRuntimeAuditRecorderOptions{
			Repository:       repo,
			IDGen:            &mcpWorkdirLeaseSequenceIDGen{next: 8101},
			MetricsCollector: metrics,
			NowMillis:        func() int64 { return 2000 },
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
			Transport:       "streamable_http",
			EventType:       "mcp.tool.failed",
			ErrorCode:       "transport_failed",
			ElapsedMillis:   25,
			OutputBytes:     128,
		},
	)

	require.NoError(t, err)
	require.Equal(t, []RuntimeMCPInvocationMetricsObservation{
		{
			Transport:   "streamable_http",
			Result:      "failed",
			ErrorCode:   "transport_failed",
			LatencyMs:   25,
			OutputBytes: 128,
		},
	}, metrics.mcpInvocations)
}

func TestApplicationADKMCPRuntimeAuditRecorderSkipsNonTerminalRuntimeMetrics(
	t *testing.T,
) {
	repo := &recordingMCPRuntimeAuditRepository{}
	metrics := &recordingRuntimeMetricsCollector{}
	recorder := NewApplicationADKMCPRuntimeAuditRecorder(
		ApplicationADKMCPRuntimeAuditRecorderOptions{
			Repository:       repo,
			IDGen:            &mcpWorkdirLeaseSequenceIDGen{next: 8101},
			MetricsCollector: metrics,
			NowMillis:        func() int64 { return 2000 },
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
			Transport:       "stdio",
			EventType:       "mcp.tool.started",
			ElapsedMillis:   5,
		},
	)

	require.NoError(t, err)
	require.Empty(t, metrics.mcpInvocations)
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

func TestApplicationListMCPRuntimeAuditEventsReturnsMetadataOnlyPage(t *testing.T) {
	repo := &recordingMCPRuntimeAuditRepository{
		listedEvents: []*domainentity.MCPRuntimeAuditEvent{
			{
				ID:              8201,
				SpaceID:         30,
				ThreadID:        10,
				RunID:           20,
				ServerID:        100,
				RuntimeToolName: "mcp_100_get_weather",
				EventType:       "mcp.tool.completed",
				ErrorCode:       "",
				ElapsedMillis:   42,
				OutputBytes:     256,
				CreatedAt:       5000,
			},
		},
		listTotal: 2,
	}
	authorizer := &recordingMCPRuntimeAuditAuthorizer{}
	app := &ApplicationService{
		MCPRuntimeAuditRepository: repo,
		MCPRuntimeAuditAuthorizer: authorizer,
	}

	resp, err := app.ListMCPRuntimeAuditEvents(
		context.Background(),
		&ListMCPRuntimeAuditEventsRequest{
			ThreadID: 10,
			RunID:    20,
			ViewerID: 40,
			Page:     2,
			PageSize: 1,
		},
	)

	require.NoError(t, err)
	require.Equal(t, MCPRuntimeAuditAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(40), authorizer.req.ViewerID)
	require.Equal(t, int64(10), repo.listReq.ThreadID)
	require.Equal(t, int64(20), repo.listReq.RunID)
	require.Equal(t, int32(1), repo.listReq.Limit)
	require.Equal(t, int32(1), repo.listReq.Offset)
	require.Equal(t, int64(2), resp.Total)
	require.Len(t, resp.Events, 1)
	require.Equal(t, int64(8201), resp.Events[0].EventID)
	require.Equal(t, "mcp_100_get_weather", resp.Events[0].RuntimeToolName)
	require.Equal(t, "mcp.tool.completed", resp.Events[0].EventType)
	require.Equal(t, int64(42), resp.Events[0].ElapsedMillis)

	require.NotContains(t, resp.Events[0].RuntimeToolName, "api_key")
	require.NotContains(t, resp.Events[0].ErrorCode, "provider_raw")
}

type recordingMCPRuntimeAuditRepository struct {
	createdEvent *domainentity.MCPRuntimeAuditEvent
	createCalls  int
	createErr    error
	listedEvents []*domainentity.MCPRuntimeAuditEvent
	listTotal    int64
	listReq      domainrepo.ListMCPRuntimeAuditEventsRequest
	listCalls    int
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
) ([]*domainentity.MCPRuntimeAuditEvent, int64, error) {
	r.listCalls++
	r.listReq = req
	return r.listedEvents, r.listTotal, nil
}

type recordingMCPRuntimeAuditAuthorizer struct {
	req MCPRuntimeAuditAccessRequest
	err error
}

func (a *recordingMCPRuntimeAuditAuthorizer) AuthorizeMCPRuntimeAuditAccess(
	_ context.Context,
	req MCPRuntimeAuditAccessRequest,
) error {
	a.req = req
	return a.err
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

type managementAuditRepositorySpy struct {
	mu          sync.Mutex
	nextID      int64
	events      []*ManagementAuditEvent
	createErr   error
	completeErr error
}

func (r *managementAuditRepositorySpy) CreatePending(_ context.Context, event *ManagementAuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createErr != nil {
		return r.createErr
	}
	r.nextID++
	event.EventID = r.nextID
	r.events = append(r.events, cloneManagementAuditEvent(event))
	return nil
}

func (r *managementAuditRepositorySpy) Complete(
	_ context.Context,
	eventID int64,
	completion ManagementAuditCompletion,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completeErr != nil {
		return r.completeErr
	}
	for _, event := range r.events {
		if event.EventID != eventID {
			continue
		}
		event.Status = completion.Status
		event.LatencyMs = completion.LatencyMs
		event.ErrorCode = normalizeManagementAuditErrorCode(completion.ErrorCode)
		event.ErrorSummary = safeManagementAuditErrorSummary(event.ErrorCode)
		event.CompletedAt = completion.CompletedAt
		return nil
	}
	return ErrManagementAuditTransition
}

func (r *managementAuditRepositorySpy) List(
	_ context.Context,
	spaceID int64,
	serverID int64,
	cursor *ManagementAuditCursor,
	limit int,
) ([]*ManagementAuditEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]*ManagementAuditEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.SpaceID != spaceID || event.ServerID != serverID {
			continue
		}
		if cursor != nil && (event.CreatedAt > cursor.CreatedAt ||
			(event.CreatedAt == cursor.CreatedAt && event.EventID >= cursor.EventID)) {
			continue
		}
		events = append(events, cloneManagementAuditEvent(event))
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CreatedAt == events[j].CreatedAt {
			return events[i].EventID > events[j].EventID
		}
		return events[i].CreatedAt > events[j].CreatedAt
	})
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func (r *managementAuditRepositorySpy) snapshot() []*ManagementAuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]*ManagementAuditEvent, 0, len(r.events))
	for _, event := range r.events {
		events = append(events, cloneManagementAuditEvent(event))
	}
	return events
}

type managementAuditExecutor struct {
	execute func(call RuntimeToolCall) (*RuntimeToolResult, error)
	calls   int
}

func (e *managementAuditExecutor) ExecuteMCPTool(_ context.Context, call RuntimeToolCall) (*RuntimeToolResult, error) {
	e.calls++
	if e.execute == nil {
		return nil, ErrRuntimeUnavailable
	}
	return e.execute(call)
}

func TestApplicationServiceTestCallPersistsPendingThenSafeSuccess(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Auth = `{"token":"auth-secret"}`
	server.Config = `{"url":"https://mcp.example.com"}`
	server.Description = "config-secret"
	require.NoError(t, catalog.Upsert(context.Background(), server))
	auditRepo := &managementAuditRepositorySpy{nextID: 900}
	executor := &managementAuditExecutor{}
	executor.execute = func(call RuntimeToolCall) (*RuntimeToolResult, error) {
		events := auditRepo.snapshot()
		require.Len(t, events, 1)
		require.Equal(t, ManagementAuditStatusPending, events[0].Status)
		require.Equal(t, int64(7), events[0].ActorID)
		require.Equal(t, int64(10), events[0].SpaceID)
		require.Equal(t, int64(41), events[0].ServerID)
		require.Equal(t, "search", events[0].ToolName)
		require.Zero(t, events[0].CompletedAt)
		return &RuntimeToolResult{
			Status:    "success",
			Output:    `{"provider_body":"result-secret"}`,
			LatencyMs: 23,
		}, nil
	}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
		AuditRepository:     auditRepo,
	})

	response, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID:  41,
		ToolName:  "search",
		Arguments: `{"query":"argument-secret"}`,
	})

	require.NoError(t, err)
	require.Equal(t, "success", response.Data.Status)
	events := auditRepo.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, int64(901), events[0].EventID)
	require.Equal(t, ManagementAuditStatusSuccess, events[0].Status)
	require.Equal(t, int64(23), events[0].LatencyMs)
	require.Empty(t, events[0].ErrorCode)
	require.Empty(t, events[0].ErrorSummary)
	require.Positive(t, events[0].CreatedAt)
	require.GreaterOrEqual(t, events[0].CompletedAt, events[0].CreatedAt)

	encoded, err := json.Marshal(events[0])
	require.NoError(t, err)
	for _, forbidden := range []string{
		"argument-secret", "result-secret", "config-secret", "auth-secret",
		"arguments", "result", "config", "auth", "provider_body",
	} {
		require.NotContains(t, string(encoded), forbidden)
	}
}

func TestApplicationServiceTestCallFailsClosedWhenPendingAuditCannotBeCreated(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	auditRepo := &managementAuditRepositorySpy{createErr: errors.New("database contains private detail")}
	executor := &managementAuditExecutor{execute: func(RuntimeToolCall) (*RuntimeToolResult, error) {
		return &RuntimeToolResult{Status: "success"}, nil
	}}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
		AuditRepository:     auditRepo,
	})

	_, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.ErrorIs(t, err, ErrManagementAuditUnavailable)
	require.NotContains(t, err.Error(), "private detail")
	require.Zero(t, executor.calls)
}

func TestApplicationServiceTestCallLeavesPendingWhenCompletionWriteFails(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	auditRepo := &managementAuditRepositorySpy{completeErr: errors.New("completion secret")}
	executor := &managementAuditExecutor{execute: func(RuntimeToolCall) (*RuntimeToolResult, error) {
		return &RuntimeToolResult{Status: "success", Output: `{"secret":"provider"}`, LatencyMs: 9}, nil
	}}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
		AuditRepository:     auditRepo,
	})

	response, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.NoError(t, err)
	require.Equal(t, "success", response.Data.Status)
	events := auditRepo.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, ManagementAuditStatusPending, events[0].Status)
	require.Zero(t, events[0].CompletedAt)
}

func TestApplicationServiceTestCallPersistsSafeFailure(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	auditRepo := &managementAuditRepositorySpy{}
	executor := &managementAuditExecutor{execute: func(RuntimeToolCall) (*RuntimeToolResult, error) {
		return nil, errors.New("dial token=runtime-secret provider_body=private")
	}}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
		AuditRepository:     auditRepo,
	})

	_, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.ErrorIs(t, err, ErrRuntimeCallFailed)
	events := auditRepo.snapshot()
	require.Len(t, events, 1)
	require.Equal(t, ManagementAuditStatusFailed, events[0].Status)
	require.Equal(t, ManagementAuditErrorRuntimeFailed, events[0].ErrorCode)
	require.Equal(t, "runtime call failed", events[0].ErrorSummary)
	encoded, marshalErr := json.Marshal(events[0])
	require.NoError(t, marshalErr)
	require.NotContains(t, string(encoded), "runtime-secret")
	require.NotContains(t, string(encoded), "provider_body")
}

func TestApplicationServiceListAuditEventsAuthorizesAndUsesStableCursor(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	auditRepo := NewInMemoryManagementAuditRepository()
	for i := 0; i < 105; i++ {
		event := &ManagementAuditEvent{
			SpaceID:   10,
			ServerID:  41,
			ActorID:   7,
			ToolName:  "search",
			Status:    ManagementAuditStatusPending,
			CreatedAt: 1000,
		}
		require.NoError(t, auditRepo.CreatePending(context.Background(), event))
	}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: &managementRoleReader{spaces: []*userentity.Space{{ID: 10, RoleType: 3}}},
		AuditRepository:     auditRepo,
	})

	first, err := svc.ListAuditEvents(managementContext(7), 41, "", 1000)
	require.NoError(t, err)
	require.Len(t, first.Data.Events, 100)
	require.NotEmpty(t, first.Data.NextCursor)
	require.Equal(t, "105", first.Data.Events[0].EventID)
	require.Equal(t, "6", first.Data.Events[99].EventID)

	second, err := svc.ListAuditEvents(managementContext(7), 41, first.Data.NextCursor, 1000)
	require.NoError(t, err)
	require.Len(t, second.Data.Events, 5)
	require.Empty(t, second.Data.NextCursor)
	require.Equal(t, "5", second.Data.Events[0].EventID)
	require.Equal(t, "1", second.Data.Events[4].EventID)

	_, err = svc.ListAuditEvents(managementContext(7), 41, "invalid-cursor", 20)
	require.True(t, IsClientError(err))

	unauthenticated := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		AuditRepository:     auditRepo,
	})
	_, err = unauthenticated.ListAuditEvents(context.Background(), 41, "", 20)
	require.ErrorIs(t, err, ErrMCPUnauthenticated)

	crossSpace := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(99),
		AuditRepository:     auditRepo,
	})
	_, err = crossSpace.ListAuditEvents(managementContext(7), 41, "", 20)
	require.ErrorIs(t, err, ErrMCPForbidden)
	_, err = crossSpace.ListAuditEvents(managementContext(7), 404, "", 20)
	require.ErrorIs(t, err, ErrMCPForbidden)
}

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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestApplicationGuardrailAuditRecorderPersistsContentFreeDecision(
	t *testing.T,
) {
	repo := &recordingGuardrailAuditRepository{}
	recorder := NewApplicationGuardrailAuditRecorder(
		ApplicationGuardrailAuditRecorderOptions{
			Repository: repo,
			IDGen:      &mcpWorkdirLeaseSequenceIDGen{next: 9101},
			NowMillis:  func() int64 { return 2000 },
		},
	)

	err := recorder.RecordGuardrailDecision(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:search_docs",
			Operation:  "invoke",
			Source:     "adk_tool_wrapper",
			FailMode:   GuardrailFailClosed,
			Metadata: map[string]string{
				"raw_prompt": "secret prompt",
			},
		},
		GuardrailDecision{
			Action:     GuardrailActionConfirm,
			Provider:   " scanner provider /mnt/raw ",
			ReasonCode: " high risk reason ",
			Message:    "Review secret sk-secret from /mnt/raw/object",
			RuleIDs: []string{
				" rule:high_risk ",
				"",
				strings.Repeat("x", 120),
			},
			Metadata: map[string]string{
				"object_uri": "s3://bucket/raw",
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, &domainentity.GuardrailAuditEvent{
		ID:         9101,
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
		Provider:   "scanner_provider_mnt_raw",
		ReasonCode: "high_risk_reason",
		RuleIDs:    "rule:high_risk," + strings.Repeat("x", 64),
		CreatedAt:  2000,
	}, repo.createdEvent)

	serialized := strings.Join([]string{
		repo.createdEvent.EventType,
		repo.createdEvent.TargetType,
		repo.createdEvent.TargetID,
		repo.createdEvent.Operation,
		repo.createdEvent.Source,
		repo.createdEvent.Action,
		repo.createdEvent.FailMode,
		repo.createdEvent.Provider,
		repo.createdEvent.ReasonCode,
		repo.createdEvent.RuleIDs,
	}, " ")
	require.NotContains(t, serialized, "sk-secret")
	require.NotContains(t, serialized, "/mnt/raw")
	require.NotContains(t, serialized, "secret prompt")
	require.NotContains(t, serialized, "s3://")
}

func TestApplicationGuardrailAuditRecorderRedactsUnsafeTargetAndErrors(
	t *testing.T,
) {
	repo := &recordingGuardrailAuditRepository{}
	recorder := NewApplicationGuardrailAuditRecorder(
		ApplicationGuardrailAuditRecorderOptions{
			Repository: repo,
			IDGen:      &mcpWorkdirLeaseSequenceIDGen{next: 9102},
			NowMillis:  func() int64 { return 3000 },
		},
	)

	err := recorder.RecordGuardrailDecision(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetNetwork,
			TargetID:   "https://internal.example/path?token=sk-secret",
			Operation:  "fetch url",
			Source:     "web fetch /mnt/raw",
			FailMode:   GuardrailFailOpen,
		},
		GuardrailDecision{
			Action:     GuardrailActionDeny,
			Provider:   "network scanner",
			ReasonCode: "blocked private url",
		},
	)

	require.NoError(t, err)
	require.Equal(t, "redacted", repo.createdEvent.TargetID)
	require.Equal(t, "fetch_url", repo.createdEvent.Operation)
	require.Equal(t, "web_fetch_mnt_raw", repo.createdEvent.Source)

	broken := NewApplicationGuardrailAuditRecorder(
		ApplicationGuardrailAuditRecorderOptions{},
	)
	err = broken.RecordGuardrailDecision(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetNetwork,
			TargetID:   "https://internal.example/path?token=sk-secret",
			Operation:  "fetch",
			FailMode:   GuardrailFailClosed,
		},
		GuardrailDecision{Action: GuardrailActionDeny},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "guardrail audit record failed")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "internal.example")
}

func TestApplicationListGuardrailAuditEventsReturnsMetadataOnlyPage(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{
		listedEvents: []*domainentity.GuardrailAuditEvent{
			{
				ID:         9103,
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
				CreatedAt:  4000,
			},
		},
		listTotal: 2,
	}
	authorizer := &recordingGuardrailAuditAuthorizer{}
	app := &ApplicationService{
		GuardrailAuditRepository: repo,
		GuardrailAuditAuthorizer: authorizer,
	}

	resp, err := app.ListGuardrailAuditEvents(
		context.Background(),
		&ListGuardrailAuditEventsRequest{
			ThreadID: 10,
			RunID:    20,
			ViewerID: 40,
			Page:     2,
			PageSize: 1,
		},
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailAuditAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(40), authorizer.req.ViewerID)
	require.Equal(t, int64(20), repo.listReq.RunID)
	require.Equal(t, int64(10), repo.listReq.ThreadID)
	require.Equal(t, int32(1), repo.listReq.Limit)
	require.Equal(t, int32(1), repo.listReq.Offset)
	require.Equal(t, int64(2), resp.Total)
	require.Len(t, resp.Events, 1)
	require.Equal(t, int64(9103), resp.Events[0].EventID)
	require.Equal(t, "tool_call", resp.Events[0].TargetType)
	require.Equal(t, "runtime_tool:search_docs", resp.Events[0].TargetID)
	require.Equal(t, "guardrail.decision.confirm", resp.Events[0].EventType)

	serialized := strings.Join([]string{
		resp.Events[0].EventType,
		resp.Events[0].TargetType,
		resp.Events[0].TargetID,
		resp.Events[0].Operation,
		resp.Events[0].Source,
		resp.Events[0].Action,
		resp.Events[0].FailMode,
		resp.Events[0].Provider,
		resp.Events[0].ReasonCode,
		resp.Events[0].RuleIDs,
	}, " ")
	require.NotContains(t, serialized, "prompt")
	require.NotContains(t, serialized, "tool_args")
	require.NotContains(t, serialized, "s3://")
	require.NotContains(t, serialized, "checkpoint")
}

func TestApplicationExportGuardrailAuditEventsReturnsMetadataOnlyPage(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{
		listedEvents: []*domainentity.GuardrailAuditEvent{
			{
				ID:         9104,
				SpaceID:    30,
				ThreadID:   10,
				RunID:      20,
				ActorID:    40,
				EventType:  "guardrail.decision.deny",
				TargetType: "network",
				TargetID:   "web_fetch",
				Operation:  "invoke",
				Source:     "adk_runtime_tool",
				Action:     "deny",
				FailMode:   "fail_closed",
				Provider:   "http_scanner",
				ReasonCode: "network_review",
				RuleIDs:    "url_review,external_policy",
				CreatedAt:  5000,
			},
		},
		listTotal: 7,
	}
	authorizer := &recordingGuardrailAuditAuthorizer{}
	app := &ApplicationService{
		GuardrailAuditRepository: repo,
		GuardrailAuditAuthorizer: authorizer,
	}

	resp, err := app.ExportGuardrailAuditEvents(
		context.Background(),
		&ExportGuardrailAuditEventsRequest{
			ThreadID: 10,
			RunID:    20,
			ViewerID: 40,
			Page:     2,
			PageSize: 3,
		},
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailAuditExportSchema, resp.Schema)
	require.Equal(t, int64(10), resp.ThreadID)
	require.Positive(t, resp.ExportedAt)
	require.Equal(t, int32(2), resp.Page)
	require.Equal(t, int32(3), resp.PageSize)
	require.Equal(t, int64(7), resp.Total)
	require.Equal(t, GuardrailAuditAccessOperationExport, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(40), authorizer.req.ViewerID)
	require.Equal(t, int64(20), repo.listReq.RunID)
	require.Equal(t, int64(10), repo.listReq.ThreadID)
	require.Equal(t, int32(3), repo.listReq.Limit)
	require.Equal(t, int32(3), repo.listReq.Offset)
	require.Len(t, resp.Events, 1)
	require.Equal(t, int64(9104), resp.Events[0].EventID)
	require.Equal(t, "network", resp.Events[0].TargetType)
	require.Equal(t, "web_fetch", resp.Events[0].TargetID)

	serialized := strings.Join([]string{
		resp.Events[0].EventType,
		resp.Events[0].TargetType,
		resp.Events[0].TargetID,
		resp.Events[0].Operation,
		resp.Events[0].Source,
		resp.Events[0].Action,
		resp.Events[0].FailMode,
		resp.Events[0].Provider,
		resp.Events[0].ReasonCode,
		resp.Events[0].RuleIDs,
	}, " ")
	require.NotContains(t, serialized, "secret prompt")
	require.NotContains(t, serialized, "tool_args")
	require.NotContains(t, serialized, "s3://")
	require.NotContains(t, serialized, "checkpoint")
	require.NotContains(t, serialized, "provider_raw")
}

func TestApplicationExportGuardrailAuditEventsDeniesUnauthorizedViewerBeforeRepository(
	t *testing.T,
) {
	repo := &recordingGuardrailAuditRepository{}
	authorizer := &recordingGuardrailAuditAuthorizer{
		err: ErrGuardrailAuditAccessDenied,
	}
	app := &ApplicationService{
		GuardrailAuditRepository: repo,
		GuardrailAuditAuthorizer: authorizer,
	}

	_, err := app.ExportGuardrailAuditEvents(
		context.Background(),
		&ExportGuardrailAuditEventsRequest{
			ThreadID: 10,
			RunID:    20,
			ViewerID: 41,
			Page:     1,
			PageSize: 100,
		},
	)

	require.ErrorIs(t, err, ErrGuardrailAuditAccessDenied)
	require.Equal(t, GuardrailAuditAccessOperationExport, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(41), authorizer.req.ViewerID)
	require.Equal(t, 0, repo.listCalls)
}

func TestApplicationListGuardrailAuditEventsDeniesUnauthorizedViewerBeforeRepository(
	t *testing.T,
) {
	repo := &recordingGuardrailAuditRepository{}
	authorizer := &recordingGuardrailAuditAuthorizer{
		err: ErrGuardrailAuditAccessDenied,
	}
	app := &ApplicationService{
		GuardrailAuditRepository: repo,
		GuardrailAuditAuthorizer: authorizer,
	}

	_, err := app.ListGuardrailAuditEvents(
		context.Background(),
		&ListGuardrailAuditEventsRequest{
			ThreadID: 10,
			RunID:    20,
			ViewerID: 41,
			Page:     1,
			PageSize: 20,
		},
	)

	require.ErrorIs(t, err, ErrGuardrailAuditAccessDenied)
	require.Equal(t, GuardrailAuditAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(41), authorizer.req.ViewerID)
	require.Equal(t, 0, repo.listCalls)
}

func TestApplicationListGuardrailAuditEventsRequiresThreadIDBeforeRepository(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{}
	app := &ApplicationService{
		GuardrailAuditRepository: repo,
	}

	_, err := app.ListGuardrailAuditEvents(
		context.Background(),
		&ListGuardrailAuditEventsRequest{
			RunID:    20,
			Page:     1,
			PageSize: 20,
		},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "thread id is required")
	require.Equal(t, 0, repo.listCalls)
}

func TestThreadOwnerGuardrailAuditAuthorizerAllowsOnlyThreadCreator(t *testing.T) {
	threadSVC := &recordingThreadService{
		got: &domainentity.Thread{
			ID:        10,
			CreatorID: 40,
		},
	}
	authorizer := NewThreadOwnerGuardrailAuditAuthorizer(threadSVC)

	err := authorizer.AuthorizeGuardrailAuditAccess(
		context.Background(),
		GuardrailAuditAccessRequest{
			ThreadID:  10,
			RunID:     20,
			ViewerID:  40,
			Operation: GuardrailAuditAccessOperationList,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(10), threadSVC.getID)

	err = authorizer.AuthorizeGuardrailAuditAccess(
		context.Background(),
		GuardrailAuditAccessRequest{
			ThreadID:  10,
			RunID:     20,
			ViewerID:  41,
			Operation: GuardrailAuditAccessOperationList,
		},
	)

	require.ErrorIs(t, err, ErrGuardrailAuditAccessDenied)
}

type recordingGuardrailAuditRepository struct {
	createdEvent        *domainentity.GuardrailAuditEvent
	listedEvents        []*domainentity.GuardrailAuditEvent
	listBeforeEvents    []*domainentity.GuardrailAuditEvent
	listReq             domainrepo.ListGuardrailAuditEventsRequest
	listBeforeReq       domainrepo.ListGuardrailAuditEventsBeforeRequest
	deleteBeforeReq     domainrepo.DeleteGuardrailAuditEventsBeforeRequest
	deleteByIDsReq      domainrepo.DeleteGuardrailAuditEventsByIDsRequest
	listTotal           int64
	listBeforeTotal     int64
	deleteBeforeDeleted int64
	deleteByIDsDeleted  int64
	createCalls         int
	listCalls           int
	listBeforeCalls     int
	deleteBeforeCalls   int
	deleteByIDsCalls    int
	createErr           error
	listErr             error
	listBeforeErr       error
	deleteBeforeErr     error
	deleteByIDsErr      error
}

func (r *recordingGuardrailAuditRepository) CreateGuardrailAuditEvent(
	ctx context.Context,
	event *domainentity.GuardrailAuditEvent,
) error {
	r.createCalls++
	if event != nil {
		cloned := *event
		r.createdEvent = &cloned
	}
	return r.createErr
}

func (r *recordingGuardrailAuditRepository) ListGuardrailAuditEvents(
	ctx context.Context,
	req domainrepo.ListGuardrailAuditEventsRequest,
) ([]*domainentity.GuardrailAuditEvent, int64, error) {
	r.listCalls++
	r.listReq = req
	return r.listedEvents, r.listTotal, r.listErr
}

func (r *recordingGuardrailAuditRepository) ListGuardrailAuditEventsBefore(
	ctx context.Context,
	req domainrepo.ListGuardrailAuditEventsBeforeRequest,
) ([]*domainentity.GuardrailAuditEvent, int64, error) {
	r.listBeforeCalls++
	r.listBeforeReq = req
	return r.listBeforeEvents, r.listBeforeTotal, r.listBeforeErr
}

func (r *recordingGuardrailAuditRepository) DeleteGuardrailAuditEventsBefore(
	ctx context.Context,
	req domainrepo.DeleteGuardrailAuditEventsBeforeRequest,
) (int64, error) {
	r.deleteBeforeCalls++
	r.deleteBeforeReq = req
	return r.deleteBeforeDeleted, r.deleteBeforeErr
}

func (r *recordingGuardrailAuditRepository) DeleteGuardrailAuditEventsByIDs(
	ctx context.Context,
	req domainrepo.DeleteGuardrailAuditEventsByIDsRequest,
) (int64, error) {
	r.deleteByIDsCalls++
	r.deleteByIDsReq = req
	return r.deleteByIDsDeleted, r.deleteByIDsErr
}

type recordingGuardrailAuditAuthorizer struct {
	req GuardrailAuditAccessRequest
	err error
}

func (a *recordingGuardrailAuditAuthorizer) AuthorizeGuardrailAuditAccess(
	ctx context.Context,
	req GuardrailAuditAccessRequest,
) error {
	a.req = req
	return a.err
}

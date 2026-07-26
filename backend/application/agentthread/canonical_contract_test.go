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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestCanonicalExtensionsKeepLegacyCreateTaskThreadDefaults(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdThreadRunMessage: &domainservice.CreateThreadRunMessageResult{
			Thread: &entity.Thread{
				ID: 10, SpaceID: 1, CreatorID: 2, Title: "legacy defaults",
				Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			},
			Run: &entity.Run{
				ID: 20, ThreadID: 10, SpaceID: 1, CreatorID: 2,
				RunKind: entity.RunKindTask, Status: entity.RunStatusPending,
			},
			Message: &entity.Message{
				ID: 30, ThreadID: 10, RunID: 20,
				Role: entity.MessageRoleUser, Content: "legacy defaults",
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	_, err := app.CreateTaskThread(context.Background(), &CreateTaskThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Message: "legacy defaults",
	})

	require.NoError(t, err)
	require.NotNil(t, domainSVC.createThreadRunMessageReq)
	require.Equal(t, entity.ThreadSourceWeb, domainSVC.createThreadRunMessageReq.Thread.Source)
	require.JSONEq(t, `{"source":"workbench_new_task"}`, domainSVC.createThreadRunMessageReq.Thread.Metadata)
	require.Empty(t, domainSVC.createThreadRunMessageReq.Run.Metadata)
	require.JSONEq(t, `{"source":"workbench_new_task"}`, domainSVC.createThreadRunMessageReq.Message.Metadata)
}

func TestCanonicalCreateTaskThreadPersistsInitialWithSeparateThreadMetadata(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdThreadRunMessage: &domainservice.CreateThreadRunMessageResult{
			Thread: &entity.Thread{
				ID: 10, SpaceID: 1, CreatorID: 2, Title: "canonical initial",
				Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceAPI,
				Metadata: `{"origin":"canonical"}`,
			},
			Run: &entity.Run{
				ID: 20, ThreadID: 10, SpaceID: 1, CreatorID: 2,
				RunKind: entity.RunKindTask, Status: entity.RunStatusPending,
				Metadata: `{"origin":"run"}`,
			},
			Message: &entity.Message{
				ID: 30, ThreadID: 10, RunID: 20,
				Role: entity.MessageRoleUser, Content: "canonical initial",
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.CreateTaskThread(context.Background(), &CreateTaskThreadRequest{
		SpaceID:        1,
		UserID:         2,
		Message:        "canonical initial",
		ThreadMetadata: `{"origin":"canonical"}`,
		ThreadSource:   ThreadSourceAPI,
		Metadata:       `{"origin":"run"}`,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Thread)
	require.NotNil(t, resp.Message)
	require.NotNil(t, resp.Run)
	require.Nil(t, domainSVC.createReq)
	require.NotNil(t, domainSVC.createThreadRunMessageReq)
	require.Equal(t, entity.ThreadSourceAPI, domainSVC.createThreadRunMessageReq.Thread.Source)
	require.JSONEq(t, `{"origin":"canonical"}`, domainSVC.createThreadRunMessageReq.Thread.Metadata)
	require.JSONEq(t, `{"origin":"run"}`, domainSVC.createThreadRunMessageReq.Run.Metadata)
	require.JSONEq(t, `{"source":"workbench_new_task"}`, domainSVC.createThreadRunMessageReq.Message.Metadata)
}

func TestCanonicalCreateTaskThreadDeferredValidatesBeforeCreatingOnlyThread(t *testing.T) {
	domainSVC := &recordingThreadService{
		created: &entity.Thread{
			ID: 10, SpaceID: 1, CreatorID: 2, Title: "创建技能",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceAPI,
			Metadata: `{"origin":"canonical"}`,
		},
	}
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}
	app := &ApplicationService{ThreadSVC: domainSVC, RuntimePolicy: &policy}

	resp, err := app.CreateTaskThread(context.Background(), &CreateTaskThreadRequest{
		SpaceID:        1,
		UserID:         2,
		Message:        "我想创建一个技能，请先询问我技能用途、使用场景和期望输出",
		DeferStart:     true,
		ThreadMetadata: `{"origin":"canonical"}`,
		ThreadSource:   ThreadSourceAPI,
		Config: `{
			"runtime":"eino_adk",
			"mode":"pro",
			"skills":{"enabled":true,"allowed_skills":["skill-creator"]}
		}`,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Thread)
	require.Nil(t, resp.Message)
	require.Nil(t, resp.Run)
	require.NotNil(t, domainSVC.createReq)
	require.Equal(t, "创建技能", domainSVC.createReq.Title)
	require.Equal(t, entity.ThreadSourceAPI, domainSVC.createReq.Source)
	require.JSONEq(t, `{"origin":"canonical"}`, domainSVC.createReq.Metadata)
	require.Nil(t, domainSVC.createThreadRunMessageReq)
	require.Nil(t, domainSVC.createRunReq)
	require.Nil(t, domainSVC.appendReq)
}

func TestCanonicalCreateTaskThreadInvalidDeferredConfigHasNoPersistence(t *testing.T) {
	domainSVC := &recordingThreadService{}
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: false}
	app := &ApplicationService{ThreadSVC: domainSVC, RuntimePolicy: &policy}

	resp, err := app.CreateTaskThread(context.Background(), &CreateTaskThreadRequest{
		SpaceID:        1,
		UserID:         2,
		Message:        "invalid canonical deferred",
		DeferStart:     true,
		ThreadMetadata: `{"origin":"canonical"}`,
		ThreadSource:   ThreadSourceAPI,
		Config:         `{"runtime":"eino_adk"}`,
	})

	require.Nil(t, resp)
	require.ErrorContains(t, err, "eino adk runtime is disabled by server policy")
	require.Nil(t, domainSVC.createReq)
	require.Nil(t, domainSVC.createThreadRunMessageReq)
	require.Nil(t, domainSVC.createRunReq)
	require.Nil(t, domainSVC.appendReq)
}

func TestCanonicalApplicationSearchThreadsUsesAuthoritativeViewer(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{},
		searchThreads: []*entity.Thread{{
			ID: 10, SpaceID: 20, CreatorID: 30, Title: "canonical",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		}},
		searchThreadsTotal: 1,
	}
	workspaceAuthorizer := &recordingWorkspaceAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           domainSVC,
		ThreadAuthorizer:    &recordingThreadAuthorizer{},
		WorkspaceAuthorizer: workspaceAuthorizer,
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})
	status := ThreadStatusIdle

	resp, err := app.SearchThreads(ctx, &SearchThreadsRequest{
		SpaceID: 20,
		UserID:  999,
		IDs:     []int64{10},
		Status:  &status,
		Metadata: map[string]any{
			"source": "canonical",
		},
		SortBy: "thread_id", SortOrder: "asc",
		Page: CanonicalPage{Offset: 7, Limit: 3},
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Equal(t, int64(10), resp.Threads[0].ThreadID)
	require.NotNil(t, domainSVC.searchThreadsReq)
	require.Equal(t, int64(30), domainSVC.searchThreadsReq.UserID)
	require.Equal(t, int64(20), domainSVC.searchThreadsReq.SpaceID)
	require.Equal(t, domainservice.CanonicalPage{Offset: 7, Limit: 3}, domainSVC.searchThreadsReq.Page)
	require.Len(t, workspaceAuthorizer.requests, 1)
	require.Equal(t, WorkspaceAccessRequest{ViewerID: 30, SpaceID: 20}, workspaceAuthorizer.requests[0])
}

func TestCanonicalApplicationQueryMethodsAuthorizeAndProject(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got:    &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			gotRun: &entity.Run{ID: 40, ThreadID: 10, SpaceID: 20, CreatorID: 30},
		},
		searchRuns:         []*entity.Run{{ID: 40, ThreadID: 10}},
		searchRunsTotal:    1,
		events:             []*entity.RunEvent{{ID: 50, ThreadID: 10, RunID: 40}},
		eventsHasMore:      true,
		checkpoints:        []*entity.Checkpoint{{ID: 60, ThreadID: 10, RunID: 40}},
		checkpointsHasMore: true,
	}
	threadAuthorizer := &recordingThreadAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           domainSVC,
		ThreadAuthorizer:    threadAuthorizer,
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})

	runs, err := app.SearchRuns(ctx, &SearchRunsRequest{
		ThreadID: 10, Page: CanonicalPage{Limit: 3},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), runs.Total)
	require.Equal(t, int64(40), runs.Runs[0].RunID)

	events, err := app.ListRunEventsByCursor(ctx, &ListRunEventsByCursorRequest{
		ThreadID: 10, RunID: 40, AfterEventID: 49,
		EventTypes: []string{"message"}, Limit: 2,
	})
	require.NoError(t, err)
	require.True(t, events.HasMore)
	require.Equal(t, int64(50), events.Events[0].EventID)

	checkpoints, err := app.ListCheckpointsBefore(ctx, &ListCheckpointsBeforeRequest{
		ThreadID: 10, BeforeCheckpointID: 70, Limit: 2,
	})
	require.NoError(t, err)
	require.True(t, checkpoints.HasMore)
	require.Equal(t, int64(60), checkpoints.Checkpoints[0].CheckpointID)
	require.NotEmpty(t, threadAuthorizer.requests)
}

func TestCanonicalApplicationPatchThreadAuthorizesAndProjects(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
		patchedThread: &entity.Thread{
			ID: 10, SpaceID: 20, CreatorID: 30, Title: "new",
			Metadata: `{"custom":true}`, UpdatedAt: 100,
		},
	}
	threadAuthorizer := &recordingThreadAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           domainSVC,
		ThreadAuthorizer:    threadAuthorizer,
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})
	title := "new"

	resp, err := app.PatchThread(ctx, &PatchThreadRequest{
		ThreadID: 10, Title: &title,
		MetadataPatch: map[string]any{"custom": true}, UpdatedAt: 100,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), resp.Thread.ThreadID)
	require.Equal(t, "new", resp.Thread.Title)
	require.Equal(t, 1, domainSVC.patchThreadCalls)
	require.Equal(t, int64(10), domainSVC.patchThreadReq.ThreadID)
	require.Len(t, threadAuthorizer.requests, 1)
}

func TestCanonicalApplicationPatchThreadRejectsUnauthorizedBeforeMutation(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
	}
	app := &ApplicationService{
		ThreadSVC: domainSVC,
		ThreadAuthorizer: &recordingThreadAuthorizer{
			err: ErrThreadAccessDenied,
		},
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})
	title := "new"

	resp, err := app.PatchThread(ctx, &PatchThreadRequest{ThreadID: 10, Title: &title})

	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Zero(t, domainSVC.patchThreadCalls)
}

func TestCanonicalApplicationDeleteThreadIfIdleAuthorizesAndPreservesActiveRunError(t *testing.T) {
	t.Run("deletes idle thread", func(t *testing.T) {
		domainSVC := &recordingCanonicalQueryThreadService{
			recordingThreadService: &recordingThreadService{
				got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			},
			deleteThreadIfIdleOK: true,
		}
		threadAuthorizer := &recordingThreadAuthorizer{}
		app := &ApplicationService{
			ThreadSVC:           domainSVC,
			ThreadAuthorizer:    threadAuthorizer,
			WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
		}
		ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})

		resp, err := app.DeleteThreadIfIdle(ctx, &DeleteThreadIfIdleRequest{ThreadID: 10})

		require.NoError(t, err)
		require.True(t, resp.Deleted)
		require.Equal(t, 1, domainSVC.deleteThreadIfIdleCalls)
		require.Equal(t, int64(10), domainSVC.deleteThreadIfIdleReq.ThreadID)
		require.Len(t, threadAuthorizer.requests, 1)
	})

	t.Run("preserves active run error", func(t *testing.T) {
		domainSVC := &recordingCanonicalQueryThreadService{
			recordingThreadService: &recordingThreadService{
				got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			},
			deleteThreadIfIdleErr: ErrActiveRunExists,
		}
		app := &ApplicationService{
			ThreadSVC:           domainSVC,
			ThreadAuthorizer:    &recordingThreadAuthorizer{},
			WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
		}
		ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})

		resp, err := app.DeleteThreadIfIdle(ctx, &DeleteThreadIfIdleRequest{ThreadID: 10})

		require.Nil(t, resp)
		require.ErrorIs(t, err, ErrActiveRunExists)
		require.Equal(t, 1, domainSVC.deleteThreadIfIdleCalls)
	})
}

func TestCanonicalApplicationDeleteThreadIfIdleRejectsUnauthorizedBeforeMutation(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
	}
	app := &ApplicationService{
		ThreadSVC: domainSVC,
		ThreadAuthorizer: &recordingThreadAuthorizer{
			err: ErrThreadAccessDenied,
		},
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})

	resp, err := app.DeleteThreadIfIdle(ctx, &DeleteThreadIfIdleRequest{ThreadID: 10})

	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Zero(t, domainSVC.deleteThreadIfIdleCalls)
}

func TestCanonicalApplicationUpdatePublicThreadStateCreatesIsolatedCheckpoint(t *testing.T) {
	previous := &entity.Checkpoint{
		ID: 60, ThreadID: 10, RunID: 30,
		CheckpointNS: "canonical.public", RuntimeType: "canonical_public_state",
		RuntimeKey: "thread:10", ChannelValues: `{"custom":{"keep":true,"old":1}}`,
	}
	created := &entity.Checkpoint{
		ID: 70, ThreadID: 10, RunID: 40, ParentCheckpointID: 60,
		CheckpointNS: "canonical.public", RuntimeType: "canonical_public_state",
		RuntimeKey: "thread:10", ChannelValues: `{"custom":{"keep":false,"old":1,"new":"value"}}`,
		ChannelVersions: `{}`, PendingSends: `[]`,
		Metadata: `{"source":"canonical_public_state","as_node":"editor"}`,
	}
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got:        &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			checkpoint: previous, checkpoints: []*entity.Checkpoint{previous}, checkpointTotal: 1,
			createdCheckpoint: created,
		},
		searchRuns:      []*entity.Run{{ID: 40, ThreadID: 10, ParentRunID: 0, CreatedAt: 200}},
		searchRunsTotal: 1,
		preparedPublicState: &domainservice.PreparedPublicThreadState{
			ChannelValues: created.ChannelValues,
			Metadata:      created.Metadata,
		},
	}
	threadAuthorizer := &recordingThreadAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           domainSVC,
		ThreadAuthorizer:    threadAuthorizer,
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})
	values := map[string]any{
		"custom": map[string]any{"keep": false, "new": "value"},
	}

	resp, err := app.UpdatePublicThreadState(ctx, &UpdatePublicThreadStateRequest{
		ThreadID: 10, BaseCheckpointID: 60, Values: values, AsNode: "editor",
	})

	require.NoError(t, err)
	require.Equal(t, int64(70), resp.Checkpoint.CheckpointID)
	require.Equal(t, int64(10), domainSVC.getID)
	require.NotNil(t, domainSVC.searchRunsReq)
	require.Equal(t, int64(10), domainSVC.searchRunsReq.ThreadID)
	require.Nil(t, domainSVC.searchRunsReq.ParentRunID)
	require.Equal(t, domainservice.CanonicalPage{Limit: 1}, domainSVC.searchRunsReq.Page)
	require.Equal(t, &domainservice.GetCheckpointRequest{CheckpointID: 60}, domainSVC.getCheckpointReq)
	require.Equal(t, &domainservice.ListCheckpointsRequest{
		ThreadID: 10, RuntimeType: "canonical_public_state", Limit: 1,
	}, domainSVC.listCheckpointsReq)
	require.Equal(t, 1, domainSVC.preparePublicStateCalls)
	require.Equal(t, values, domainSVC.preparePublicStateReq.Values)
	require.Equal(t, previous.ChannelValues, domainSVC.preparePublicStateReq.PreviousChannelValues)
	require.Equal(t, "editor", domainSVC.preparePublicStateReq.AsNode)
	require.Equal(t, &domainservice.CreateCheckpointRequest{
		ThreadID: 10, RunID: 40, ParentCheckpointID: 60,
		CheckpointNS: "canonical.public", RuntimeType: "canonical_public_state",
		RuntimeKey: "thread:10", EnvelopeVersion: 0,
		ChannelValues: created.ChannelValues, ChannelVersions: `{}`, PendingSends: `[]`,
		Metadata: created.Metadata,
	}, domainSVC.createCheckpointReq)
	require.Nil(t, domainSVC.getLatestRuntimeReq)
	require.Nil(t, domainSVC.deleteRuntimeReq)
	require.Len(t, threadAuthorizer.requests, 1)
}

func TestCanonicalApplicationUpdatePublicThreadStateUsesLatestCheckpointAsParent(t *testing.T) {
	latest := &entity.Checkpoint{ID: 55, ThreadID: 10, RunID: 40, RuntimeType: "eino_adk"}
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got:               &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			latestCheckpoint:  latest,
			createdCheckpoint: &entity.Checkpoint{ID: 70, ThreadID: 10, RunID: 40, ParentCheckpointID: 55},
		},
		searchRuns:      []*entity.Run{{ID: 40, ThreadID: 10}},
		searchRunsTotal: 1,
		preparedPublicState: &domainservice.PreparedPublicThreadState{
			ChannelValues: `{"custom":{"visible":true}}`, Metadata: `{}`,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.UpdatePublicThreadState(context.Background(), &UpdatePublicThreadStateRequest{
		ThreadID: 10, Values: map[string]any{"custom": map[string]any{"visible": true}},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Checkpoint)
	require.Equal(t, &domainservice.GetLatestCheckpointRequest{ThreadID: 10}, domainSVC.getLatestCheckpointReq)
	require.Equal(t, int64(55), domainSVC.createCheckpointReq.ParentCheckpointID)
}

func TestCanonicalApplicationUpdatePublicThreadStateReturnsStableConflictWithoutRun(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.UpdatePublicThreadState(context.Background(), &UpdatePublicThreadStateRequest{
		ThreadID: 10, Values: map[string]any{"custom": map[string]any{"visible": true}},
	})

	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrPublicThreadStateConflict)
	require.Zero(t, domainSVC.preparePublicStateCalls)
	require.Nil(t, domainSVC.createCheckpointReq)
}

func TestCanonicalApplicationUpdatePublicThreadStateRejectsForeignBaseCheckpoint(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got:        &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			checkpoint: &entity.Checkpoint{ID: 60, ThreadID: 11, RunID: 40},
		},
		searchRuns:      []*entity.Run{{ID: 40, ThreadID: 10}},
		searchRunsTotal: 1,
	}
	app := &ApplicationService{
		ThreadSVC:           domainSVC,
		ThreadAuthorizer:    &recordingThreadAuthorizer{},
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})

	resp, err := app.UpdatePublicThreadState(ctx, &UpdatePublicThreadStateRequest{
		ThreadID: 10, BaseCheckpointID: 60,
		Values: map[string]any{"custom": map[string]any{"visible": true}},
	})

	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Zero(t, domainSVC.preparePublicStateCalls)
	require.Nil(t, domainSVC.createCheckpointReq)
}

func TestCanonicalApplicationUpdatePublicThreadStatePreservesUnsupportedChannelError(t *testing.T) {
	domainSVC := &recordingCanonicalQueryThreadService{
		recordingThreadService: &recordingThreadService{
			got: &entity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
		searchRuns:            []*entity.Run{{ID: 40, ThreadID: 10}},
		searchRunsTotal:       1,
		preparePublicStateErr: domainservice.ErrUnsupportedPublicStateChannel,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.UpdatePublicThreadState(context.Background(), &UpdatePublicThreadStateRequest{
		ThreadID: 10, Values: map[string]any{"messages": []any{}},
	})

	require.Nil(t, resp)
	require.ErrorIs(t, err, ErrUnsupportedPublicStateChannel)
	require.Nil(t, domainSVC.createCheckpointReq)
}

type recordingCanonicalQueryThreadService struct {
	*recordingThreadService
	searchThreads           []*entity.Thread
	searchThreadsTotal      int64
	searchThreadsReq        *domainservice.SearchThreadsRequest
	searchRuns              []*entity.Run
	searchRunsTotal         int64
	searchRunsReq           *domainservice.SearchRunsRequest
	events                  []*entity.RunEvent
	eventsHasMore           bool
	eventsReq               *domainservice.ListRunEventsByCursorRequest
	checkpoints             []*entity.Checkpoint
	checkpointsHasMore      bool
	checkpointsReq          *domainservice.ListCheckpointsBeforeRequest
	patchedThread           *entity.Thread
	patchThreadReq          *domainservice.PatchThreadRequest
	patchThreadCalls        int
	deleteThreadIfIdleOK    bool
	deleteThreadIfIdleErr   error
	deleteThreadIfIdleReq   *domainservice.DeleteThreadIfIdleRequest
	deleteThreadIfIdleCalls int
	preparedPublicState     *domainservice.PreparedPublicThreadState
	preparePublicStateErr   error
	preparePublicStateReq   *domainservice.PreparePublicThreadStateRequest
	preparePublicStateCalls int
}

func (s *recordingCanonicalQueryThreadService) SearchThreads(
	_ context.Context,
	req *domainservice.SearchThreadsRequest,
) ([]*entity.Thread, int64, error) {
	s.searchThreadsReq = req
	return s.searchThreads, s.searchThreadsTotal, nil
}

func (s *recordingCanonicalQueryThreadService) SearchRuns(
	_ context.Context,
	req *domainservice.SearchRunsRequest,
) ([]*entity.Run, int64, error) {
	s.searchRunsReq = req
	return s.searchRuns, s.searchRunsTotal, nil
}

func (s *recordingCanonicalQueryThreadService) ListRunEventsByCursor(
	_ context.Context,
	req *domainservice.ListRunEventsByCursorRequest,
) ([]*entity.RunEvent, bool, error) {
	s.eventsReq = req
	return s.events, s.eventsHasMore, nil
}

func (s *recordingCanonicalQueryThreadService) ListCheckpointsBefore(
	_ context.Context,
	req *domainservice.ListCheckpointsBeforeRequest,
) ([]*entity.Checkpoint, bool, error) {
	s.checkpointsReq = req
	return s.checkpoints, s.checkpointsHasMore, nil
}

func (s *recordingCanonicalQueryThreadService) PatchThread(
	_ context.Context,
	req *domainservice.PatchThreadRequest,
) (*entity.Thread, error) {
	s.patchThreadCalls++
	s.patchThreadReq = req
	return s.patchedThread, nil
}

func (s *recordingCanonicalQueryThreadService) DeleteThreadIfIdle(
	_ context.Context,
	req *domainservice.DeleteThreadIfIdleRequest,
) (bool, error) {
	s.deleteThreadIfIdleCalls++
	s.deleteThreadIfIdleReq = req
	return s.deleteThreadIfIdleOK, s.deleteThreadIfIdleErr
}

func (s *recordingCanonicalQueryThreadService) PreparePublicThreadState(
	_ context.Context,
	req *domainservice.PreparePublicThreadStateRequest,
) (*domainservice.PreparedPublicThreadState, error) {
	s.preparePublicStateCalls++
	s.preparePublicStateReq = req
	return s.preparedPublicState, s.preparePublicStateErr
}

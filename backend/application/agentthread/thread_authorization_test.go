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

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestThreadOwnerAuthorizerAllowsOwnerInMatchingSpace(t *testing.T) {
	threadSVC := &threadAuthorizationServiceStub{
		thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		run:    &domainentity.Run{ID: 40, ThreadID: 10},
	}
	authorizer := NewThreadOwnerAuthorizer(threadSVC)

	err := authorizer.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		SpaceID:  20,
		ThreadID: 10,
		RunID:    40,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), threadSVC.gotThreadID)
	require.Equal(t, int64(40), threadSVC.gotRunID)
}

func TestThreadOwnerAuthorizerRejectsInvalidOwnerSpaceAndRun(t *testing.T) {
	tests := []struct {
		name string
		req  ThreadAccessRequest
		run  *domainentity.Run
	}{
		{
			name: "missing viewer",
			req:  ThreadAccessRequest{SpaceID: 20, ThreadID: 10},
		},
		{
			name: "missing space",
			req:  ThreadAccessRequest{ViewerID: 30, ThreadID: 10},
		},
		{
			name: "missing thread",
			req:  ThreadAccessRequest{ViewerID: 30, SpaceID: 20},
		},
		{
			name: "different owner",
			req:  ThreadAccessRequest{ViewerID: 31, SpaceID: 20, ThreadID: 10},
		},
		{
			name: "different space",
			req:  ThreadAccessRequest{ViewerID: 30, SpaceID: 21, ThreadID: 10},
		},
		{
			name: "run belongs to another thread",
			req:  ThreadAccessRequest{ViewerID: 30, SpaceID: 20, ThreadID: 10, RunID: 40},
			run:  &domainentity.Run{ID: 40, ThreadID: 11},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewThreadOwnerAuthorizer(&threadAuthorizationServiceStub{
				thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
				run:    tt.run,
			})

			err := authorizer.AuthorizeThreadAccess(context.Background(), tt.req)

			require.ErrorIs(t, err, ErrThreadAccessDenied)
		})
	}
}

func TestThreadOwnerAuthorizerMasksMissingResources(t *testing.T) {
	notFound := errors.New("record not found: internal id 10")
	tests := []struct {
		name      string
		thread    *domainentity.Thread
		threadErr error
		run       *domainentity.Run
		runErr    error
	}{
		{name: "thread absent"},
		{name: "thread lookup error", threadErr: notFound},
		{
			name:   "run absent",
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
		{
			name:   "run lookup error",
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			runErr: notFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewThreadOwnerAuthorizer(&threadAuthorizationServiceStub{
				thread:    tt.thread,
				threadErr: tt.threadErr,
				run:       tt.run,
				runErr:    tt.runErr,
			})

			err := authorizer.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
				ViewerID: 30,
				SpaceID:  20,
				ThreadID: 10,
				RunID:    40,
			})

			require.ErrorIs(t, err, ErrThreadAccessDenied)
			require.NotErrorIs(t, err, notFound)
			require.NotContains(t, err.Error(), "internal id")
		})
	}
}

func TestUserSpaceWorkspaceAuthorizerRequiresCurrentMembership(t *testing.T) {
	authorizer := NewUserSpaceWorkspaceAuthorizer(&userSpaceReaderStub{
		member: true,
	})

	require.NoError(t, authorizer.AuthorizeWorkspaceAccess(
		context.Background(),
		WorkspaceAccessRequest{ViewerID: 30, SpaceID: 20},
	))
	authorizer.UserSpaceReader = &userSpaceReaderStub{member: false}
	require.ErrorIs(t, authorizer.AuthorizeWorkspaceAccess(
		context.Background(),
		WorkspaceAccessRequest{ViewerID: 30, SpaceID: 99},
	), ErrThreadAccessDenied)
	require.ErrorIs(t, NewUserSpaceWorkspaceAuthorizer(nil).AuthorizeWorkspaceAccess(
		context.Background(),
		WorkspaceAccessRequest{ViewerID: 30, SpaceID: 20},
	), ErrThreadAuthorizationUnavailable)
	lookupCause := errors.New("database secret detail")
	lookupErr := NewUserSpaceWorkspaceAuthorizer(&userSpaceReaderStub{
		err: lookupCause,
	})
	err := lookupErr.AuthorizeWorkspaceAccess(
		context.Background(),
		WorkspaceAccessRequest{ViewerID: 30, SpaceID: 20},
	)
	require.ErrorIs(t, err, ErrThreadAuthorizationUnavailable)
	require.ErrorIs(t, err, lookupCause)
}

func TestApplicationAuthorizeThreadAccessResolvesTrustedSpace(t *testing.T) {
	threadSVC := &threadAuthorizationServiceStub{
		thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
	}
	authorizer := &recordingThreadAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           threadSVC,
		ThreadAuthorizer:    authorizer,
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}

	err := app.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, ThreadAccessRequest{
		ViewerID: 30,
		SpaceID:  20,
		ThreadID: 10,
	}, authorizer.requests[0])
}

func TestApplicationAccessDeniedWithoutThreadAuthorizer(t *testing.T) {
	app := &ApplicationService{
		ThreadSVC: &threadAuthorizationServiceStub{
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
	}

	err := app.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
	})

	require.ErrorIs(t, err, ErrThreadAccessDenied)
}

func TestApplicationAccessDeniedWithoutWorkspaceAuthorizer(t *testing.T) {
	app := &ApplicationService{
		ThreadSVC: &threadAuthorizationServiceStub{
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
		ThreadAuthorizer: &recordingThreadAuthorizer{},
	}

	err := app.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
	})

	require.ErrorIs(t, err, ErrThreadAccessDenied)
}

func TestApplicationAuthorizeThreadAccessRejectsViewerOutsideWorkspace(t *testing.T) {
	app := &ApplicationService{
		ThreadSVC: &threadAuthorizationServiceStub{
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
		ThreadAuthorizer: &recordingThreadAuthorizer{},
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{
			err: ErrThreadAccessDenied,
		},
	}

	err := app.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
	})

	require.ErrorIs(t, err, ErrThreadAccessDenied)
}

func TestApplicationCreateThreadRejectsUnauthorizedWorkspaceBeforeMutation(t *testing.T) {
	threadSVC := &recordingThreadService{
		created: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
	}
	app := &ApplicationService{
		ThreadSVC:        threadSVC,
		ThreadAuthorizer: &recordingThreadAuthorizer{},
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{
			err: ErrThreadAccessDenied,
		},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{ViewerID: 30})

	_, err := app.CreateThread(ctx, &CreateThreadRequest{
		SpaceID: 20,
		UserID:  999,
		Title:   "must not persist",
	})

	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Nil(t, threadSVC.createReq)
}

func TestApplicationAuthenticatedContextWithoutMarkerStillAuthorizes(t *testing.T) {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &entity.Session{UserID: 30})
	threadAuthorizer := &recordingThreadAuthorizer{}
	workspaceAuthorizer := &recordingWorkspaceAuthorizer{}
	app := &ApplicationService{
		ThreadSVC: &threadAuthorizationServiceStub{
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		},
		ThreadAuthorizer:    threadAuthorizer,
		WorkspaceAuthorizer: workspaceAuthorizer,
	}

	_, err := app.GetThread(ctx, &GetThreadRequest{ThreadID: 10})

	require.NoError(t, err)
	require.Len(t, threadAuthorizer.requests, 1)
	require.Equal(t, int64(30), threadAuthorizer.requests[0].ViewerID)
	require.Equal(t, int64(20), threadAuthorizer.requests[0].SpaceID)
	require.Len(t, workspaceAuthorizer.requests, 1)
}

func TestApplicationAuthorizeThreadAccessLoadsResourcesOnce(t *testing.T) {
	threadSVC := &threadAuthorizationServiceStub{
		thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		run:    &domainentity.Run{ID: 40, ThreadID: 10},
	}
	app := &ApplicationService{
		ThreadSVC:           threadSVC,
		ThreadAuthorizer:    NewThreadOwnerAuthorizer(threadSVC),
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}

	err := app.AuthorizeThreadAccess(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
		RunID:    40,
	})

	require.NoError(t, err)
	require.Equal(t, 1, threadSVC.getThreadCalls)
	require.Equal(t, 1, threadSVC.getRunCalls)
}

func TestApplicationAuthorizeThreadAccessReusesRequestScope(t *testing.T) {
	threadSVC := &threadAuthorizationServiceStub{
		thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		run:    &domainentity.Run{ID: 40, ThreadID: 10},
	}
	workspaceAuthorizer := &recordingWorkspaceAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           threadSVC,
		ThreadAuthorizer:    NewThreadOwnerAuthorizer(threadSVC),
		WorkspaceAuthorizer: workspaceAuthorizer,
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
		RunID:    40,
	})

	require.NoError(t, app.AuthorizeThreadAccess(ctx, ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
		RunID:    40,
	}))
	require.NoError(t, app.AuthorizeThreadAccess(ctx, ThreadAccessRequest{
		ViewerID: 30,
		ThreadID: 10,
		RunID:    40,
	}))

	require.Equal(t, 1, threadSVC.getThreadCalls)
	require.Equal(t, 1, threadSVC.getRunCalls)
	require.Len(t, workspaceAuthorizer.requests, 1)
}

func TestApplicationAuthorizeThreadAccessRejectsNegativeIdentifiers(t *testing.T) {
	tests := []ThreadAccessRequest{
		{ViewerID: 30, SpaceID: -1, ThreadID: 10},
		{ViewerID: 30, ThreadID: -1, RunID: 40},
	}
	for _, req := range tests {
		threadSVC := &threadAuthorizationServiceStub{
			thread: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
			run:    &domainentity.Run{ID: 40, ThreadID: 10},
		}
		app := &ApplicationService{
			ThreadSVC:        threadSVC,
			ThreadAuthorizer: &recordingThreadAuthorizer{},
		}

		err := app.AuthorizeThreadAccess(context.Background(), req)

		require.ErrorIs(t, err, ErrThreadAccessDenied)
	}
}

func TestApplicationResourceFamiliesAuthorizesThreadAccess(t *testing.T) {
	tests := []struct {
		name      string
		threadID  int64
		runID     int64
		operation func(context.Context, *ApplicationService) error
	}{
		{
			name:     "thread read",
			threadID: 10,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.GetThread(ctx, &GetThreadRequest{ThreadID: 10})
				return err
			},
		},
		{
			name:     "thread mutation",
			threadID: 10,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.UpdateThreadMetadata(ctx, &UpdateThreadMetadataRequest{
					ThreadID: 10,
					Metadata: `{}`,
				})
				return err
			},
		},
		{
			name:     "message read",
			threadID: 10,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.ListMessages(ctx, &ListMessagesRequest{ThreadID: 10})
				return err
			},
		},
		{
			name:     "message mutation",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.AppendMessage(ctx, &AppendMessageRequest{
					ThreadID: 10,
					RunID:    40,
					Role:     MessageRoleUser,
					Content:  "hello",
				})
				return err
			},
		},
		{
			name:     "run read",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.GetRun(ctx, &GetRunRequest{RunID: 40})
				return err
			},
		},
		{
			name:     "run idempotency read",
			threadID: 10,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.GetRunByIdempotencyKey(ctx, &GetRunByIdempotencyKeyRequest{
					ThreadID: 10, IdempotencyKey: "canonical-replay-1",
				})
				return err
			},
		},
		{
			name:     "run creation",
			threadID: 10,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.CreateRun(ctx, &CreateRunRequest{ThreadID: 10, Input: `{}`})
				return err
			},
		},
		{
			name:     "checkpoint read",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.ListCheckpoints(ctx, &ListCheckpointsRequest{
					ThreadID: 10,
					RunID:    40,
				})
				return err
			},
		},
		{
			name:     "checkpoint creation",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.CreateCheckpoint(ctx, &CreateCheckpointRequest{
					ThreadID: 10,
					RunID:    40,
				})
				return err
			},
		},
		{
			name:     "event read",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.ListRunEvents(ctx, &ListRunEventsRequest{
					ThreadID: 10,
					RunID:    40,
				})
				return err
			},
		},
		{
			name:     "event mutation",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.AppendRunEvent(ctx, &AppendRunEventRequest{
					ThreadID:  10,
					RunID:     40,
					EventType: "test",
				})
				return err
			},
		},
		{
			name:     "token read",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.GetRunTokenUsage(ctx, &GetTokenUsageRequest{RunID: 40})
				return err
			},
		},
		{
			name:     "token mutation",
			threadID: 10,
			runID:    40,
			operation: func(ctx context.Context, app *ApplicationService) error {
				_, err := app.RecordTokenUsage(ctx, &RecordTokenUsageRequest{
					RunID:       40,
					TotalTokens: 1,
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threadSVC := authorizedOperationThreadService()
			authorizer := &recordingThreadAuthorizer{}
			app := &ApplicationService{
				ThreadSVC:           threadSVC,
				ThreadAuthorizer:    authorizer,
				WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
			}
			ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{
				ViewerID: 30,
				ThreadID: tt.threadID,
				RunID:    tt.runID,
			})

			err := tt.operation(ctx, app)

			require.NoError(t, err)
			require.NotEmpty(t, authorizer.requests)
			last := authorizer.requests[len(authorizer.requests)-1]
			require.Equal(t, int64(30), last.ViewerID)
			require.Equal(t, int64(20), last.SpaceID)
			require.Equal(t, tt.threadID, last.ThreadID)
			require.Equal(t, tt.runID, last.RunID)
		})
	}
}

func TestApplicationAccessDeniedPreventsMutation(t *testing.T) {
	threadSVC := authorizedOperationThreadService()
	app := &ApplicationService{
		ThreadSVC: threadSVC,
		ThreadAuthorizer: &recordingThreadAuthorizer{
			err: ErrThreadAccessDenied,
		},
		WorkspaceAuthorizer: &recordingWorkspaceAuthorizer{},
	}
	ctx := WithThreadAccessRequest(context.Background(), ThreadAccessRequest{
		ViewerID: 31,
		ThreadID: 10,
		RunID:    40,
	})

	_, err := app.AppendMessage(ctx, &AppendMessageRequest{
		ThreadID: 10,
		RunID:    40,
		Role:     MessageRoleUser,
		Content:  "must not persist",
	})

	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Nil(t, threadSVC.appendReq)
}

func authorizedOperationThreadService() *recordingThreadService {
	return &recordingThreadService{
		got: &domainentity.Thread{ID: 10, SpaceID: 20, CreatorID: 30},
		gotRun: &domainentity.Run{
			ID:       40,
			ThreadID: 10,
		},
		appended: &domainentity.Message{ID: 50, ThreadID: 10, RunID: 40},
		createdRun: &domainentity.Run{
			ID:       41,
			ThreadID: 10,
		},
		createdRunBundle: &domainservice.CreateRunBundleResult{
			Run: &domainentity.Run{ID: 41, ThreadID: 10},
		},
		createdCheckpoint: &domainentity.Checkpoint{ID: 60, ThreadID: 10, RunID: 40},
		appendedRunEvent:  &domainentity.RunEvent{ID: 70, ThreadID: 10, RunID: 40},
		recordedTokenUsage: &domainentity.TokenUsage{
			ID:    80,
			RunID: 40,
		},
	}
}

type recordingThreadAuthorizer struct {
	requests []ThreadAccessRequest
	err      error
}

type recordingWorkspaceAuthorizer struct {
	requests []WorkspaceAccessRequest
	err      error
}

func (a *recordingWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	_ context.Context,
	req WorkspaceAccessRequest,
) error {
	a.requests = append(a.requests, req)
	return a.err
}

func (a *recordingThreadAuthorizer) AuthorizeThreadAccess(
	_ context.Context,
	req ThreadAccessRequest,
) error {
	a.requests = append(a.requests, req)
	return a.err
}

type threadAuthorizationServiceStub struct {
	domainservice.ThreadService
	thread         *domainentity.Thread
	threadErr      error
	run            *domainentity.Run
	runErr         error
	gotThreadID    int64
	gotRunID       int64
	getThreadCalls int
	getRunCalls    int
}

type userSpaceReaderStub struct {
	member bool
	err    error
}

func (s *userSpaceReaderStub) IsSpaceMember(
	context.Context,
	int64,
	int64,
) (bool, error) {
	return s.member, s.err
}

func (s *threadAuthorizationServiceStub) GetThread(
	_ context.Context,
	threadID int64,
) (*domainentity.Thread, error) {
	s.getThreadCalls++
	s.gotThreadID = threadID
	return s.thread, s.threadErr
}

func (s *threadAuthorizationServiceStub) GetRun(
	_ context.Context,
	req *domainservice.GetRunRequest,
) (*domainentity.Run, error) {
	s.getRunCalls++
	if req != nil {
		s.gotRunID = req.RunID
	}
	return s.run, s.runErr
}

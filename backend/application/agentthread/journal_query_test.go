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

func TestGetJournalBootstrapReauthorizesAndPreservesFrozenBoundary(t *testing.T) {
	authorizer := &recordingThreadAuthorizer{}
	workspaceAuthorizer := &recordingWorkspaceAuthorizer{}
	repository := &journalQueryRepositoryStub{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{{
			ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att-1", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
			NextSequence: 4, SnapshotsEnabled: true,
			ProjectionState: domainentity.JournalProjectionStateHealthy,
		}},
		SelectedAttempt: &domainentity.RunAttempt{
			ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att-1", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
			NextSequence: 4, SnapshotsEnabled: true,
			ProjectionState: domainentity.JournalProjectionStateHealthy,
		},
		Events: []*domainentity.JournalEvent{{
			ID: 101, ThreadID: 1, RunID: 10, JournalRunID: 10,
			AttemptID: "att-1", Sequence: 3, Visibility: domainentity.JournalVisibilityUser,
		}},
		LatestSequence: 3, ResolvedAfterSequence: 2,
	}}
	service := &ApplicationService{
		ThreadAuthorizer:       authorizer,
		WorkspaceAuthorizer:    workspaceAuthorizer,
		JournalQueryRepository: repository,
	}

	result, err := service.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", AfterSequence: 2, Limit: 20,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, uint64(3), result.LatestSequence)
	require.Equal(t, uint64(2), result.ResolvedAfterSequence)
	require.Equal(t, "att-1", result.SelectedAttempt.AttemptID)
	require.True(t, result.JournalEnabled)
	require.True(t, result.SnapshotsEnabled)
	require.Equal(t, []JournalContentType{
		JournalContentTypeDocument,
		JournalContentTypeTerminal,
		JournalContentTypeCode,
		JournalContentTypeSkill,
		JournalContentTypeBrowser,
	}, result.ContentTypes)
	require.Equal(t, []ThreadAccessRequest{{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10,
	}}, authorizer.requests)
	require.Equal(t, []WorkspaceAccessRequest{{
		ViewerID: 2, SpaceID: 3,
	}}, workspaceAuthorizer.requests)
	require.Equal(t, domainrepo.GetJournalBootstrapRequest{
		RunID: 10, AttemptID: "att-1", AfterSequence: 2,
		AfterSequenceSet: true, Limit: 20,
	}, repository.req)
}

func TestHumanResumeJournalBootstrapSelectsSuccessorAttempt(t *testing.T) {
	sourceAttemptID := "attempt-source"
	sourceCheckpointID := int64(7001)
	recoveryKey := "human-resume:source:response-1"
	sourceStartedAt, sourceEndedAt := int64(1_000), int64(2_000)
	sourceTerminalEventID := int64(102)
	active := uint8(1)
	source := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: sourceAttemptID, Ordinal: 1,
		Status: domainentity.RunAttemptStatusInterrupted, NextSequence: 3,
		ProjectionState: domainentity.JournalProjectionStateHealthy,
		CreatedAt:       900, StartedAt: &sourceStartedAt, EndedAt: &sourceEndedAt,
		TerminalEventID: &sourceTerminalEventID,
	}
	target := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 20,
		AttemptID: "attempt-target", Ordinal: 2,
		Status: domainentity.RunAttemptStatusPending, ActiveSlot: &active,
		NextSequence: 1, ProjectionState: domainentity.JournalProjectionStateHealthy,
		CreatedAt: 2_000, SourceAttemptID: &sourceAttemptID,
		SourceCheckpointID: &sourceCheckpointID, RecoveryIdempotencyKey: &recoveryKey,
	}
	repository := &journalQueryRepositoryStub{result: &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{source, target}, SelectedAttempt: target,
	}}
	service := &ApplicationService{
		ThreadAuthorizer:       &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:    &recordingWorkspaceAuthorizer{},
		JournalQueryRepository: repository,
	}

	result, err := service.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10,
	})

	require.NoError(t, err)
	require.Len(t, result.Attempts, 2)
	require.Same(t, target, result.SelectedAttempt)
	require.Equal(t, domainentity.RunAttemptStatusInterrupted, result.Attempts[0].Status)
	require.True(t, result.Attempts[0].Status.IsTerminal())
	require.Equal(t, domainentity.RunAttemptStatusPending, result.SelectedAttempt.Status)
	require.Equal(t, sourceAttemptID, *result.SelectedAttempt.SourceAttemptID)
	require.Equal(t, sourceCheckpointID, *result.SelectedAttempt.SourceCheckpointID)
	require.Equal(t, recoveryKey, *result.SelectedAttempt.RecoveryIdempotencyKey)
	require.Empty(t, repository.req.AttemptID)
}

func TestGetJournalBootstrapStopsBeforeRepositoryWhenWorkspaceAccessIsRevoked(t *testing.T) {
	repository := &journalQueryRepositoryStub{result: journalQueryBootstrap(nil)}
	service := &ApplicationService{
		ThreadAuthorizer:       &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:    &recordingWorkspaceAuthorizer{err: ErrThreadAccessDenied},
		JournalQueryRepository: repository,
	}

	result, err := service.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10, AttemptID: "att-1",
	})
	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Nil(t, result)
	require.Zero(t, repository.req.RunID)
}

func TestGetJournalBootstrapDoesNotReuseStaleThreadAccessCache(t *testing.T) {
	threadService := &threadAuthorizationServiceStub{}
	repository := &journalQueryRepositoryStub{result: journalQueryBootstrap(nil)}
	service := &ApplicationService{
		ThreadAuthorizer:       NewThreadOwnerAuthorizer(threadService),
		WorkspaceAuthorizer:    &recordingWorkspaceAuthorizer{},
		JournalQueryRepository: repository,
	}
	ctx := context.WithValue(context.Background(), threadAccessContextKey{}, &threadAccessContextState{
		threads: map[int64]*domainentity.Thread{
			1: {ID: 1, SpaceID: 3, CreatorID: 2},
		},
		runs: map[int64]*domainentity.Run{
			10: {ID: 10, ThreadID: 1},
		},
	})

	result, err := service.GetJournalBootstrap(ctx, GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10, AttemptID: "att-1",
	})
	require.ErrorIs(t, err, ErrThreadAccessDenied)
	require.Nil(t, result)
	require.Equal(t, 1, threadService.getThreadCalls)
	require.Zero(t, repository.req.RunID)
}

func TestGetJournalBootstrapRejectsEntirePageWhenSnapshotSourceIsRevoked(t *testing.T) {
	digest := strings.Repeat("a", 64)
	event := &domainentity.JournalEvent{
		ID: 101, ThreadID: 1, RunID: 10, JournalRunID: 10,
		AttemptID: "att-1", Sequence: 1, EventType: "action.completed",
		SnapshotID: "snapshot-1", Visibility: domainentity.JournalVisibilityUser,
	}
	repository := &journalQueryRepositoryStub{result: journalQueryBootstrap(event)}
	artifactAuthorizer := &journalSnapshotArtifactAuthorizerStub{allowed: true}
	snapshotRepository := &journalSnapshotRepositoryStub{
		snapshots: map[string]*domainentity.JournalContentSnapshot{
			"snapshot-1": {
				SnapshotID: "snapshot-1", SpaceID: 3, ThreadID: 1,
				RunID: 10, JournalRunID: 10, AttemptID: "att-1", EventID: 101,
				Visibility:         domainentity.JournalVisibilityUser,
				SourceResourceType: "artifact", SourceResourceID: "701",
				SourceRevision: digest,
			},
		},
	}
	service := &ApplicationService{
		ThreadAuthorizer:          &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:       &recordingWorkspaceAuthorizer{},
		JournalQueryRepository:    repository,
		JournalSnapshotRepository: snapshotRepository,
		JournalSnapshotAuthorizer: &journalSnapshotAuthorizerStub{allowed: true},
		JournalSnapshotArtifactReader: &journalSnapshotArtifactReaderStub{
			artifact: &domainentity.AgentArtifact{
				ID: 701, SpaceID: 3, ThreadID: 1, RunID: 10,
				Metadata: `{"content_hash":"` + digest + `"}`,
			},
		},
		ArtifactAuthorizer: artifactAuthorizer,
	}

	request := GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10, AttemptID: "att-1",
	}
	result, err := service.GetJournalBootstrap(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	artifactAuthorizer.allowed = false
	result, err = service.GetJournalBootstrap(context.Background(), request)
	require.ErrorIs(t, err, ErrJournalSnapshotNoPermission)
	require.Nil(t, result)
}

func TestGetJournalBootstrapReauthorizesArtifactEventsWithoutSnapshots(t *testing.T) {
	event := &domainentity.JournalEvent{
		ID: 101, ThreadID: 1, RunID: 10, JournalRunID: 10,
		AttemptID: "att-1", Sequence: 1, EventType: "artifact.created",
		Payload:    `{"type":"artifact","data":{"artifact_id":"701"}}`,
		Visibility: domainentity.JournalVisibilityUser,
	}
	artifactAuthorizer := &journalSnapshotArtifactAuthorizerStub{allowed: true}
	service := &ApplicationService{
		ThreadAuthorizer:       &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:    &recordingWorkspaceAuthorizer{},
		JournalQueryRepository: &journalQueryRepositoryStub{result: journalQueryBootstrap(event)},
		JournalSnapshotArtifactReader: &journalSnapshotArtifactReaderStub{
			artifact: &domainentity.AgentArtifact{
				ID: 701, SpaceID: 3, ThreadID: 1, RunID: 10,
			},
		},
		ArtifactAuthorizer: artifactAuthorizer,
	}

	request := GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10, AttemptID: "att-1",
	}
	result, err := service.GetJournalBootstrap(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, result)

	artifactAuthorizer.allowed = false
	result, err = service.GetJournalBootstrap(context.Background(), request)
	require.ErrorIs(t, err, ErrJournalSnapshotNoPermission)
	require.Nil(t, result)
}

func journalQueryBootstrap(event *domainentity.JournalEvent) *domainrepo.GetJournalBootstrapResult {
	attempt := &domainentity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-1", Ordinal: 1, Status: domainentity.RunAttemptStatusRunning,
		NextSequence: 2, SnapshotsEnabled: true,
		ProjectionState: domainentity.JournalProjectionStateHealthy,
	}
	result := &domainrepo.GetJournalBootstrapResult{
		Attempts: []*domainentity.RunAttempt{attempt}, SelectedAttempt: attempt,
	}
	if event != nil {
		result.Events = []*domainentity.JournalEvent{event}
		result.LatestSequence = 1
	}
	return result
}

type journalQueryRepositoryStub struct {
	req    domainrepo.GetJournalBootstrapRequest
	result *domainrepo.GetJournalBootstrapResult
	err    error
}

func (r *journalQueryRepositoryStub) GetJournalBootstrap(
	_ context.Context,
	req domainrepo.GetJournalBootstrapRequest,
) (*domainrepo.GetJournalBootstrapResult, error) {
	r.req = req
	return r.result, r.err
}

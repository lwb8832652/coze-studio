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
	"time"

	"github.com/stretchr/testify/require"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

type journalConfigProviderStub struct {
	configuration *adminconfig.BasicConfiguration
	calls         int
}

func (s *journalConfigProviderStub) GetBaseConfig(context.Context) (*adminconfig.BasicConfiguration, error) {
	s.calls++
	return s.configuration, nil
}

func enabledJournalConfiguration() *adminconfig.BasicConfiguration {
	return &adminconfig.BasicConfiguration{
		JournalRuntimeConfiguration: &adminconfig.JournalRuntimeConfiguration{
			JournalProjection:                    true,
			JournalUI:                            true,
			JournalSnapshots:                     true,
			CheckpointRecovery:                   true,
			JournalProjectionRolloutBasisPoints:  10000,
			JournalUIRolloutBasisPoints:          10000,
			JournalSnapshotsRolloutBasisPoints:   10000,
			CheckpointRecoveryRolloutBasisPoints: 10000,
		},
	}
}

func disabledJournalConfiguration() *adminconfig.BasicConfiguration {
	configuration := enabledJournalConfiguration()
	configuration.JournalRuntimeConfiguration.JournalProjection = false
	configuration.JournalRuntimeConfiguration.JournalUI = false
	configuration.JournalRuntimeConfiguration.JournalSnapshots = false
	configuration.JournalRuntimeConfiguration.CheckpointRecovery = false
	return configuration
}

func TestJournalFeatureGateUsesStableSpaceBucketsAndRolloutBoundaries(t *testing.T) {
	provider := &journalConfigProviderStub{configuration: enabledJournalConfiguration()}
	gate := NewJournalFeatureGate(provider, JournalFeatureGateOptions{})

	first := JournalRolloutBucket(42, JournalFeatureProjection)
	second := JournalRolloutBucket(42, JournalFeatureProjection)
	require.Equal(t, first, second)
	require.GreaterOrEqual(t, first, int32(0))
	require.Less(t, first, int32(10000))
	require.NotEqual(t, first, JournalRolloutBucket(42, JournalFeatureUI))

	provider.configuration.JournalRuntimeConfiguration.JournalProjectionRolloutBasisPoints = 0
	enabled, err := gate.Enabled(context.Background(), JournalFeatureProjection, 42)
	require.NoError(t, err)
	require.False(t, enabled)

	provider.configuration.JournalRuntimeConfiguration.JournalProjectionRolloutBasisPoints = 10000
	gate.Invalidate()
	enabled, err = gate.Enabled(context.Background(), JournalFeatureProjection, 42)
	require.NoError(t, err)
	require.True(t, enabled)
}

func TestJournalFeatureGateRefreshesKillSwitchWithinThirtySeconds(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	provider := &journalConfigProviderStub{configuration: enabledJournalConfiguration()}
	gate := NewJournalFeatureGate(provider, JournalFeatureGateOptions{
		CacheTTL: 30 * time.Second,
		Now:      func() time.Time { return now },
	})

	enabled, err := gate.MasterEnabled(context.Background(), JournalFeatureProjection)
	require.NoError(t, err)
	require.True(t, enabled)
	require.Equal(t, 1, provider.calls)

	provider.configuration.JournalRuntimeConfiguration.JournalProjection = false
	now = now.Add(29 * time.Second)
	enabled, err = gate.MasterEnabled(context.Background(), JournalFeatureProjection)
	require.NoError(t, err)
	require.True(t, enabled)
	require.Equal(t, 1, provider.calls)

	now = now.Add(2 * time.Second)
	enabled, err = gate.MasterEnabled(context.Background(), JournalFeatureProjection)
	require.NoError(t, err)
	require.False(t, enabled)
	require.Equal(t, 2, provider.calls)
}

func TestJournalFeatureGateEnrollsOnlyProAndUltraRootTasks(t *testing.T) {
	gate := NewJournalFeatureGate(
		&journalConfigProviderStub{configuration: enabledJournalConfiguration()},
		JournalFeatureGateOptions{},
	)

	for _, test := range []struct {
		name     string
		mode     DeerFlowMode
		runKind  domainentity.RunKind
		parentID int64
		want     bool
	}{
		{name: "pro", mode: DeerFlowModePro, runKind: domainentity.RunKindTask, want: true},
		{name: "ultra", mode: DeerFlowModeUltra, runKind: domainentity.RunKindTask, want: true},
		{name: "child pro", mode: DeerFlowModePro, runKind: domainentity.RunKindTask, parentID: 9},
		{name: "subagent ultra", mode: DeerFlowModeUltra, runKind: domainentity.RunKindSubagent},
	} {
		t.Run(test.name, func(t *testing.T) {
			decision, err := gate.DecideEnrollment(context.Background(), JournalEnrollmentInput{
				SpaceID:     42,
				RunKind:     test.runKind,
				ParentRunID: test.parentID,
				RunConfig:   `{"runtime":"eino_adk","mode":"` + string(test.mode) + `"}`,
			})
			require.NoError(t, err)
			require.Equal(t, test.want, decision.Enrolled)
			if test.want {
				require.True(t, decision.SnapshotsEnabled)
				require.Equal(t, domainentity.JournalSchemaVersion, decision.EnrollmentVersion)
			}
		})
	}
}

func TestJournalFeatureGateEnrollsAutomaticRootTasks(t *testing.T) {
	gate := NewJournalFeatureGate(
		&journalConfigProviderStub{configuration: enabledJournalConfiguration()},
		JournalFeatureGateOptions{},
	)

	decision, err := gate.DecideEnrollment(context.Background(), JournalEnrollmentInput{
		SpaceID:   42,
		RunKind:   domainentity.RunKindTask,
		RunConfig: `{"runtime":"eino_adk","requested_policy":"auto"}`,
	})

	require.NoError(t, err)
	require.True(t, decision.Enrolled)
	require.True(t, decision.SnapshotsEnabled)
}

func TestApplicationCreateTaskThreadPersistsJournalEnrollmentInAtomicBundle(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdThreadRunMessage: &domainservice.CreateThreadRunMessageResult{
			Thread: &domainentity.Thread{ID: 10, SpaceID: 42, CreatorID: 2},
			Run: &domainentity.Run{
				ID: 20, ThreadID: 10, SpaceID: 42, CreatorID: 2,
				RunKind: domainentity.RunKindTask, Status: domainentity.RunStatusPending,
			},
			Message: &domainentity.Message{
				ID: 30, ThreadID: 10, RunID: 20, Role: domainentity.MessageRoleUser,
			},
		},
	}
	app := &ApplicationService{
		ThreadSVC: domainSVC,
		JournalFeatureGate: NewJournalFeatureGate(
			&journalConfigProviderStub{configuration: enabledJournalConfiguration()},
			JournalFeatureGateOptions{},
		),
	}

	_, err := app.CreateTaskThread(context.Background(), &CreateTaskThreadRequest{
		SpaceID: 42, UserID: 2, Message: "分析项目需求",
		Config: `{"runtime":"eino_adk","mode":"pro"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, domainSVC.createThreadRunMessageReq)
	require.True(t, domainSVC.createThreadRunMessageReq.EnrollJournal)
	require.NotNil(t, domainSVC.createThreadRunMessageReq.JournalEnrollment)
	require.Equal(
		t,
		domainentity.JournalSchemaVersion,
		domainSVC.createThreadRunMessageReq.JournalEnrollment.EnrollmentVersion,
	)
	require.True(t, domainSVC.createThreadRunMessageReq.JournalEnrollment.SnapshotsEnabled)
}

func TestJournalFeatureGateBlocksBootstrapBeforeRepository(t *testing.T) {
	repository := &journalQueryRepositoryStub{result: journalQueryBootstrap(nil)}
	service := &ApplicationService{
		ThreadAuthorizer:       &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:    &recordingWorkspaceAuthorizer{},
		JournalQueryRepository: repository,
		JournalFeatureGate: NewJournalFeatureGate(
			&journalConfigProviderStub{configuration: disabledJournalConfiguration()},
			JournalFeatureGateOptions{},
		),
	}

	result, err := service.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10,
	})
	require.ErrorIs(t, err, domainrepo.ErrJournalNotEnrolled)
	require.Nil(t, result)
	require.Zero(t, repository.req.RunID)
}

func TestJournalFeatureGateHeartbeatDisablesActiveAttemptButKeepsUIHistory(t *testing.T) {
	configuration := enabledJournalConfiguration()
	configuration.JournalRuntimeConfiguration.JournalProjection = false
	repository := &journalQueryRepositoryStub{result: journalQueryBootstrap(nil)}
	controller := &journalProjectionControllerStub{}
	service := &ApplicationService{
		ThreadAuthorizer:            &recordingThreadAuthorizer{},
		WorkspaceAuthorizer:         &recordingWorkspaceAuthorizer{},
		JournalQueryRepository:      repository,
		JournalProjectionController: controller,
		JournalFeatureGate: NewJournalFeatureGate(
			&journalConfigProviderStub{configuration: configuration},
			JournalFeatureGateOptions{},
		),
	}

	result, err := service.GetJournalBootstrap(context.Background(), GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.JournalEnabled)
	require.False(t, result.SnapshotsEnabled)
	require.Equal(t, domainentity.JournalProjectionStateDisabled, result.SelectedAttempt.ProjectionState)
	require.Equal(t, domainentity.JournalProjectionStateDisabled, result.Attempts[0].ProjectionState)
	require.Equal(t, int64(10), controller.runID)
}

func TestJournalFeatureGateBlocksSnapshotReadAndWrite(t *testing.T) {
	service, repository, _ := newJournalSnapshotApplicationTestService()
	service.JournalFeatureGate = NewJournalFeatureGate(
		&journalConfigProviderStub{configuration: disabledJournalConfiguration()},
		JournalFeatureGateOptions{},
	)

	view, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		ViewerID: 2, SpaceID: 3, ThreadID: 1, RunID: 10, SnapshotID: "snap-disabled",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)
	require.Nil(t, view)
	require.Empty(t, repository.audits)

	snapshot, event, err := service.SubmitJournalContent(context.Background(), JournalContentSubmission{
		SpaceID: 3, ThreadID: 1, RunID: 10, AttemptID: "att-1",
		IdempotencyKey: "snapshot-disabled", Status: domainentity.JournalContentStatusReady,
		ContentType: JournalContentTypeDocument,
		Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "blocked", Content: "blocked",
		}},
		Action: JournalContentAction{ActionID: "write-disabled", Operation: "write_file", Target: "blocked.md"},
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)
	require.Nil(t, snapshot)
	require.Nil(t, event)
	require.Empty(t, repository.reservations)
}

func TestJournalFeatureGateBlocksRecoveryBeforeRepository(t *testing.T) {
	service, _, _ := newJournalRecoveryTestService(t)
	service.JournalFeatureGate = NewJournalFeatureGate(
		&journalConfigProviderStub{configuration: disabledJournalConfiguration()},
		JournalFeatureGateOptions{},
	)

	result, err := service.RecoverJournal(context.Background(), RecoverJournalRequest{
		ViewerID: 9, SpaceID: 7, ThreadID: 42, RunID: 10,
		Action: JournalRecoveryActionResume, IdempotencyKey: "recovery-disabled",
	})
	require.ErrorIs(t, err, ErrJournalRecoveryDependencyMissing)
	require.Nil(t, result)
}

type journalProjectionControllerStub struct {
	runID int64
	err   error
}

func (s *journalProjectionControllerStub) DisableActiveJournalProjection(
	_ context.Context,
	runID int64,
	_ int64,
) (*domainentity.RunAttempt, bool, error) {
	s.runID = runID
	return nil, s.err == nil, s.err
}

func TestJournalFeatureGateDisablesActiveProjectionAndKeepsBaseEvent(t *testing.T) {
	domainSVC := &recordingThreadService{
		got: &domainentity.Thread{ID: 1, SpaceID: 3},
		appendedRunEvent: &domainentity.RunEvent{
			ID: 100, ThreadID: 1, RunID: 10, EventType: "tool.started",
		},
	}
	controller := &journalProjectionControllerStub{}
	service := &ApplicationService{
		ThreadSVC:                   domainSVC,
		JournalProjectionController: controller,
		JournalFeatureGate: NewJournalFeatureGate(
			&journalConfigProviderStub{configuration: disabledJournalConfiguration()},
			JournalFeatureGateOptions{},
		),
	}

	event, err := service.appendProjectedRunEvent(context.Background(), RunEvent{
		ThreadID: 1, RunID: 10, EventType: "tool.started", Payload: `{}`,
	})
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, int64(10), controller.runID)
	require.NotNil(t, domainSVC.appendRunEventReq)
	require.Nil(t, domainSVC.appendRunEventReq.Journal)
	require.False(t, domainSVC.appendRunEventReq.JournalProjectionFailed)
}

func TestJournalFeatureGateFailsClosedWhenProjectionStateCannotBeDisabled(t *testing.T) {
	domainSVC := &recordingThreadService{got: &domainentity.Thread{ID: 1, SpaceID: 3}}
	controller := &journalProjectionControllerStub{err: errors.New("database unavailable")}
	service := &ApplicationService{
		ThreadSVC:                   domainSVC,
		JournalProjectionController: controller,
		JournalFeatureGate: NewJournalFeatureGate(
			&journalConfigProviderStub{configuration: disabledJournalConfiguration()},
			JournalFeatureGateOptions{},
		),
	}

	event, err := service.appendProjectedRunEvent(context.Background(), RunEvent{
		ThreadID: 1, RunID: 10, EventType: "tool.started", Payload: `{}`,
	})
	require.Error(t, err)
	require.Nil(t, event)
	require.Nil(t, domainSVC.appendRunEventReq)
}

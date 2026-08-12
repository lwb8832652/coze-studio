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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveBootstrapCoordinatorCommitsFreshRootADKRun(t *testing.T) {
	attempt := freshAdaptiveBootstrapAttemptForTest()
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: attempt}
	repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	run := freshAdaptiveBootstrapRunForTest()
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    repo,
		IDGen:         ids,
		Now:           func() int64 { return 999 },
	})

	err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, 1, reader.calls)
	require.Len(t, repo.readRequests, 1)
	require.Equal(t, repository.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
	}, repo.readRequests[0])
	require.Equal(t, []int{3}, ids.counts)
	require.Len(t, repo.commitRequests, 1)
	req := repo.commitRequests[0]
	require.Equal(t, int64(10), req.ThreadID)
	require.Equal(t, int64(20), req.ExecutionRunID)
	require.Equal(t, int64(30), req.JournalRunID)
	require.Equal(t, "attempt-1", req.AttemptID)
	require.Equal(t, "worker-1", req.LeaseOwner)
	require.Equal(t, "lease-1", req.LeaseToken)
	require.Equal(t, uint64(4), req.Generation)
	require.Equal(t, int64(999), req.Now)
	require.Equal(t, int64(700), req.FactCreatedAt)
	require.Equal(t, []int64{101, 102, 103}, []int64{
		req.AdmissionEventID, req.DecisionEventID, req.CheckpointID,
	})
	require.Equal(t, adaptiveBootstrapStableKeyForTest("operation", run, attempt), req.OperationKey)
	require.Equal(t, adaptiveBootstrapStableKeyForTest("decision", run, attempt), req.Decision.DecisionID)
	require.Equal(t, entity.AdaptiveAdmissionSnapshot{
		Schema:             entity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled: false,
		Source:             entity.AdaptiveAdmissionSourceFresh,
		Capabilities: entity.AdaptiveCapabilities{
			PlanAllowed: true, HumanInteractionAllowed: true,
		},
		Limits: entity.AdaptiveLimits{
			MaxToolCalls: 24, MaxReplans: 2, MaxVerificationRepairs: 2,
			MaxConsecutiveNoProgress: 3, MaxActiveDurationSeconds: 1200,
		},
	}, req.Admission)
	require.Equal(t, entity.ExecutionDecisionExecute, req.Decision.Decision)
	require.Equal(t, entity.ExecutionShapeMultiStep, req.Decision.ExecutionShape)
	require.Equal(t, uint64(1), req.Decision.DecisionRevision)
	require.Equal(t, int64(20), *req.Decision.PlanScopeRunID)
	require.Equal(t, int64(700), req.Decision.CreatedAt)

	otherRepo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
	otherCoordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    otherRepo,
		IDGen:         &adaptiveBootstrapIDGeneratorStub{ids: []int64{201, 202, 203}},
		Now:           func() int64 { return 123456 },
	})
	otherRun := *run
	otherRun.LeaseOwner, otherRun.LeaseToken = "another-worker", "another-lease"
	require.NoError(t, otherCoordinator.Bootstrap(context.Background(), &otherRun))
	require.Equal(t, req.OperationKey, otherRepo.commitRequests[0].OperationKey)
	require.Equal(t, req.Decision.DecisionID, otherRepo.commitRequests[0].Decision.DecisionID)
}

func TestAdaptiveBootstrapCoordinatorReplaysBeforeAllocatingIDs(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{readResult: &repository.CommitAdaptiveExecutionBootstrapResult{
		Authority: repository.AdaptiveExecutionBootstrapAuthority{ExecutionGeneration: 4},
	}}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
	})

	err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.NoError(t, err)
	require.Equal(t, 1, reader.calls)
	require.Len(t, repo.readRequests, 1)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorRejectsReplayFromAnotherGeneration(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{readResult: &repository.CommitAdaptiveExecutionBootstrapResult{
		Authority: repository.AdaptiveExecutionBootstrapAuthority{ExecutionGeneration: 3},
	}}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    repo,
		IDGen:         ids,
		Now:           func() int64 { return 999 },
	})

	err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.ErrorContains(t, err, "generation does not match")
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorSkipsNotEnrolledRun(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{err: repository.ErrJournalNotEnrolled}
	repo := &adaptiveBootstrapRepositoryStub{}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
	})

	run := freshAdaptiveBootstrapRunForTest()
	run.ExecutionGeneration = 0
	run.LeaseOwner = ""
	run.LeaseToken = ""
	err := coordinator.Bootstrap(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, 1, reader.calls)
	require.Empty(t, repo.readRequests)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorRejectsUnsafeFreshAttemptBeforeWriting(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RunSummary, *entity.RunAttempt)
	}{
		{
			name: "source attempt lineage",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				value := "source-attempt"
				attempt.SourceAttemptID = &value
			},
		},
		{
			name: "source checkpoint lineage",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				value := int64(91)
				attempt.SourceCheckpointID = &value
			},
		},
		{
			name: "recovery lineage",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				value := "recover-1"
				attempt.RecoveryIdempotencyKey = &value
			},
		},
		{
			name:   "thread ownership",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) { attempt.ThreadID++ },
		},
		{
			name:   "execution ownership",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) { attempt.ExecutionRunID++ },
		},
		{
			name: "inactive attempt",
			mutate: func(_ *RunSummary, attempt *entity.RunAttempt) {
				attempt.Status = entity.RunAttemptStatusCompleted
			},
		},
		{
			name:   "missing lease",
			mutate: func(run *RunSummary, _ *entity.RunAttempt) { run.LeaseToken = "" },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := freshAdaptiveBootstrapRunForTest()
			attempt := freshAdaptiveBootstrapAttemptForTest()
			test.mutate(run, attempt)
			reader := &adaptiveBootstrapAttemptReaderStub{attempt: attempt}
			repo := &adaptiveBootstrapRepositoryStub{readErr: repository.ErrAdaptiveExecutionBootstrapNotFound}
			ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
			coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
				AttemptReader: reader, Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
			})

			err := coordinator.Bootstrap(context.Background(), run)

			require.Error(t, err)
			require.Empty(t, repo.readRequests)
			require.Empty(t, ids.counts)
			require.Empty(t, repo.commitRequests)
		})
	}
}

func freshAdaptiveBootstrapRunForTest() *RunSummary {
	return &RunSummary{
		ThreadID: 10, RunID: 20, RunKind: RunKindTask, Status: RunStatusRunning,
		LeaseOwner: "worker-1", LeaseToken: "lease-1", ExecutionGeneration: 4,
	}
}

func freshAdaptiveBootstrapAttemptForTest() *entity.RunAttempt {
	active := uint8(1)
	return &entity.RunAttempt{
		ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20, AttemptID: "attempt-1",
		Status: entity.RunAttemptStatusRunning, ActiveSlot: &active, CreatedAt: 700,
	}
}

func adaptiveBootstrapStableKeyForTest(
	kind string,
	run *RunSummary,
	attempt *entity.RunAttempt,
) string {
	input := strings.Join([]string{
		"workbench-adaptive-bootstrap.v1", kind,
		strconv.FormatInt(run.ThreadID, 10), strconv.FormatInt(run.RunID, 10),
		strconv.FormatInt(attempt.JournalRunID, 10), attempt.AttemptID,
		strconv.FormatUint(run.ExecutionGeneration, 10),
	}, "\x00")
	digest := sha256.Sum256([]byte(input))
	return "adaptive-" + kind + ":" + hex.EncodeToString(digest[:])
}

type adaptiveBootstrapAttemptReaderStub struct {
	attempt *entity.RunAttempt
	err     error
	calls   int
}

func (s *adaptiveBootstrapAttemptReaderStub) GetActiveJournalAttempt(
	context.Context,
	int64,
) (*entity.RunAttempt, error) {
	s.calls++
	return s.attempt, s.err
}

type adaptiveBootstrapRepositoryStub struct {
	readResult            *repository.CommitAdaptiveExecutionBootstrapResult
	readErr               error
	commitResult          *repository.CommitAdaptiveExecutionBootstrapResult
	commitErr             error
	returnNilCommitResult bool
	readRequests          []repository.ReadAdaptiveExecutionBootstrapRequest
	commitRequests        []repository.CommitAdaptiveExecutionBootstrapRequest
}

func (s *adaptiveBootstrapRepositoryStub) ReadAdaptiveExecutionBootstrap(
	_ context.Context,
	req repository.ReadAdaptiveExecutionBootstrapRequest,
) (*repository.CommitAdaptiveExecutionBootstrapResult, error) {
	s.readRequests = append(s.readRequests, req)
	return s.readResult, s.readErr
}

func (s *adaptiveBootstrapRepositoryStub) CommitAdaptiveExecutionBootstrap(
	_ context.Context,
	req repository.CommitAdaptiveExecutionBootstrapRequest,
) (*repository.CommitAdaptiveExecutionBootstrapResult, error) {
	s.commitRequests = append(s.commitRequests, req)
	if s.commitResult == nil && s.commitErr == nil && !s.returnNilCommitResult {
		s.commitResult = &repository.CommitAdaptiveExecutionBootstrapResult{}
	}
	return s.commitResult, s.commitErr
}

type adaptiveBootstrapIDGeneratorStub struct {
	ids    []int64
	err    error
	counts []int
}

func (s *adaptiveBootstrapIDGeneratorStub) GenMultiIDs(
	_ context.Context,
	counts int,
) ([]int64, error) {
	s.counts = append(s.counts, counts)
	return append([]int64(nil), s.ids...), s.err
}

func TestAdaptiveBootstrapCoordinatorSurfacesPreReadFailure(t *testing.T) {
	readErr := errors.New("read failed")
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{readErr: readErr}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader, Repository: repo, IDGen: &adaptiveBootstrapIDGeneratorStub{}, Now: func() int64 { return 999 },
	})

	err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.ErrorIs(t, err, readErr)
	require.Empty(t, repo.commitRequests)
}

func TestAdaptiveBootstrapCoordinatorRejectsMissingCommitResult(t *testing.T) {
	reader := &adaptiveBootstrapAttemptReaderStub{attempt: freshAdaptiveBootstrapAttemptForTest()}
	repo := &adaptiveBootstrapRepositoryStub{
		readErr:               repository.ErrAdaptiveExecutionBootstrapNotFound,
		returnNilCommitResult: true,
	}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: reader,
		Repository:    repo,
		IDGen:         &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}},
		Now:           func() int64 { return 999 },
	})

	err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())

	require.ErrorContains(t, err, "commit result is required")
	require.Len(t, repo.commitRequests, 1)
}

func TestAdaptiveBootstrapCoordinatorFuncRejectsNilFunction(t *testing.T) {
	var coordinator AdaptiveBootstrapCoordinator = AdaptiveBootstrapCoordinatorFunc(nil)

	require.NotPanics(t, func() {
		err := coordinator.Bootstrap(context.Background(), freshAdaptiveBootstrapRunForTest())
		require.ErrorContains(t, err, "coordinator function is required")
	})
}

func ExampleAdaptiveBootstrapCoordinator_stableIdentity() {
	run := freshAdaptiveBootstrapRunForTest()
	attempt := freshAdaptiveBootstrapAttemptForTest()
	fmt.Println(adaptiveBootstrapStableKeyForTest("operation", run, attempt) == adaptiveBootstrapStableKeyForTest("operation", run, attempt))
	// Output: true
}

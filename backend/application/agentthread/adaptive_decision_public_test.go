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
	"encoding/json"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/stretchr/testify/require"
)

func TestPublicAdaptiveExecutionMapsAllFourModesAndRedactsAuthority(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Admission.FeatureGateEnabled = true
	facts.Admission.Capabilities.HumanInteractionAllowed = true
	facts.Decision.SafeSummary = "Use the durable execution decision."

	tests := []struct {
		name     string
		decision entity.ExecutionDecisionKind
		shape    entity.ExecutionShape
		mode     PublicAdaptiveExecutionMode
		question *string
	}{
		{name: "direct", decision: entity.ExecutionDecisionDirect, mode: PublicAdaptiveExecutionModeDirect},
		{name: "single step", decision: entity.ExecutionDecisionExecute, shape: entity.ExecutionShapeSingleStep, mode: PublicAdaptiveExecutionModeSingleStep},
		{name: "multi step", decision: entity.ExecutionDecisionExecute, shape: entity.ExecutionShapeMultiStep, mode: PublicAdaptiveExecutionModeMultiStep},
		{name: "clarification", decision: entity.ExecutionDecisionClarification, mode: PublicAdaptiveExecutionModeClarification, question: stringPointer("Which repository should be changed?")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := facts.Decision
			decision.Decision = test.decision
			decision.ExecutionShape = test.shape
			decision.ClarificationQuestion = test.question
			if test.shape == entity.ExecutionShapeMultiStep {
				planScope := run.RunID
				decision.PlanScopeRunID = &planScope
			} else {
				decision.PlanScopeRunID = nil
			}

			got, err := ProjectPublicAdaptiveExecution(facts.Admission, decision)

			require.NoError(t, err)
			require.Equal(t, PublicAdaptiveExecutionSchemaV1, got.Schema)
			require.True(t, got.Enabled)
			require.Equal(t, test.mode, got.Mode)
			require.Equal(t, decision.SafeSummary, got.SafeSummary)
			require.Equal(t, test.question, got.ClarificationQuestion)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), decision.DecisionID)
			require.NotContains(t, string(encoded), decision.AttemptID)
			require.NotContains(t, string(encoded), "plan_scope")
			require.NotContains(t, string(encoded), "provider")
			require.NotContains(t, string(encoded), "tool")
		})
	}
}

func TestPublicAdaptiveExecutionRejectsCorruptDurablePair(t *testing.T) {
	run := freshAdaptiveBootstrapRunForTest()
	facts := adaptiveBootstrapFactsForRunTest(t, run)
	facts.Decision.SafeSummary = "/private/runtime/checkpoint"

	got, err := ProjectPublicAdaptiveExecution(facts.Admission, facts.Decision)

	require.Error(t, err)
	require.Nil(t, got)
}

func TestPublicRunProjectsOnlySafeAdaptiveExecution(t *testing.T) {
	question := "Which repository should be changed?"
	run := &RunSummary{
		RunID: 20, ThreadID: 10, SpaceID: 30, CreatorID: 40,
		RunKind: RunKindTask, Status: RunStatusPending, CreatedAt: 100, UpdatedAt: 101,
		AdaptiveExecution: &PublicAdaptiveExecutionSummary{
			Schema: PublicAdaptiveExecutionSchemaV1, Enabled: true,
			Mode:        PublicAdaptiveExecutionModeClarification,
			SafeSummary: "Need one safe clarification.", ClarificationQuestion: &question,
		},
	}

	got := ProjectPublicRun(run)
	require.NotNil(t, got)
	require.NotNil(t, got.AdaptiveExecution)
	require.Equal(t, *run.AdaptiveExecution, *got.AdaptiveExecution)
	run.AdaptiveExecution.SafeSummary = "mutated"
	*run.AdaptiveExecution.ClarificationQuestion = "mutated"
	require.Equal(t, "Need one safe clarification.", got.AdaptiveExecution.SafeSummary)
	require.Equal(t, "Which repository should be changed?", *got.AdaptiveExecution.ClarificationQuestion)
}

func TestGetListAndSearchRunsHydrateDurableAdaptiveExecution(t *testing.T) {
	domainRun := &entity.Run{
		ID: 20, ThreadID: 10, SpaceID: 30, CreatorID: 40,
		RunKind: entity.RunKindTask, Status: entity.RunStatusRunning,
		CreatedAt: 100, UpdatedAt: 101,
	}
	base := &recordingThreadService{
		gotRun: domainRun, runs: []*entity.Run{domainRun}, runTotal: 1,
	}
	threadSVC := &publicAdaptiveCanonicalThreadService{
		ThreadService: base, searchRuns: []*entity.Run{domainRun},
	}
	facts := adaptiveBootstrapFactsForRunTest(t, freshAdaptiveBootstrapRunForTest())
	facts.Admission.FeatureGateEnabled = true
	reader := &publicAdaptiveExecutionReaderStub{results: map[int64]*repository.CommitAdaptiveExecutionBootstrapResult{
		20: {Admission: facts.Admission, Decision: facts.Decision},
	}}
	app := &ApplicationService{
		ThreadSVC:                    threadSVC,
		AdaptiveBootstrapByRunReader: reader,
	}

	get, err := app.GetRun(context.Background(), &GetRunRequest{
		RunID: 20, IncludeAdaptiveExecution: true,
	})
	require.NoError(t, err)
	require.Equal(t, PublicAdaptiveExecutionModeMultiStep, get.Run.AdaptiveExecution.Mode)
	list, err := app.ListRuns(context.Background(), &ListRunsRequest{
		ThreadID: 10, IncludeAdaptiveExecution: true,
	})
	require.NoError(t, err)
	require.Len(t, list.Runs, 1)
	require.Equal(t, PublicAdaptiveExecutionModeMultiStep, list.Runs[0].AdaptiveExecution.Mode)
	search, err := app.SearchRuns(context.Background(), &SearchRunsRequest{ThreadID: 10})
	require.NoError(t, err)
	require.Len(t, search.Runs, 1)
	require.Equal(t, PublicAdaptiveExecutionModeMultiStep, search.Runs[0].AdaptiveExecution.Mode)
	require.Len(t, reader.requests, 3)
}

func TestRunHydrationOmitsMissingAndFailsClosedOnConflict(t *testing.T) {
	domainRun := &entity.Run{
		ID: 20, ThreadID: 10, SpaceID: 30, CreatorID: 40,
		RunKind: entity.RunKindTask, Status: entity.RunStatusPending,
	}
	base := &recordingThreadService{gotRun: domainRun}

	missing := &ApplicationService{
		ThreadSVC: base,
		AdaptiveBootstrapByRunReader: &publicAdaptiveExecutionReaderStub{
			errs: map[int64]error{20: repository.ErrAdaptiveExecutionBootstrapNotFound},
		},
	}
	response, err := missing.GetRun(context.Background(), &GetRunRequest{
		RunID: 20, IncludeAdaptiveExecution: true,
	})
	require.NoError(t, err)
	require.Nil(t, response.Run.AdaptiveExecution)

	conflict := &ApplicationService{
		ThreadSVC: base,
		AdaptiveBootstrapByRunReader: &publicAdaptiveExecutionReaderStub{
			errs: map[int64]error{20: repository.ErrAdaptiveExecutionBootstrapConflict},
		},
	}
	response, err = conflict.GetRun(context.Background(), &GetRunRequest{
		RunID: 20, IncludeAdaptiveExecution: true,
	})
	require.Nil(t, response)
	require.ErrorIs(t, err, repository.ErrAdaptiveExecutionBootstrapConflict)
}

type publicAdaptiveExecutionReaderStub struct {
	results  map[int64]*repository.CommitAdaptiveExecutionBootstrapResult
	errs     map[int64]error
	requests []repository.ReadAdaptiveExecutionBootstrapByRunRequest
}

func (s *publicAdaptiveExecutionReaderStub) ReadAdaptiveExecutionBootstrapByRun(
	_ context.Context,
	req repository.ReadAdaptiveExecutionBootstrapByRunRequest,
) (*repository.CommitAdaptiveExecutionBootstrapResult, error) {
	s.requests = append(s.requests, req)
	if err := s.errs[req.ExecutionRunID]; err != nil {
		return nil, err
	}
	return s.results[req.ExecutionRunID], nil
}

type publicAdaptiveCanonicalThreadService struct {
	domainservice.ThreadService
	searchRuns []*entity.Run
}

func (s *publicAdaptiveCanonicalThreadService) SearchThreads(
	context.Context,
	*domainservice.SearchThreadsRequest,
) ([]*entity.Thread, int64, error) {
	return nil, 0, nil
}

func (s *publicAdaptiveCanonicalThreadService) SearchRuns(
	context.Context,
	*domainservice.SearchRunsRequest,
) ([]*entity.Run, int64, error) {
	return s.searchRuns, int64(len(s.searchRuns)), nil
}

func (s *publicAdaptiveCanonicalThreadService) ListRunEventsByCursor(
	context.Context,
	*domainservice.ListRunEventsByCursorRequest,
) ([]*entity.RunEvent, int64, bool, error) {
	return nil, 0, false, nil
}

func (s *publicAdaptiveCanonicalThreadService) ListCheckpointsBefore(
	context.Context,
	*domainservice.ListCheckpointsBeforeRequest,
) ([]*entity.Checkpoint, bool, error) {
	return nil, false, nil
}

var _ repository.AdaptiveExecutionBootstrapByRunRepository = (*publicAdaptiveExecutionReaderStub)(nil)
var _ domainservice.CanonicalQueryService = (*publicAdaptiveCanonicalThreadService)(nil)

func stringPointer(value string) *string {
	return &value
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package aio

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"testing"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	lifecycleOldSentinel    = "newx-generation-0123456789abcdef0123456789abcdef"
	lifecycleWinnerSentinel = "newx-generation-fedcba9876543210fedcba9876543210"
)

type lifecycleRepositoryGet struct {
	state domainsandbox.AIOGenerationState
	err   error
}

type lifecycleRepository struct {
	gets        []lifecycleRepositoryGet
	getCalls    int
	casState    domainsandbox.AIOGenerationState
	casReplaced bool
	casErr      error
	casInputs   []domainsandbox.CompareAndReplaceAIOSentinelInput
}

func (repository *lifecycleRepository) GetAIOGeneration(_ context.Context, _ string) (domainsandbox.AIOGenerationState, error) {
	index := repository.getCalls
	repository.getCalls++
	if len(repository.gets) == 0 {
		return domainsandbox.AIOGenerationState{}, nil
	}
	if index >= len(repository.gets) {
		index = len(repository.gets) - 1
	}
	return repository.gets[index].state, repository.gets[index].err
}

func (repository *lifecycleRepository) CompareAndReplaceAIOSentinel(_ context.Context, input domainsandbox.CompareAndReplaceAIOSentinelInput) (domainsandbox.AIOGenerationState, bool, error) {
	repository.casInputs = append(repository.casInputs, input)
	return repository.casState, repository.casReplaced, repository.casErr
}

type lifecycleListResult struct {
	response *sandboxapi.ResponseActiveShellSessionsResult
	err      error
}

type lifecycleUpstream struct {
	healthErr       error
	healthCalls     int
	lists           []lifecycleListResult
	listCalls       int
	createResponse  *sandboxapi.ResponseShellCreateSessionResponse
	createErr       error
	createRequests  []*sandboxapi.ShellCreateSessionRequest
	viewResponse    *sandboxapi.ResponseShellViewResult
	viewErr         error
	viewRequests    []*sandboxapi.ShellViewRequest
	cleanupErr      error
	cleanupRequests []string
}

func (upstream *lifecycleUpstream) Health(context.Context) error {
	upstream.healthCalls++
	return upstream.healthErr
}

func (upstream *lifecycleUpstream) ListSessions(context.Context) (*sandboxapi.ResponseActiveShellSessionsResult, error) {
	index := upstream.listCalls
	upstream.listCalls++
	if len(upstream.lists) == 0 {
		return lifecycleSessionList(), nil
	}
	if index >= len(upstream.lists) {
		index = len(upstream.lists) - 1
	}
	return upstream.lists[index].response, upstream.lists[index].err
}

func (upstream *lifecycleUpstream) Create(_ context.Context, request *sandboxapi.ShellCreateSessionRequest) (*sandboxapi.ResponseShellCreateSessionResponse, error) {
	upstream.createRequests = append(upstream.createRequests, request)
	if upstream.createResponse != nil || upstream.createErr != nil {
		return upstream.createResponse, upstream.createErr
	}
	return lifecycleCreateResponse(value(request.Id)), nil
}

func (upstream *lifecycleUpstream) View(_ context.Context, request *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error) {
	upstream.viewRequests = append(upstream.viewRequests, request)
	if upstream.viewResponse != nil || upstream.viewErr != nil {
		return upstream.viewResponse, upstream.viewErr
	}
	return lifecycleViewResponse(request.Id), nil
}

func (upstream *lifecycleUpstream) Cleanup(_ context.Context, sessionID string) error {
	upstream.cleanupRequests = append(upstream.cleanupRequests, sessionID)
	return upstream.cleanupErr
}

func TestLifecycleRawHealthUnknownPreservesGenerationAndSkipsPersistence(t *testing.T) {
	repository := &lifecycleRepository{
		gets: []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}},
	}
	upstream := &lifecycleUpstream{lists: []lifecycleListResult{{response: lifecycleSessionList(lifecycleOldSentinel)}}}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	first, err := supervisor.Observe(context.Background())
	if err != nil || !first.Ready || first.Generation != 7 {
		t.Fatalf("initial Observe() = %#v, %v", first, err)
	}
	upstream.healthErr = errors.New("transport detail that must not escape")
	second, err := supervisor.Observe(context.Background())
	if !errors.Is(err, ErrLifecycleUnknown) {
		t.Fatalf("unknown health error = %v, want ErrLifecycleUnknown", err)
	}
	if second.Ready || second.State != LifecycleStateUnknown || second.Generation != 7 {
		t.Fatalf("unknown health snapshot = %#v", second)
	}
	if repository.getCalls != 1 || len(repository.casInputs) != 0 || len(upstream.createRequests) != 0 {
		t.Fatalf("unknown health mutated state: gets=%d cas=%d creates=%d", repository.getCalls, len(repository.casInputs), len(upstream.createRequests))
	}
	if err.Error() == upstream.healthErr.Error() {
		t.Fatal("unknown health exposed upstream error detail")
	}
}

func TestLifecycleSuccessfulListKeepsPresentSentinelAndGeneration(t *testing.T) {
	repository := &lifecycleRepository{gets: []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}}}
	upstream := &lifecycleUpstream{lists: []lifecycleListResult{{response: lifecycleSessionList(lifecycleOldSentinel)}}}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if err != nil || !snapshot.Ready || snapshot.State != LifecycleStateReady || snapshot.Generation != 7 {
		t.Fatalf("Observe() = %#v, %v", snapshot, err)
	}
	if len(repository.casInputs) != 0 || len(upstream.createRequests) != 0 || len(upstream.cleanupRequests) != 0 {
		t.Fatalf("present sentinel was replaced: cas=%d create=%d cleanup=%d", len(repository.casInputs), len(upstream.createRequests), len(upstream.cleanupRequests))
	}
	if text := snapshot.String(); text == "" || regexp.MustCompile(`[0-9a-f]{32}`).MatchString(text) {
		t.Fatalf("snapshot string exposed sentinel material: %q", text)
	}
}

func TestLifecycleInitialMissingCreatesVerifiesAndPublishesGenerationOne(t *testing.T) {
	repository := &lifecycleRepository{
		gets:        []lifecycleRepositoryGet{{}},
		casState:    lifecycleGeneration(1, lifecycleWinnerSentinel),
		casReplaced: true,
	}
	upstream := &lifecycleUpstream{lists: []lifecycleListResult{{response: lifecycleSessionList()}}}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if err != nil || !snapshot.Ready || snapshot.Generation != 1 {
		t.Fatalf("Observe() = %#v, %v", snapshot, err)
	}
	if len(upstream.createRequests) != 1 || len(upstream.viewRequests) != 1 || len(repository.casInputs) != 1 {
		t.Fatalf("candidate sequence counts: create=%d view=%d cas=%d", len(upstream.createRequests), len(upstream.viewRequests), len(repository.casInputs))
	}
	candidate := value(upstream.createRequests[0].Id)
	if !regexp.MustCompile(`^newx-generation-[0-9a-f]{32}$`).MatchString(candidate) {
		t.Fatalf("candidate ID = %q", candidate)
	}
	if value(upstream.createRequests[0].ExecDir) != "/mnt/user-data" || value(upstream.createRequests[0].PreserveSymlinks) {
		t.Fatalf("candidate create request = %#v", upstream.createRequests[0])
	}
	if upstream.viewRequests[0].Id != candidate {
		t.Fatalf("verified ID = %q, want candidate", upstream.viewRequests[0].Id)
	}
	input := repository.casInputs[0]
	if input.DeploymentID != "runner-dev-a" || input.ExpectedSentinelID != "" || input.CandidateSentinelID != candidate {
		t.Fatalf("CAS input = %#v", input)
	}
	if candidate != lifecycleWinnerSentinel {
		t.Fatalf("deterministic candidate = %q, want %q", candidate, lifecycleWinnerSentinel)
	}
}

func TestLifecycleConfirmedMissingReplacesExactlyOnceThroughRepositoryCAS(t *testing.T) {
	repository := &lifecycleRepository{
		gets:        []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}},
		casState:    lifecycleGeneration(8, lifecycleWinnerSentinel),
		casReplaced: true,
	}
	upstream := &lifecycleUpstream{lists: []lifecycleListResult{{response: lifecycleSessionList()}}}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if err != nil || snapshot.Generation != 8 || !snapshot.Ready {
		t.Fatalf("Observe() = %#v, %v", snapshot, err)
	}
	if len(repository.casInputs) != 1 || repository.casInputs[0].ExpectedSentinelID != lifecycleOldSentinel {
		t.Fatalf("replacement CAS inputs = %#v", repository.casInputs)
	}
	if len(upstream.cleanupRequests) != 0 {
		t.Fatalf("CAS winner cleaned a sentinel: %v", upstream.cleanupRequests)
	}
}

func TestLifecycleCASLoserCleansCandidateRereadsAndConfirmsWinner(t *testing.T) {
	repository := &lifecycleRepository{
		gets: []lifecycleRepositoryGet{
			{state: lifecycleGeneration(7, lifecycleOldSentinel)},
			{state: lifecycleGeneration(8, lifecycleOldSentinel)},
		},
		casState: lifecycleGeneration(8, lifecycleOldSentinel),
	}
	upstream := &lifecycleUpstream{lists: []lifecycleListResult{
		{response: lifecycleSessionList()},
		{response: lifecycleSessionList(lifecycleOldSentinel)},
	}}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if err != nil || !snapshot.Ready || snapshot.Generation != 8 {
		t.Fatalf("Observe() = %#v, %v", snapshot, err)
	}
	if repository.getCalls != 2 || upstream.listCalls != 2 {
		t.Fatalf("loser did not reread winner: gets=%d lists=%d", repository.getCalls, upstream.listCalls)
	}
	if len(upstream.cleanupRequests) != 1 || upstream.cleanupRequests[0] != lifecycleWinnerSentinel {
		t.Fatalf("loser cleanup = %v", upstream.cleanupRequests)
	}
	if upstream.cleanupRequests[0] == lifecycleOldSentinel {
		t.Fatal("loser cleaned the persisted winner")
	}
}

func TestLifecycleCASLoserCleanupFailureIsStableAndNeverCleansWinner(t *testing.T) {
	repository := &lifecycleRepository{
		gets:        []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}},
		casState:    lifecycleGeneration(8, lifecycleOldSentinel),
		casReplaced: false,
	}
	upstream := &lifecycleUpstream{
		lists:      []lifecycleListResult{{response: lifecycleSessionList()}},
		cleanupErr: errors.New("secret upstream cleanup detail"),
	}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if !errors.Is(err, ErrLifecycleCandidateCleanup) {
		t.Fatalf("Observe() error = %v, want ErrLifecycleCandidateCleanup", err)
	}
	if snapshot.Ready || snapshot.State != LifecycleStateRecovering || snapshot.Generation != 7 {
		t.Fatalf("cleanup failure snapshot = %#v", snapshot)
	}
	if repository.getCalls != 1 || upstream.listCalls != 1 || len(upstream.cleanupRequests) != 1 {
		t.Fatalf("cleanup failure continued adoption: gets=%d lists=%d cleanup=%v", repository.getCalls, upstream.listCalls, upstream.cleanupRequests)
	}
	if upstream.cleanupRequests[0] == lifecycleOldSentinel || err.Error() == upstream.cleanupErr.Error() {
		t.Fatalf("cleanup failure exposed detail or touched winner: cleanup=%v error=%v", upstream.cleanupRequests, err)
	}
}

func TestLifecycleCASUnknownNeverCleansPossiblyPublishedCandidate(t *testing.T) {
	repository := &lifecycleRepository{
		gets:   []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}},
		casErr: errors.New("commit outcome unknown"),
	}
	upstream := &lifecycleUpstream{lists: []lifecycleListResult{{response: lifecycleSessionList()}}}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if !errors.Is(err, ErrLifecycleUnknown) {
		t.Fatalf("Observe() error = %v, want ErrLifecycleUnknown", err)
	}
	if snapshot.Ready || snapshot.State != LifecycleStateUnknown || snapshot.Generation != 7 {
		t.Fatalf("unknown CAS snapshot = %#v", snapshot)
	}
	if len(upstream.cleanupRequests) != 0 {
		t.Fatalf("unknown CAS result cleaned a possible winner: %v", upstream.cleanupRequests)
	}
}

func TestLifecycleCandidateMustBeVerifiedBeforeCAS(t *testing.T) {
	repository := &lifecycleRepository{gets: []lifecycleRepositoryGet{{}}}
	upstream := &lifecycleUpstream{
		lists:        []lifecycleListResult{{response: lifecycleSessionList()}},
		viewResponse: lifecycleViewResponse(lifecycleOldSentinel),
	}
	supervisor := newTestLifecycleSupervisor(t, repository, upstream)

	snapshot, err := supervisor.Observe(context.Background())
	if !errors.Is(err, ErrLifecycleCandidateRejected) {
		t.Fatalf("Observe() error = %v, want ErrLifecycleCandidateRejected", err)
	}
	if snapshot.Ready || len(repository.casInputs) != 0 || len(upstream.cleanupRequests) != 1 {
		t.Fatalf("unverified candidate was published: snapshot=%#v cas=%d cleanup=%v", snapshot, len(repository.casInputs), upstream.cleanupRequests)
	}
}

func TestLifecycleListUnknownAndOwnershipMismatchFailClosedWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		repository *lifecycleRepository
		upstream   *lifecycleUpstream
	}{
		{
			name:       "malformed successful list",
			repository: &lifecycleRepository{gets: []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}}},
			upstream:   &lifecycleUpstream{lists: []lifecycleListResult{{response: &sandboxapi.ResponseActiveShellSessionsResult{}}}},
		},
		{
			name:       "list transport error",
			repository: &lifecycleRepository{gets: []lifecycleRepositoryGet{{state: lifecycleGeneration(7, lifecycleOldSentinel)}}},
			upstream:   &lifecycleUpstream{lists: []lifecycleListResult{{err: errors.New("timeout")}}},
		},
		{
			name:       "singleton ownership mismatch",
			repository: &lifecycleRepository{gets: []lifecycleRepositoryGet{{err: domainsandbox.ErrConfigurationInvalid}}},
			upstream:   &lifecycleUpstream{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			supervisor := newTestLifecycleSupervisor(t, test.repository, test.upstream)
			snapshot, err := supervisor.Observe(context.Background())
			if !errors.Is(err, ErrLifecycleUnknown) || snapshot.Ready || snapshot.State != LifecycleStateUnknown {
				t.Fatalf("Observe() = %#v, %v", snapshot, err)
			}
			if len(test.repository.casInputs) != 0 || len(test.upstream.createRequests) != 0 || len(test.upstream.cleanupRequests) != 0 {
				t.Fatalf("unknown state mutated lifecycle: cas=%d create=%d cleanup=%d", len(test.repository.casInputs), len(test.upstream.createRequests), len(test.upstream.cleanupRequests))
			}
		})
	}
}

func TestLifecycleDisabledDoesNotRequireDependenciesOrTouchRawAIO(t *testing.T) {
	supervisor, err := NewLifecycleSupervisor(LifecycleConfig{})
	if err != nil {
		t.Fatalf("NewLifecycleSupervisor(disabled) error = %v", err)
	}
	snapshot, err := supervisor.Observe(context.Background())
	if err != nil || snapshot.Enabled || snapshot.Ready || snapshot.State != LifecycleStateDisabled || snapshot.Generation != 0 {
		t.Fatalf("disabled Observe() = %#v, %v", snapshot, err)
	}
}

func newTestLifecycleSupervisor(t *testing.T, repository *lifecycleRepository, upstream *lifecycleUpstream) *LifecycleSupervisor {
	t.Helper()
	supervisor, err := NewLifecycleSupervisor(LifecycleConfig{
		Enabled: true, DeploymentID: "runner-dev-a", Repository: repository, Upstream: upstream,
		Random: bytes.NewReader([]byte{0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10, 0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10}),
	})
	if err != nil {
		t.Fatalf("NewLifecycleSupervisor() error = %v", err)
	}
	return supervisor
}

func lifecycleGeneration(generation uint64, sentinel string) domainsandbox.AIOGenerationState {
	return domainsandbox.AIOGenerationState{DeploymentID: "runner-dev-a", Generation: generation, SentinelID: sentinel}
}

func lifecycleSessionList(sessionIDs ...string) *sandboxapi.ResponseActiveShellSessionsResult {
	sessions := make(map[string]*sandboxapi.ShellSessionInfo, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessions[sessionID] = &sandboxapi.ShellSessionInfo{WorkingDir: lifecycleExecDir}
	}
	success := true
	return &sandboxapi.ResponseActiveShellSessionsResult{
		Success: &success,
		Data:    &sandboxapi.ActiveShellSessionsResult{Sessions: sessions},
	}
}

func lifecycleCreateResponse(sessionID string) *sandboxapi.ResponseShellCreateSessionResponse {
	success := true
	return &sandboxapi.ResponseShellCreateSessionResponse{
		Success: &success,
		Data:    &sandboxapi.ShellCreateSessionResponse{SessionId: sessionID, WorkingDir: lifecycleExecDir},
	}
}

func lifecycleViewResponse(sessionID string) *sandboxapi.ResponseShellViewResult {
	success := true
	return &sandboxapi.ResponseShellViewResult{
		Success: &success,
		Data:    &sandboxapi.ShellViewResult{SessionId: sessionID},
	}
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	DefaultProviderHealthMaxAge = 5 * time.Minute
)

type ProviderLookup interface {
	GetProviderByKey(ctx context.Context, providerKey string) (*domainsandbox.Provider, error)
}

// ProviderDescriptor is the bounded, non-secret input to adapter eligibility
// checks and the observable portion of a runtime selection.
type ProviderDescriptor struct {
	ProviderKey  string                      `json:"provider_key"`
	ProviderType domainsandbox.ProviderType  `json:"provider_type"`
	Scope        domainsandbox.Scope         `json:"scope"`
	Policy       domainsandbox.RuntimePolicy `json:"policy"`
}

// RuntimeProviderFactory keeps environment and adapter-specific checks outside
// the router. ValidateConfig receives no encrypted fields. Build may decrypt
// credentials and construct transport configuration. ValidateConfig runs
// before capacity acquisition; Build runs only after capacity is acquired and
// a capacityLease cleanup handle exists. Both methods must be bounded and have
// no execution side effects beyond constructing the returned runtime.
type RuntimeProviderFactory interface {
	ValidateConfig(ctx context.Context, descriptor ProviderDescriptor) error
	Build(ctx context.Context, provider domainsandbox.Provider) (infrasandbox.RuntimeProvider, error)
}

type ResolveProviderRequest struct {
	ProviderKey string
	Scope       domainsandbox.Scope
	leaseToken  string
	leaseFence  string
	state       *resolveRequestState
}

type LookupProviderExecutionRequest struct {
	ProviderKey   string
	Scope         domainsandbox.Scope
	OperationID   string
	RequestDigest infrasandbox.ExecutionRequestDigest
	Legacy        bool
	SpaceID       string
	ProjectID     string
	Generation    uint64
	leaseToken    string
	leaseFence    string
	state         *resolveRequestState
	version       executionLookupRequestVersion
	identity      infrasandbox.ExecutionLookupRequest
}

type executionLookupRequestVersion uint8

const (
	executionLookupRequestVersionCurrent executionLookupRequestVersion = iota + 1
	executionLookupRequestVersionLegacy
)

func (LookupProviderExecutionRequest) String() string {
	return "LookupProviderExecutionRequest{identity:<redacted> lease:<redacted>}"
}
func (LookupProviderExecutionRequest) GoString() string {
	return "LookupProviderExecutionRequest{identity:<redacted> lease:<redacted>}"
}
func (LookupProviderExecutionRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "LookupProviderExecutionRequest{identity:<redacted> lease:<redacted>}")
}

type LookupProviderExecutionResult struct {
	Status    infrasandbox.ExecutionLookupStatus
	Execution infrasandbox.ExecuteResult
	Selection *SelectedProvider
}

func (ResolveProviderRequest) String() string   { return "ResolveProviderRequest{lease:<redacted>}" }
func (ResolveProviderRequest) GoString() string { return "ResolveProviderRequest{lease:<redacted>}" }

func (ResolveProviderRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ResolveProviderRequest{lease:<redacted>}")
}

type resolveRequestStatus uint8

const (
	resolveRequestNew resolveRequestStatus = iota
	resolveRequestResolving
	resolveRequestConsumed
)

type resolveRequestState struct {
	mu     sync.Mutex
	status resolveRequestStatus
}

// NewResolveProviderRequest is for trusted in-process callers. The generated
// token is intentionally unexported and omitted from serialization, so an API
// request cannot supply user-controlled lease identity. Reuse the returned
// request value when retrying the same logical resolve operation.
func NewResolveProviderRequest(
	providerKey string,
	scope domainsandbox.Scope,
) (ResolveProviderRequest, error) {
	return newResolveProviderRequest(providerKey, scope, rand.Reader)
}

func NewLookupProviderExecutionRequest(
	providerKey string,
	scope domainsandbox.Scope,
	operationID string,
	requestDigest infrasandbox.ExecutionRequestDigest,
) (LookupProviderExecutionRequest, error) {
	if domainsandbox.ValidateProviderKey(providerKey) != nil || !validRouterScope(scope) ||
		operationID == "" || requestDigest.IsZero() {
		return LookupProviderExecutionRequest{}, domainsandbox.ErrInvalidInput
	}
	token, err := generateOpaqueLeaseToken(rand.Reader)
	if err != nil {
		return LookupProviderExecutionRequest{}, err
	}
	fence, err := generateOpaqueLeaseToken(rand.Reader)
	if err != nil {
		return LookupProviderExecutionRequest{}, err
	}
	identity := infrasandbox.ExecutionLookupRequest{
		Scope: scope, WorkloadKind: infrasandbox.WorkloadAppDev,
		OperationID: operationID, RequestDigest: requestDigest,
	}
	return LookupProviderExecutionRequest{
		ProviderKey: providerKey, Scope: scope, OperationID: operationID,
		RequestDigest: requestDigest, leaseToken: token, leaseFence: fence,
		state: &resolveRequestState{}, version: executionLookupRequestVersionCurrent,
		identity: identity,
	}, nil
}

func NewLegacyLookupProviderExecutionRequest(
	providerKey string,
	scope domainsandbox.Scope,
	spaceID string,
	projectID string,
	generation uint64,
	operationID string,
) (LookupProviderExecutionRequest, error) {
	input := infrasandbox.ExecutionLookupRequest{
		Legacy: true, SpaceID: spaceID, ProjectID: projectID, Generation: generation,
		Scope: scope, WorkloadKind: infrasandbox.WorkloadAppDev, OperationID: operationID,
	}
	if domainsandbox.ValidateProviderKey(providerKey) != nil {
		return LookupProviderExecutionRequest{}, domainsandbox.ErrInvalidInput
	}
	if _, err := infrasandbox.NormalizeExecutionLookupRequest(input); err != nil {
		return LookupProviderExecutionRequest{}, domainsandbox.ErrInvalidInput
	}
	token, err := generateOpaqueLeaseToken(rand.Reader)
	if err != nil {
		return LookupProviderExecutionRequest{}, err
	}
	fence, err := generateOpaqueLeaseToken(rand.Reader)
	if err != nil {
		return LookupProviderExecutionRequest{}, err
	}
	return LookupProviderExecutionRequest{
		ProviderKey: providerKey, Scope: scope, OperationID: operationID,
		Legacy: true, SpaceID: spaceID, ProjectID: projectID, Generation: generation,
		leaseToken: token, leaseFence: fence, state: &resolveRequestState{},
		version: executionLookupRequestVersionLegacy, identity: input,
	}, nil
}

func newResolveProviderRequest(
	providerKey string,
	scope domainsandbox.Scope,
	entropy io.Reader,
) (ResolveProviderRequest, error) {
	if domainsandbox.ValidateProviderKey(providerKey) != nil || !validRouterScope(scope) {
		return ResolveProviderRequest{}, domainsandbox.ErrInvalidInput
	}
	token, err := generateOpaqueLeaseToken(entropy)
	if err != nil {
		return ResolveProviderRequest{}, err
	}
	fence, err := generateOpaqueLeaseToken(entropy)
	if err != nil {
		return ResolveProviderRequest{}, err
	}
	return ResolveProviderRequest{
		ProviderKey: providerKey,
		Scope:       scope,
		leaseToken:  token,
		leaseFence:  fence,
		state:       &resolveRequestState{},
	}, nil
}

type SelectedProvider struct {
	ProviderDescriptor
	runtime           infrasandbox.RuntimeProvider
	lease             *capacityLease
	metrics           SandboxMetricsRecorder
	runtimeAudit      SandboxRuntimeAuditRecorder
	providerID        int64
	mu                sync.Mutex
	state             selectedProviderState
	epoch             uint64
	lifecycleCtx      context.Context
	lifecycleCancel   context.CancelFunc
	inflightCount     int
	drainSignal       chan struct{}
	executionID       string
	executeRequest    infrasandbox.ExecuteRequest
	hasExecuteRequest bool
	uncertainExecute  bool
	cancelInFlight    bool
	cancelAccepted    bool
	cancelAttempt     *providerCancelAttempt
	closeMu           sync.Mutex
	closeAttempt      *runtimeCloseAttempt
	closeSucceeded    bool
}

type providerCancelAttempt struct {
	done chan struct{}
	err  error
}

type runtimeCloseAttempt struct {
	done chan struct{}
	err  error
}

func (*SelectedProvider) String() string   { return "SelectedProvider{lifecycle:<redacted>}" }
func (*SelectedProvider) GoString() string { return "SelectedProvider{lifecycle:<redacted>}" }

func (*SelectedProvider) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "SelectedProvider{lifecycle:<redacted>}")
}

type selectedProviderState uint8

const (
	selectedProviderReady selectedProviderState = iota
	selectedProviderSubmitting
	selectedProviderActive
	selectedProviderCanceling
	selectedProviderUncertain
	selectedProviderCompleted
	selectedProviderContractViolation
	selectedProviderContractCleaning
	selectedProviderContractCleanupPending
	selectedProviderReleasing
	selectedProviderCleanupPending
	selectedProviderReleased
)

const selectedProviderCleanupTimeout = 5 * time.Second

func (s *SelectedProvider) Health(ctx context.Context) (infrasandbox.HealthResult, error) {
	if s == nil || s.runtime == nil || ctx == nil {
		return infrasandbox.HealthResult{}, domainsandbox.ErrInvalidInput
	}
	s.mu.Lock()
	if s.state == selectedProviderReleased || s.state == selectedProviderReleasing ||
		s.state == selectedProviderCleanupPending || s.state == selectedProviderContractViolation ||
		s.state == selectedProviderContractCleaning || s.state == selectedProviderContractCleanupPending {
		s.mu.Unlock()
		return infrasandbox.HealthResult{}, domainsandbox.ErrExecutionForbidden
	}
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return infrasandbox.HealthResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	result, err := s.runtime.Health(callCtx)
	if err != nil {
		return infrasandbox.HealthResult{}, normalizeOperationalError(callCtx, err)
	}
	return result, nil
}

func (s *SelectedProvider) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (result infrasandbox.ExecuteResult, resultErr error) {
	startedAt := time.Now()
	defer func() {
		if s == nil {
			return
		}
		if s.metrics != nil {
			resultCode := sandboxMetricsResultCode(resultErr)
			if resultErr == nil && result.Status != "" {
				resultCode = string(result.Status)
			}
			s.metrics.RecordExecution(ctx, SandboxExecutionMetricsObservation{
				Scope:        s.Scope,
				ProviderType: s.ProviderType,
				Outcome:      sandboxMetricsOutcome(resultErr),
				ResultCode:   resultCode,
				Elapsed:      time.Since(startedAt),
			})
		}
		if s.runtimeAudit != nil && ctx != nil {
			s.runtimeAudit.RecordSandboxRuntimeAudit(ctx, SandboxRuntimeAuditObservation{
				ProviderID: s.providerID, Scope: s.Scope,
				Outcome: sandboxMetricsOutcome(resultErr), CorrelationID: request.IdempotencyKey,
			})
		}
	}()
	if s == nil || s.runtime == nil || ctx == nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return infrasandbox.ExecuteResult{}, err
	}
	s.mu.Lock()
	originState := s.state
	_, async := s.runtime.(infrasandbox.AsyncRuntimeProvider)
	reconciler, canReconcile := s.runtime.(infrasandbox.ExecutionReconciler)
	if async && !canReconcile {
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrConfigurationInvalid
	}
	switch s.state {
	case selectedProviderReady:
		s.executeRequest = cloneRouterExecuteRequest(request)
		s.hasExecuteRequest = true
	case selectedProviderUncertain:
		if !s.uncertainExecute || !s.hasExecuteRequest || !sameRouterExecuteRequest(s.executeRequest, request) {
			s.mu.Unlock()
			return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
		}
	default:
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.transitionLocked(selectedProviderSubmitting)
	operationEpoch := s.epoch
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.settleExecuteFailureLocked(originState, false)
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	var err error
	if originState == selectedProviderUncertain {
		result, err = reconciler.Reconcile(callCtx, request)
	} else {
		result, err = s.runtime.Execute(callCtx, request)
	}
	uncertain := infrasandbox.IsExecutionSubmissionUncertain(err)
	normalizedErr := normalizeOperationalError(callCtx, err)
	s.mu.Lock()
	if !s.operationMatchesLocked(operationEpoch, selectedProviderSubmitting) {
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	if err != nil {
		if originState == selectedProviderUncertain {
			s.settleExecuteFailureLocked(
				originState,
				!infrasandbox.IsAuthoritativeNotRecorded(err),
			)
		} else {
			s.settleExecuteFailureLocked(originState, uncertain)
		}
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, normalizedErr
	}
	normalized, validationErr := infrasandbox.NormalizeExecuteResult(result, s.Policy.MaxOutputBytes)
	if validationErr != nil {
		if !async {
			s.executionID = ""
			s.uncertainExecute = false
			s.transitionLocked(selectedProviderContractViolation)
		} else {
			s.settleExecuteFailureLocked(originState, true)
		}
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	if !async && !terminalExecutionStatus(normalized.Status) {
		s.executionID = normalized.ExecutionID
		s.uncertainExecute = false
		s.transitionLocked(selectedProviderContractViolation)
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	s.executionID = normalized.ExecutionID
	s.uncertainExecute = false
	if terminalExecutionStatus(normalized.Status) {
		s.transitionLocked(selectedProviderCompleted)
	} else {
		s.transitionLocked(selectedProviderActive)
	}
	s.mu.Unlock()
	return normalized, nil
}

func (s *SelectedProvider) Status(ctx context.Context) (infrasandbox.ExecuteResult, error) {
	if s == nil || s.runtime == nil || ctx == nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	async, ok := s.runtime.(infrasandbox.AsyncRuntimeProvider)
	if !ok {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Lock()
	if (s.state != selectedProviderActive && s.state != selectedProviderCanceling && s.state != selectedProviderUncertain) || s.executionID == "" {
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	executionID := s.executionID
	expectedState := s.state
	operationEpoch := s.epoch
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	result, err := async.Status(callCtx, executionID)
	normalizedErr := normalizeOperationalError(callCtx, err)
	normalized, validationErr := infrasandbox.NormalizeExecuteResult(result, s.Policy.MaxOutputBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.operationMatchesLocked(operationEpoch, expectedState) {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	if err != nil {
		s.transitionLocked(selectedProviderUncertain)
		s.uncertainExecute = false
		return infrasandbox.ExecuteResult{}, normalizedErr
	}
	if validationErr != nil || normalized.ExecutionID != executionID {
		s.transitionLocked(selectedProviderUncertain)
		s.uncertainExecute = false
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	if terminalExecutionStatus(normalized.Status) {
		s.transitionLocked(selectedProviderCompleted)
	} else if s.cancelAccepted {
		s.transitionLocked(selectedProviderCanceling)
	} else {
		s.transitionLocked(selectedProviderActive)
	}
	return normalized, nil
}

// BeginBuild invokes the optional AppDev build control capability without
// exposing the selected runtime. It participates in the same lifecycle drain
// as Status/Cancel, so cleanup prevents new control I/O and can drain it.
func (s *SelectedProvider) BeginBuild(
	ctx context.Context,
	executionID string,
	stableOperationID string,
) (infrasandbox.BuildObservation, error) {
	if s == nil || s.runtime == nil || ctx == nil || executionID == "" || stableOperationID == "" {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrInvalidInput
	}
	executor, ok := s.runtime.(infrasandbox.AppDevBuildExecutor)
	if !ok {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Lock()
	if s.state != selectedProviderActive || s.executionID != executionID || s.cancelInFlight || s.cancelAccepted {
		s.mu.Unlock()
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	operationEpoch := s.epoch
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	observation, err := executor.BeginBuild(callCtx, executionID, stableOperationID)
	if err != nil {
		return infrasandbox.BuildObservation{}, normalizeOperationalError(callCtx, err)
	}
	normalized, err := infrasandbox.NormalizeBuildObservation(observation)
	if err != nil {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrUnavailable
	}
	s.mu.Lock()
	valid := s.operationMatchesLocked(operationEpoch, selectedProviderActive) && !s.cancelInFlight && !s.cancelAccepted
	s.mu.Unlock()
	if !valid {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	return normalized, nil
}

// BuildStatus polls an existing idempotent AppDev build operation. It never
// begins a new build and is rejected once selection cleanup has started.
func (s *SelectedProvider) BuildStatus(
	ctx context.Context,
	executionID string,
	stableOperationID string,
) (infrasandbox.BuildObservation, error) {
	if s == nil || s.runtime == nil || ctx == nil || executionID == "" || stableOperationID == "" {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrInvalidInput
	}
	executor, ok := s.runtime.(infrasandbox.AppDevBuildExecutor)
	if !ok {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Lock()
	if s.state != selectedProviderActive || s.executionID != executionID || s.cancelInFlight || s.cancelAccepted {
		s.mu.Unlock()
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	operationEpoch := s.epoch
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	observation, err := executor.BuildStatus(callCtx, executionID, stableOperationID)
	if err != nil {
		return infrasandbox.BuildObservation{}, normalizeOperationalError(callCtx, err)
	}
	normalized, err := infrasandbox.NormalizeBuildObservation(observation)
	if err != nil {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrUnavailable
	}
	s.mu.Lock()
	valid := s.operationMatchesLocked(operationEpoch, selectedProviderActive) && !s.cancelInFlight && !s.cancelAccepted
	s.mu.Unlock()
	if !valid {
		return infrasandbox.BuildObservation{}, domainsandbox.ErrExecutionForbidden
	}
	return normalized, nil
}

// PublishArtifact forwards the one-time upload capability through the selected
// authenticated provider channel while preserving lifecycle cancellation.
func (s *SelectedProvider) PublishArtifact(
	ctx context.Context,
	executionID string,
	request infrasandbox.ArtifactPublishRequest,
) (infrasandbox.ArtifactPublishResult, error) {
	if s == nil || s.runtime == nil || ctx == nil || executionID == "" {
		return infrasandbox.ArtifactPublishResult{}, domainsandbox.ErrInvalidInput
	}
	publisher, ok := s.runtime.(infrasandbox.ArtifactPublisher)
	if !ok {
		return infrasandbox.ArtifactPublishResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Lock()
	if s.state != selectedProviderActive || s.executionID != executionID || s.cancelInFlight || s.cancelAccepted {
		s.mu.Unlock()
		return infrasandbox.ArtifactPublishResult{}, domainsandbox.ErrExecutionForbidden
	}
	operationEpoch := s.epoch
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return infrasandbox.ArtifactPublishResult{}, domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	result, err := publisher.PublishArtifact(callCtx, executionID, request)
	if err != nil {
		return infrasandbox.ArtifactPublishResult{}, normalizeOperationalError(callCtx, err)
	}
	s.mu.Lock()
	valid := s.operationMatchesLocked(operationEpoch, selectedProviderActive) && !s.cancelInFlight && !s.cancelAccepted
	s.mu.Unlock()
	if !valid {
		return infrasandbox.ArtifactPublishResult{}, domainsandbox.ErrExecutionForbidden
	}
	return result, nil
}

func (s *SelectedProvider) KeepAlive(ctx context.Context) error {
	if s == nil || s.runtime == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	async, ok := s.runtime.(infrasandbox.AsyncRuntimeProvider)
	if !ok {
		return domainsandbox.ErrExecutionForbidden
	}
	s.mu.Lock()
	if (s.state != selectedProviderActive && s.state != selectedProviderCanceling && s.state != selectedProviderUncertain) || s.executionID == "" {
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	}
	executionID := s.executionID
	expectedState := s.state
	operationEpoch := s.epoch
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	if err := async.KeepAlive(callCtx, executionID); err != nil {
		normalizedErr := normalizeOperationalError(callCtx, err)
		s.mu.Lock()
		matched := s.operationMatchesLocked(operationEpoch, expectedState)
		if matched {
			s.transitionLocked(selectedProviderUncertain)
			s.uncertainExecute = false
		}
		s.mu.Unlock()
		if !matched {
			return domainsandbox.ErrUnavailable
		}
		return normalizedErr
	}
	return nil
}

func (s *SelectedProvider) Cancel(ctx context.Context, _ string) error {
	if s == nil || s.runtime == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	s.mu.Lock()
	if s.state == selectedProviderCanceling && s.cancelAccepted {
		s.mu.Unlock()
		return nil
	}
	if (s.state != selectedProviderActive && s.state != selectedProviderUncertain) || s.executionID == "" {
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	}
	if s.cancelInFlight {
		attempt := s.cancelAttempt
		s.mu.Unlock()
		return waitForCancelAttempt(ctx, attempt)
	}
	executionID := s.executionID
	expectedState := s.state
	operationEpoch := s.epoch
	attempt := &providerCancelAttempt{done: make(chan struct{})}
	s.cancelInFlight = true
	s.cancelAttempt = attempt
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.cancelInFlight = false
		s.cancelAttempt = nil
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	err := s.runtime.Cancel(callCtx, executionID)
	normalizedErr := normalizeOperationalError(callCtx, err)
	s.mu.Lock()
	if s.cancelAttempt == attempt {
		s.cancelInFlight = false
		s.cancelAttempt = nil
		matched := s.operationMatchesLocked(operationEpoch, expectedState)
		if err == nil {
			s.cancelAccepted = true
			if matched {
				s.transitionLocked(selectedProviderCanceling)
			}
		} else if !matched {
			normalizedErr = domainsandbox.ErrUnavailable
		}
		attempt.err = normalizedErr
		close(attempt.done)
	}
	s.mu.Unlock()
	return normalizedErr
}

func (s *SelectedProvider) Renew(ctx context.Context) error {
	if s == nil || s.lease == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	s.mu.Lock()
	if s.state == selectedProviderReleased || s.state == selectedProviderReleasing ||
		s.state == selectedProviderCleanupPending || s.state == selectedProviderContractViolation ||
		s.state == selectedProviderContractCleaning || s.state == selectedProviderContractCleanupPending {
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	}
	callCtx, finishIO, ok := s.beginIOLocked(ctx)
	if !ok {
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	}
	s.mu.Unlock()
	defer finishIO()
	return s.lease.renew(callCtx)
}

func (s *SelectedProvider) Release(ctx context.Context) error {
	if s == nil || s.lease == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	s.mu.Lock()
	contractCleanup := false
	switch s.state {
	case selectedProviderSubmitting, selectedProviderActive, selectedProviderCanceling, selectedProviderUncertain:
		s.mu.Unlock()
		return domainsandbox.ErrExecutionForbidden
	case selectedProviderReleased:
		s.mu.Unlock()
		return nil
	case selectedProviderContractViolation, selectedProviderContractCleaning, selectedProviderContractCleanupPending:
		contractCleanup = true
		s.transitionLocked(selectedProviderContractCleaning)
	default:
		s.transitionLocked(selectedProviderReleasing)
	}
	s.ensureLifecycleLocked()
	s.lifecycleCancel()
	drain := s.currentDrainSignalLocked()
	s.mu.Unlock()
	if err := waitForCleanupSignal(ctx, drain); err != nil {
		s.markCleanupPending(contractCleanup)
		return err
	}
	if err := s.closeRuntime(ctx); err != nil {
		s.markCleanupPending(contractCleanup)
		return err
	}
	if err := s.lease.release(ctx); err != nil {
		s.markCleanupPending(contractCleanup)
		return domainsandbox.ErrUnavailable
	}
	s.mu.Lock()
	if s.state != selectedProviderReleased {
		s.transitionLocked(selectedProviderReleased)
	}
	s.mu.Unlock()
	return nil
}

func (s *SelectedProvider) markCleanupPending(contractCleanup bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == selectedProviderReleased {
		return
	}
	if contractCleanup {
		s.transitionLocked(selectedProviderContractCleanupPending)
	} else {
		s.transitionLocked(selectedProviderCleanupPending)
	}
}

// ExecutionCheckpoint is an opaque in-process lease checkpoint. All fields are
// intentionally unexported; explicit formatters redact every internal value and
// MarshalJSON rejects serialization rather than emitting a misleading object.
type ExecutionCheckpoint struct {
	providerKey      string
	scope            domainsandbox.Scope
	leaseToken       string
	leaseFence       string
	leaseExpiryMilli int64
	executionID      string
}

func (ExecutionCheckpoint) String() string   { return "ExecutionCheckpoint{secrets:<redacted>}" }
func (ExecutionCheckpoint) GoString() string { return "ExecutionCheckpoint{secrets:<redacted>}" }

func (ExecutionCheckpoint) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ExecutionCheckpoint{secrets:<redacted>}")
}

func (ExecutionCheckpoint) MarshalJSON() ([]byte, error) {
	return nil, errors.New("sandbox execution checkpoint cannot be encoded as JSON")
}

func (s *SelectedProvider) Checkpoint() (ExecutionCheckpoint, error) {
	if s == nil || s.lease == nil {
		return ExecutionCheckpoint{}, domainsandbox.ErrInvalidInput
	}
	s.mu.Lock()
	if (s.state != selectedProviderActive && s.state != selectedProviderCanceling && s.state != selectedProviderUncertain) ||
		!validRouterExecutionID(s.executionID) {
		s.mu.Unlock()
		return ExecutionCheckpoint{}, domainsandbox.ErrExecutionForbidden
	}
	scope := s.Scope
	executionID := s.executionID
	s.mu.Unlock()
	providerKey, token, fence, expiry, err := s.lease.snapshot()
	if err != nil {
		return ExecutionCheckpoint{}, err
	}
	return ExecutionCheckpoint{
		providerKey: providerKey, scope: scope, leaseToken: token,
		leaseFence: fence, leaseExpiryMilli: expiry, executionID: executionID,
	}, nil
}

func (s *SelectedProvider) closeRuntime(ctx context.Context) error {
	if s == nil || s.runtime == nil {
		return domainsandbox.ErrUnavailable
	}
	s.closeMu.Lock()
	if s.closeSucceeded {
		s.closeMu.Unlock()
		return nil
	}
	attempt := s.closeAttempt
	owner := false
	if attempt == nil || closeAttemptFailed(attempt) {
		attempt = &runtimeCloseAttempt{done: make(chan struct{})}
		s.closeAttempt = attempt
		owner = true
	}
	s.closeMu.Unlock()
	if owner {
		err := closeRuntimeProvider(ctx, s.runtime)
		s.closeMu.Lock()
		attempt.err = err
		if err == nil {
			s.closeSucceeded = true
		}
		close(attempt.done)
		s.closeMu.Unlock()
		return err
	}
	if err := waitForCleanupSignal(ctx, attempt.done); err != nil {
		return err
	}
	s.closeMu.Lock()
	err := attempt.err
	s.closeMu.Unlock()
	if err != nil {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func closeAttemptFailed(attempt *runtimeCloseAttempt) bool {
	select {
	case <-attempt.done:
		return attempt.err != nil
	default:
		return false
	}
}

func newSelectedProvider(
	descriptor ProviderDescriptor,
	runtime infrasandbox.RuntimeProvider,
	lease *capacityLease,
	state selectedProviderState,
	executionID string,
) *SelectedProvider {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &SelectedProvider{
		ProviderDescriptor: descriptor,
		runtime:            runtime,
		lease:              lease,
		state:              state,
		executionID:        executionID,
		lifecycleCtx:       lifecycleCtx,
		lifecycleCancel:    lifecycleCancel,
	}
}

func (s *SelectedProvider) ensureLifecycleLocked() {
	if s.lifecycleCtx != nil && s.lifecycleCancel != nil {
		return
	}
	s.lifecycleCtx, s.lifecycleCancel = context.WithCancel(context.Background())
}

func (s *SelectedProvider) beginIOLocked(parent context.Context) (context.Context, func(), bool) {
	s.ensureLifecycleLocked()
	if s.lifecycleCtx.Err() != nil {
		return nil, nil, false
	}
	if s.inflightCount == 0 {
		s.drainSignal = make(chan struct{})
	}
	s.inflightCount++
	callCtx, cancel := context.WithCancel(parent)
	stopLifecycleCancel := context.AfterFunc(s.lifecycleCtx, cancel)
	finish := func() {
		stopLifecycleCancel()
		cancel()
		s.mu.Lock()
		s.inflightCount--
		if s.inflightCount == 0 && s.drainSignal != nil {
			close(s.drainSignal)
			s.drainSignal = nil
		}
		s.mu.Unlock()
	}
	return callCtx, finish, true
}

func (s *SelectedProvider) currentDrainSignalLocked() <-chan struct{} {
	if s.inflightCount > 0 && s.drainSignal != nil {
		return s.drainSignal
	}
	drained := make(chan struct{})
	close(drained)
	return drained
}

func waitForCleanupSignal(ctx context.Context, signal <-chan struct{}) error {
	select {
	case <-signal:
		return nil
	default:
	}
	timer := time.NewTimer(selectedProviderCleanupTimeout)
	defer timer.Stop()
	select {
	case <-signal:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return domainsandbox.ErrUnavailable
	}
}

func waitForCancelAttempt(ctx context.Context, attempt *providerCancelAttempt) error {
	if attempt == nil {
		return domainsandbox.ErrUnavailable
	}
	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type pendingBuildCleanupKey struct {
	providerKey string
	scope       domainsandbox.Scope
}

type pendingBuildCleanupItem struct {
	runtime       infrasandbox.RuntimeProvider
	lease         *capacityLease
	closeComplete bool
}

type pendingBuildCleanupAttempt struct {
	done chan struct{}
	err  error
}

type pendingBuildCleanupGroup struct {
	mu         sync.Mutex
	items      []*pendingBuildCleanupItem
	attempt    *pendingBuildCleanupAttempt
	attached   bool
	generation uint64
}

type pendingBuildCleanupSnapshot struct {
	group      *pendingBuildCleanupGroup
	generation uint64
}

type ProviderRouter struct {
	pendingBuildCleanupMu         sync.Mutex
	pendingBuildCleanups          map[pendingBuildCleanupKey]*pendingBuildCleanupGroup
	pendingBuildCleanupGeneration uint64
	providers                     ProviderLookup
	factory                       RuntimeProviderFactory
	limiter                       CapacityLimiter
	leaseDuration                 time.Duration
	now                           func() time.Time
	metrics                       SandboxMetricsRecorder
	runtimeAudit                  SandboxRuntimeAuditRecorder
}

func NewProviderRouter(
	providers ProviderLookup,
	factory RuntimeProviderFactory,
	limiter CapacityLimiter,
	leaseDuration time.Duration,
) (*ProviderRouter, error) {
	return newProviderRouter(providers, factory, limiter, leaseDuration, time.Now)
}

func (r *ProviderRouter) SetMetricsRecorder(metrics SandboxMetricsRecorder) {
	if r != nil {
		r.metrics = metrics
	}
}

func (r *ProviderRouter) SetRuntimeAuditRecorder(recorder SandboxRuntimeAuditRecorder) {
	if r != nil {
		r.runtimeAudit = recorder
	}
}

func newProviderRouter(
	providers ProviderLookup,
	factory RuntimeProviderFactory,
	limiter CapacityLimiter,
	leaseDuration time.Duration,
	now func() time.Time,
) (*ProviderRouter, error) {
	if providers == nil || factory == nil || limiter == nil || now == nil ||
		!validCapacityLeaseDuration(leaseDuration) {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &ProviderRouter{
		providers:     providers,
		factory:       factory,
		limiter:       limiter,
		leaseDuration: leaseDuration,
		now:           now,
	}, nil
}

func (r *ProviderRouter) Resolve(
	ctx context.Context,
	request ResolveProviderRequest,
) (*SelectedProvider, error) {
	if r == nil || ctx == nil || domainsandbox.ValidateProviderKey(request.ProviderKey) != nil ||
		!validRouterScope(request.Scope) || !validOpaqueLeaseToken(request.leaseToken) ||
		!validOpaqueLeaseToken(request.leaseFence) || request.state == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pendingCleanupKey := pendingBuildCleanupKey{providerKey: request.ProviderKey, scope: request.Scope}
	if err := r.retryPendingBuildCleanup(ctx, pendingCleanupKey); err != nil {
		return nil, err
	}
	if err := request.beginResolve(); err != nil {
		return nil, err
	}
	consumed := false
	defer func() {
		request.finishResolve(consumed)
	}()

	descriptor, provider, err := r.prepareRuntime(ctx, request.ProviderKey, request.Scope)
	if err != nil {
		r.recordProviderSelection(ctx, request.Scope, "", err)
		return nil, err
	}

	expiryUnixMilli, err := r.limiter.Acquire(
		ctx,
		descriptor.ProviderKey,
		request.leaseToken,
		request.leaseFence,
		descriptor.Policy.MaxConcurrency,
		r.leaseDuration,
	)
	if err != nil {
		normalizedErr := normalizeOperationalError(ctx, err)
		if r.metrics != nil {
			r.metrics.RecordCapacityRejection(ctx, CapacityRejectionMetricsObservation{
				Scope: request.Scope, ProviderType: descriptor.ProviderType,
				ResultCode: sandboxMetricsResultCode(normalizedErr),
			})
		}
		r.recordProviderSelection(ctx, request.Scope, descriptor.ProviderType, normalizedErr)
		return nil, normalizedErr
	}
	lease, err := newCapacityLease(
		r.limiter,
		descriptor.ProviderKey,
		request.leaseToken,
		request.leaseFence,
		r.leaseDuration,
		expiryUnixMilli,
	)
	if err != nil {
		if releaseCapacityForCleanup(
			ctx, r.limiter, descriptor.ProviderKey, request.leaseToken, request.leaseFence,
		) != nil {
			r.recordProviderSelection(ctx, request.Scope, descriptor.ProviderType, domainsandbox.ErrUnavailable)
			return nil, domainsandbox.ErrUnavailable
		}
		r.recordProviderSelection(ctx, request.Scope, descriptor.ProviderType, domainsandbox.ErrUnavailable)
		return nil, domainsandbox.ErrUnavailable
	}
	runtime, buildErr := r.buildPreparedRuntime(ctx, provider, descriptor.Scope)
	if buildErr != nil {
		r.registerPendingBuildCleanup(pendingCleanupKey, runtime, lease)
		if r.retryPendingBuildCleanup(ctx, pendingCleanupKey) != nil {
			r.recordProviderSelection(ctx, request.Scope, descriptor.ProviderType, domainsandbox.ErrUnavailable)
			return nil, domainsandbox.ErrUnavailable
		}
		r.recordProviderSelection(ctx, request.Scope, descriptor.ProviderType, buildErr)
		return nil, buildErr
	}

	selected := newSelectedProvider(descriptor, runtime, lease, selectedProviderReady, "")
	selected.metrics = r.metrics
	selected.runtimeAudit = r.runtimeAudit
	selected.providerID = provider.ID
	r.recordProviderSelection(ctx, request.Scope, descriptor.ProviderType, nil)
	consumed = true
	return selected, nil
}

func (r *ProviderRouter) recordProviderSelection(
	ctx context.Context,
	scope domainsandbox.Scope,
	providerType domainsandbox.ProviderType,
	err error,
) {
	if r == nil || r.metrics == nil {
		return
	}
	r.metrics.RecordProviderSelection(ctx, ProviderSelectionMetricsObservation{
		Scope:        scope,
		ProviderType: providerType,
		Outcome:      sandboxMetricsOutcome(err),
		ResultCode:   sandboxMetricsResultCode(err),
	})
}

// LookupProviderExecution is the restart-only reconciliation path. It may
// allocate a fresh control-plane lease for an execution that the provider
// confirms already exists, but it never calls Execute.
func (r *ProviderRouter) LookupProviderExecution(
	ctx context.Context,
	request LookupProviderExecutionRequest,
) (LookupProviderExecutionResult, error) {
	unknown := LookupProviderExecutionResult{Status: infrasandbox.ExecutionLookupUnknown}
	if r == nil || ctx == nil || domainsandbox.ValidateProviderKey(request.ProviderKey) != nil ||
		!validRouterScope(request.Scope) || request.OperationID == "" ||
		!validOpaqueLeaseToken(request.leaseToken) || !validOpaqueLeaseToken(request.leaseFence) ||
		request.state == nil {
		return unknown, domainsandbox.ErrInvalidInput
	}
	lookupInput := infrasandbox.ExecutionLookupRequest{
		Scope: request.Scope, WorkloadKind: infrasandbox.WorkloadAppDev,
		OperationID: request.OperationID, RequestDigest: request.RequestDigest,
		Legacy: request.Legacy, SpaceID: request.SpaceID,
		ProjectID: request.ProjectID, Generation: request.Generation,
	}
	normalizedLookup, normalizeLookupErr := infrasandbox.NormalizeExecutionLookupRequest(lookupInput)
	if normalizeLookupErr != nil || normalizedLookup != request.identity ||
		(request.version == executionLookupRequestVersionCurrent && normalizedLookup.Legacy) ||
		(request.version == executionLookupRequestVersionLegacy && !normalizedLookup.Legacy) ||
		(request.version != executionLookupRequestVersionCurrent &&
			request.version != executionLookupRequestVersionLegacy) {
		return unknown, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return unknown, err
	}
	if err := request.beginResolve(); err != nil {
		return unknown, err
	}
	consumed := false
	defer func() { request.finishResolve(consumed) }()

	pendingCleanupKey := pendingBuildCleanupKey{providerKey: request.ProviderKey, scope: request.Scope}
	if err := r.retryPendingBuildCleanup(ctx, pendingCleanupKey); err != nil {
		return unknown, err
	}
	descriptor, provider, err := r.prepareRuntime(ctx, request.ProviderKey, request.Scope)
	if err != nil {
		return unknown, err
	}
	expiryUnixMilli, err := r.limiter.Acquire(
		ctx, descriptor.ProviderKey, request.leaseToken, request.leaseFence,
		descriptor.Policy.MaxConcurrency, r.leaseDuration,
	)
	if err != nil {
		return unknown, normalizeOperationalError(ctx, err)
	}
	lease, err := newCapacityLease(
		r.limiter, descriptor.ProviderKey, request.leaseToken, request.leaseFence,
		r.leaseDuration, expiryUnixMilli,
	)
	if err != nil {
		_ = releaseCapacityForCleanup(ctx, r.limiter, descriptor.ProviderKey, request.leaseToken, request.leaseFence)
		return unknown, domainsandbox.ErrUnavailable
	}
	runtime, buildErr := r.buildPreparedRuntime(ctx, provider, descriptor.Scope)
	if buildErr != nil {
		r.registerPendingBuildCleanup(pendingCleanupKey, runtime, lease)
		if r.retryPendingBuildCleanup(ctx, pendingCleanupKey) != nil {
			return unknown, domainsandbox.ErrUnavailable
		}
		return unknown, buildErr
	}
	selected := newSelectedProvider(descriptor, runtime, lease, selectedProviderReady, "")
	selected.metrics = r.metrics
	selected.runtimeAudit = r.runtimeAudit
	selected.providerID = provider.ID
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), selectedProviderCleanupTimeout)
		defer cancel()
		return selected.Release(cleanupCtx)
	}
	lookup, ok := runtime.(infrasandbox.ExecutionLookupProvider)
	if !ok {
		_ = cleanup()
		return unknown, domainsandbox.ErrConfigurationInvalid
	}
	result, lookupErr := lookup.LookupExecution(ctx, normalizedLookup)
	if lookupErr != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return unknown, domainsandbox.ErrUnavailable
		}
		return unknown, normalizeOperationalError(ctx, lookupErr)
	}
	result, err = infrasandbox.NormalizeExecutionLookupResult(result, descriptor.Policy.MaxOutputBytes)
	if err != nil {
		_ = cleanup()
		return unknown, domainsandbox.ErrUnavailable
	}
	switch result.Status {
	case infrasandbox.ExecutionLookupFound:
		selected.mu.Lock()
		selected.executionID = result.Execution.ExecutionID
		selected.transitionLocked(selectedProviderActive)
		selected.mu.Unlock()
		consumed = true
		return LookupProviderExecutionResult{
			Status: result.Status, Execution: result.Execution, Selection: selected,
		}, nil
	case infrasandbox.ExecutionLookupNotFound, infrasandbox.ExecutionLookupUnknown:
		if cleanupErr := cleanup(); cleanupErr != nil {
			return unknown, domainsandbox.ErrUnavailable
		}
		consumed = true
		return LookupProviderExecutionResult{Status: result.Status}, nil
	default:
		_ = cleanup()
		return unknown, domainsandbox.ErrUnavailable
	}
}

func (r *ProviderRouter) Resume(
	ctx context.Context,
	checkpoint ExecutionCheckpoint,
	executionID string,
) (*SelectedProvider, error) {
	if r == nil || ctx == nil || domainsandbox.ValidateProviderKey(checkpoint.providerKey) != nil ||
		!validRouterScope(checkpoint.scope) || !validOpaqueLeaseToken(checkpoint.leaseToken) ||
		!validOpaqueLeaseToken(checkpoint.leaseFence) || checkpoint.leaseExpiryMilli <= 0 ||
		!validRouterExecutionID(checkpoint.executionID) || executionID != checkpoint.executionID {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if checkpoint.leaseExpiryMilli <= r.now().UnixMilli() {
		return nil, domainsandbox.ErrExecutionForbidden
	}
	pendingCleanupKey := pendingBuildCleanupKey{providerKey: checkpoint.providerKey, scope: checkpoint.scope}
	if err := r.retryPendingBuildCleanup(ctx, pendingCleanupKey); err != nil {
		return nil, err
	}
	lease, err := newCapacityLease(
		r.limiter, checkpoint.providerKey, checkpoint.leaseToken, checkpoint.leaseFence,
		r.leaseDuration, checkpoint.leaseExpiryMilli,
	)
	if err != nil {
		return nil, err
	}
	if err := lease.renewWithID(ctx, checkpoint.resumeRenewalID()); err != nil {
		return nil, err
	}
	descriptor, provider, err := r.prepareRuntime(ctx, checkpoint.providerKey, checkpoint.scope)
	if err != nil {
		return nil, err
	}
	runtime, buildErr := r.buildPreparedRuntime(ctx, provider, descriptor.Scope)
	if buildErr != nil {
		r.registerPendingBuildCleanup(pendingCleanupKey, runtime, lease)
		if r.retryPendingBuildCleanup(ctx, pendingCleanupKey) != nil {
			return nil, domainsandbox.ErrUnavailable
		}
		return nil, buildErr
	}
	selected := newSelectedProvider(
		descriptor, runtime, lease, selectedProviderActive, checkpoint.executionID,
	)
	selected.metrics = r.metrics
	selected.runtimeAudit = r.runtimeAudit
	selected.providerID = provider.ID
	return selected, nil
}

func (checkpoint ExecutionCheckpoint) resumeRenewalID() string {
	return deriveOpaqueLeaseToken(
		"sandbox-resume-renewal-v1",
		checkpoint.providerKey,
		string(checkpoint.scope),
		checkpoint.leaseToken,
		checkpoint.leaseFence,
		strconv.FormatInt(checkpoint.leaseExpiryMilli, 10),
		checkpoint.executionID,
	)
}

func (s *SelectedProvider) transitionLocked(next selectedProviderState) {
	if s.state == next {
		return
	}
	s.state = next
	s.epoch++
}

func (s *SelectedProvider) operationMatchesLocked(epoch uint64, expected selectedProviderState) bool {
	return s.epoch == epoch && s.state == expected
}

func (s *SelectedProvider) settleExecuteFailureLocked(_ selectedProviderState, uncertain bool) {
	if uncertain {
		s.transitionLocked(selectedProviderUncertain)
		s.uncertainExecute = true
		return
	}
	s.executionID = ""
	s.executeRequest = infrasandbox.ExecuteRequest{}
	s.hasExecuteRequest = false
	s.uncertainExecute = false
	s.transitionLocked(selectedProviderReady)
}

func (r *ProviderRouter) prepareRuntime(
	ctx context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (ProviderDescriptor, domainsandbox.Provider, error) {
	provider, err := r.providers.GetProviderByKey(ctx, providerKey)
	if err != nil {
		return ProviderDescriptor{}, domainsandbox.Provider{}, normalizeOperationalError(ctx, err)
	}
	if provider == nil || provider.ProviderKey != providerKey {
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrUnavailable
	}
	if provider.DeletedAt != nil {
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrProviderNotFound
	}
	if provider.Status != domainsandbox.ProviderStatusEnabled {
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrProviderDisabled
	}
	if provider.Type != domainsandbox.ProviderTypeRemoteHTTP && provider.Type != domainsandbox.ProviderTypeLocalDebug {
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrConfigurationInvalid
	}
	if !scopeIncluded(provider.Scopes, scope) {
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrScopeUnsupported
	}
	policy, err := domainsandbox.NormalizeRuntimePolicy(provider.Policy)
	if err != nil {
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrConfigurationInvalid
	}
	if provider.Health.Status != domainsandbox.HealthStatusHealthy ||
		!scopeIncluded(provider.Health.Capabilities, scope) ||
		!freshHealthSnapshot(provider.Health.CheckedAt, r.now()) {
		if !scopeIncluded(provider.Health.Capabilities, scope) {
			return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrScopeUnsupported
		}
		return ProviderDescriptor{}, domainsandbox.Provider{}, domainsandbox.ErrProviderUnhealthy
	}
	descriptor := ProviderDescriptor{
		ProviderKey: provider.ProviderKey, ProviderType: provider.Type,
		Scope: scope, Policy: cloneRuntimePolicy(policy),
	}
	if err := r.factory.ValidateConfig(ctx, descriptor); err != nil {
		return ProviderDescriptor{}, domainsandbox.Provider{}, normalizeOperationalError(ctx, err)
	}
	return descriptor, cloneProviderForAdapter(*provider, policy), nil
}

func (r *ProviderRouter) buildPreparedRuntime(
	ctx context.Context,
	provider domainsandbox.Provider,
	scope domainsandbox.Scope,
) (infrasandbox.RuntimeProvider, error) {
	runtime, err := r.factory.Build(ctx, provider)
	if err != nil {
		return runtime, normalizeOperationalError(ctx, err)
	}
	if runtime == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	if _, async := runtime.(infrasandbox.AsyncRuntimeProvider); async {
		if _, reconciler := runtime.(infrasandbox.ExecutionReconciler); !reconciler {
			return runtime, domainsandbox.ErrConfigurationInvalid
		}
	}
	if scope == domainsandbox.ScopeAppDev {
		if _, lookup := runtime.(infrasandbox.ExecutionLookupProvider); !lookup {
			return runtime, domainsandbox.ErrConfigurationInvalid
		}
	}
	return runtime, nil
}

func (r *ProviderRouter) registerPendingBuildCleanup(key pendingBuildCleanupKey, runtime infrasandbox.RuntimeProvider, lease *capacityLease) {
	if lease == nil {
		return
	}
	item := &pendingBuildCleanupItem{
		runtime:       runtime,
		lease:         lease,
		closeComplete: runtime == nil,
	}

	r.pendingBuildCleanupMu.Lock()
	if r.pendingBuildCleanups == nil {
		r.pendingBuildCleanups = make(map[pendingBuildCleanupKey]*pendingBuildCleanupGroup)
	}
	group := r.pendingBuildCleanups[key]
	if group != nil {
		group.mu.Lock()
		if group.attached {
			group.items = append(group.items, item)
			group.mu.Unlock()
			r.pendingBuildCleanupMu.Unlock()
			return
		}
		group.mu.Unlock()
		delete(r.pendingBuildCleanups, key)
	}
	r.pendingBuildCleanupGeneration++
	if r.pendingBuildCleanupGeneration == 0 {
		r.pendingBuildCleanupGeneration++
	}
	r.pendingBuildCleanups[key] = &pendingBuildCleanupGroup{
		items:      []*pendingBuildCleanupItem{item},
		attached:   true,
		generation: r.pendingBuildCleanupGeneration,
	}
	r.pendingBuildCleanupMu.Unlock()
}

func (r *ProviderRouter) lookupPendingBuildCleanup(key pendingBuildCleanupKey) (pendingBuildCleanupSnapshot, bool) {
	r.pendingBuildCleanupMu.Lock()
	group := r.pendingBuildCleanups[key]
	if group == nil {
		r.pendingBuildCleanupMu.Unlock()
		return pendingBuildCleanupSnapshot{}, false
	}
	group.mu.Lock()
	if !group.attached || group.generation == 0 {
		if r.pendingBuildCleanups[key] == group {
			delete(r.pendingBuildCleanups, key)
		}
		group.mu.Unlock()
		r.pendingBuildCleanupMu.Unlock()
		return pendingBuildCleanupSnapshot{}, false
	}
	snapshot := pendingBuildCleanupSnapshot{group: group, generation: group.generation}
	group.mu.Unlock()
	r.pendingBuildCleanupMu.Unlock()
	return snapshot, true
}

func (r *ProviderRouter) retryPendingBuildCleanup(ctx context.Context, key pendingBuildCleanupKey) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		snapshot, ok := r.lookupPendingBuildCleanup(key)
		if !ok {
			return nil
		}
		reload, err := r.retryPendingBuildCleanupSnapshot(ctx, key, snapshot)
		if err != nil {
			return err
		}
		if !reload {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// retryPendingBuildCleanupSnapshot either completes the observed generation or
// asks the caller to reload the registry. It never grants ownership to a group
// that is no longer attached at the observed key and generation.
func (r *ProviderRouter) retryPendingBuildCleanupSnapshot(
	ctx context.Context,
	key pendingBuildCleanupKey,
	snapshot pendingBuildCleanupSnapshot,
) (reload bool, err error) {
	if snapshot.group == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r.pendingBuildCleanupMu.Lock()
	current := r.pendingBuildCleanups[key]
	if current == nil {
		r.pendingBuildCleanupMu.Unlock()
		return false, nil
	}
	if current != snapshot.group {
		r.pendingBuildCleanupMu.Unlock()
		return true, nil
	}
	group := snapshot.group
	group.mu.Lock()
	if !group.attached {
		delete(r.pendingBuildCleanups, key)
		group.mu.Unlock()
		r.pendingBuildCleanupMu.Unlock()
		return false, nil
	}
	if group.generation != snapshot.generation {
		group.mu.Unlock()
		r.pendingBuildCleanupMu.Unlock()
		return true, nil
	}
	if len(group.items) == 0 {
		delete(r.pendingBuildCleanups, key)
		group.attached = false
		group.mu.Unlock()
		r.pendingBuildCleanupMu.Unlock()
		return false, nil
	}
	if group.attempt != nil {
		attempt := group.attempt
		group.mu.Unlock()
		r.pendingBuildCleanupMu.Unlock()
		attemptErr := waitPendingBuildCleanupAttempt(ctx, attempt)
		return r.confirmPendingBuildCleanupAttempt(ctx, key, snapshot, attemptErr)
	}
	attempt := &pendingBuildCleanupAttempt{done: make(chan struct{})}
	group.attempt = attempt
	group.mu.Unlock()
	r.pendingBuildCleanupMu.Unlock()

	attemptErr := r.performPendingBuildCleanup(ctx, key, group)
	group.mu.Lock()
	attempt.err = attemptErr
	if group.attempt == attempt {
		group.attempt = nil
	}
	close(attempt.done)
	group.mu.Unlock()
	return r.confirmPendingBuildCleanupAttempt(ctx, key, snapshot, attemptErr)
}

func (r *ProviderRouter) confirmPendingBuildCleanupAttempt(
	ctx context.Context,
	key pendingBuildCleanupKey,
	observed pendingBuildCleanupSnapshot,
	attemptErr error,
) (reload bool, err error) {
	current, ok := r.lookupPendingBuildCleanup(key)
	if !ok {
		return false, nil
	}
	if current.group != observed.group || current.generation != observed.generation {
		return true, nil
	}
	if attemptErr != nil {
		return false, attemptErr
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return false, err
		}
	}
	return true, nil
}

func waitPendingBuildCleanupAttempt(ctx context.Context, attempt *pendingBuildCleanupAttempt) error {
	select {
	case <-attempt.done:
		return attempt.err
	default:
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ProviderRouter) performPendingBuildCleanup(parent context.Context, key pendingBuildCleanupKey, group *pendingBuildCleanupGroup) error {
	if parent == nil {
		parent = context.Background()
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), selectedProviderCleanupTimeout)
	defer cancel()
	if group == nil {
		return nil
	}
	group.mu.Lock()
	snapshot := pendingBuildCleanupSnapshot{group: group, generation: group.generation}
	group.mu.Unlock()

	for {
		complete, stale := r.completePendingBuildCleanupIfEmpty(key, snapshot)
		if complete {
			return nil
		}
		if stale {
			return domainsandbox.ErrUnavailable
		}
		if cleanupCtx.Err() != nil {
			return domainsandbox.ErrUnavailable
		}

		group.mu.Lock()
		if !group.attached || group.generation != snapshot.generation {
			group.mu.Unlock()
			return domainsandbox.ErrUnavailable
		}
		if len(group.items) == 0 {
			group.mu.Unlock()
			continue
		}
		item := group.items[0]
		closeComplete := item.closeComplete
		group.mu.Unlock()

		if !closeComplete {
			if err := closeRuntimeProvider(cleanupCtx, item.runtime); err != nil {
				return domainsandbox.ErrUnavailable
			}
			group.mu.Lock()
			item.closeComplete = true
			group.mu.Unlock()
		}
		if err := item.lease.release(cleanupCtx); err != nil {
			return domainsandbox.ErrUnavailable
		}

		group.mu.Lock()
		if len(group.items) > 0 && group.items[0] == item {
			group.items = group.items[1:]
			group.mu.Unlock()
			continue
		}
		group.mu.Unlock()
		return domainsandbox.ErrUnavailable
	}
}

// completePendingBuildCleanupIfEmpty treats an empty observed group as
// complete even when another goroutine already detached it. Registration and
// deletion share the registry->group lock order, so an attached empty group
// cannot gain a new item between the emptiness check and deletion.
func (r *ProviderRouter) completePendingBuildCleanupIfEmpty(
	key pendingBuildCleanupKey,
	snapshot pendingBuildCleanupSnapshot,
) (complete bool, stale bool) {
	if snapshot.group == nil {
		return true, false
	}
	r.pendingBuildCleanupMu.Lock()
	group := snapshot.group
	group.mu.Lock()
	if len(group.items) == 0 {
		if r.pendingBuildCleanups[key] == group && group.attached && group.generation == snapshot.generation {
			delete(r.pendingBuildCleanups, key)
			group.attached = false
		}
		group.mu.Unlock()
		r.pendingBuildCleanupMu.Unlock()
		return true, false
	}
	stale = r.pendingBuildCleanups[key] != group || !group.attached || group.generation != snapshot.generation
	group.mu.Unlock()
	r.pendingBuildCleanupMu.Unlock()
	return false, stale
}

func closeRuntimeProvider(ctx context.Context, runtime infrasandbox.RuntimeProvider) error {
	if runtime == nil {
		return nil
	}
	if ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	closeCtx, cancel := context.WithTimeout(ctx, selectedProviderCleanupTimeout)
	defer cancel()
	if err := runtime.CloseContext(closeCtx); err != nil {
		if contextErr := closeCtx.Err(); contextErr != nil {
			return contextErr
		}
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func releaseCapacityForCleanup(
	parent context.Context,
	limiter CapacityLimiter,
	providerKey string,
	token string,
	fence string,
) error {
	cleanupContext, cancel := newCapacityCleanupContext(parent)
	defer cancel()
	if err := limiter.Release(cleanupContext, providerKey, token, fence); err != nil {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func (r ResolveProviderRequest) beginResolve() error {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if r.state.status != resolveRequestNew {
		return domainsandbox.ErrExecutionForbidden
	}
	r.state.status = resolveRequestResolving
	return nil
}

func (r ResolveProviderRequest) finishResolve(consumed bool) {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if consumed {
		r.state.status = resolveRequestConsumed
		return
	}
	r.state.status = resolveRequestNew
}

func (r LookupProviderExecutionRequest) beginResolve() error {
	if r.state == nil {
		return domainsandbox.ErrInvalidInput
	}
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if r.state.status != resolveRequestNew {
		return domainsandbox.ErrExecutionForbidden
	}
	r.state.status = resolveRequestResolving
	return nil
}

func (r LookupProviderExecutionRequest) finishResolve(consumed bool) {
	if r.state == nil {
		return
	}
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	if consumed {
		r.state.status = resolveRequestConsumed
		return
	}
	r.state.status = resolveRequestNew
}

func validRouterScope(scope domainsandbox.Scope) bool {
	switch scope {
	case domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAppDev,
		domainsandbox.ScopePlugin:
		return true
	default:
		return false
	}
}

func validRouterExecutionID(value string) bool {
	if value == "" || len(value) > infrasandbox.MaxIdentifierBytes {
		return false
	}
	for index := range value {
		character := value[index]
		if (character < 'A' || character > 'Z') && (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' &&
			character != '-' && character != ':' {
			return false
		}
	}
	return true
}

func knownExecutionStatus(status infrasandbox.ExecutionStatus) bool {
	switch status {
	case infrasandbox.ExecutionStatusAccepted, infrasandbox.ExecutionStatusRunning,
		infrasandbox.ExecutionStatusSucceeded, infrasandbox.ExecutionStatusFailed,
		infrasandbox.ExecutionStatusCanceled, infrasandbox.ExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}

func terminalExecutionStatus(status infrasandbox.ExecutionStatus) bool {
	return status == infrasandbox.ExecutionStatusSucceeded || status == infrasandbox.ExecutionStatusFailed ||
		status == infrasandbox.ExecutionStatusCanceled || status == infrasandbox.ExecutionStatusTimedOut
}

func cloneRouterExecuteRequest(request infrasandbox.ExecuteRequest) infrasandbox.ExecuteRequest {
	cloned := request
	cloned.Deadline = request.Deadline.UTC()
	cloned.Policy = cloneRuntimePolicy(request.Policy)
	cloned.Args = append([]string(nil), request.Args...)
	cloned.Env = make(map[string]string, len(request.Env))
	for key, value := range request.Env {
		cloned.Env[key] = value
	}
	cloned.Stdin = append([]byte(nil), request.Stdin...)
	cloned.Files = append([]infrasandbox.FileReference(nil), request.Files...)
	cloned.ArtifactReferences = append([]infrasandbox.ArtifactReference(nil), request.ArtifactReferences...)
	for index := range cloned.ArtifactReferences {
		cloned.ArtifactReferences[index].ExpiresAt = cloned.ArtifactReferences[index].ExpiresAt.UTC()
	}
	return cloned
}

func sameRouterExecuteRequest(left, right infrasandbox.ExecuteRequest) bool {
	return reflect.DeepEqual(cloneRouterExecuteRequest(left), cloneRouterExecuteRequest(right))
}

func scopeIncluded(scopes []domainsandbox.Scope, requested domainsandbox.Scope) bool {
	for _, scope := range scopes {
		if scope == requested {
			return true
		}
	}
	return false
}

func freshHealthSnapshot(checkedAt, now time.Time) bool {
	if checkedAt.IsZero() || now.IsZero() || checkedAt.After(now) {
		return false
	}
	return now.Sub(checkedAt) <= DefaultProviderHealthMaxAge
}

func cloneRuntimePolicy(policy domainsandbox.RuntimePolicy) domainsandbox.RuntimePolicy {
	cloned := policy
	cloned.NetworkAllowlist = append([]string(nil), policy.NetworkAllowlist...)
	cloned.AllowedEnvNames = append([]string(nil), policy.AllowedEnvNames...)
	cloned.VirtualReadPrefixes = append([]string(nil), policy.VirtualReadPrefixes...)
	cloned.VirtualWritePrefixes = append([]string(nil), policy.VirtualWritePrefixes...)
	cloned.AllowedExecutables = append([]string(nil), policy.AllowedExecutables...)
	cloned.AllowEnv = append([]string(nil), policy.AllowEnv...)
	cloned.AllowRead = append([]string(nil), policy.AllowRead...)
	cloned.AllowWrite = append([]string(nil), policy.AllowWrite...)
	cloned.AllowRun = append([]string(nil), policy.AllowRun...)
	cloned.AllowFFI = append([]string(nil), policy.AllowFFI...)
	return cloned
}

func cloneProviderForAdapter(
	provider domainsandbox.Provider,
	policy domainsandbox.RuntimePolicy,
) domainsandbox.Provider {
	provider.Scopes = append([]domainsandbox.Scope(nil), provider.Scopes...)
	provider.Health.Capabilities = append([]domainsandbox.Scope(nil), provider.Health.Capabilities...)
	provider.Policy = cloneRuntimePolicy(policy)
	return provider
}

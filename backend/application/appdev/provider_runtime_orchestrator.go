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

package appdev

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const maxProviderRuntimeSourceArtifactBytes int64 = 100 * 1024 * 1024

var ErrProviderRuntimeSourceArtifactInvalid = errors.New("appdev provider runtime source artifact is invalid")

// ProviderRuntimeSourceArtifact is an internal capability used to mint an
// exact provider download grant. Its object key has no generic serialization
// or formatting representation.
type ProviderRuntimeSourceArtifact struct {
	objectKey string
	digest    string
	size      int64
}

func NewProviderRuntimeSourceArtifact(objectKey, digest string, size int64) (ProviderRuntimeSourceArtifact, error) {
	objectKey = strings.TrimSpace(objectKey)
	digest = strings.TrimSpace(digest)
	if !validProviderRuntimeObjectKey(objectKey) || size <= 0 || size > maxProviderRuntimeSourceArtifactBytes ||
		!strings.HasPrefix(digest, "sha256:") {
		return ProviderRuntimeSourceArtifact{}, ErrProviderRuntimeSourceArtifactInvalid
	}
	if _, err := domainappdev.ParseArtifactGrantDigest(digest); err != nil {
		return ProviderRuntimeSourceArtifact{}, ErrProviderRuntimeSourceArtifactInvalid
	}
	return ProviderRuntimeSourceArtifact{objectKey: objectKey, digest: digest, size: size}, nil
}

func (artifact ProviderRuntimeSourceArtifact) ObjectKey() string { return artifact.objectKey }
func (artifact ProviderRuntimeSourceArtifact) Digest() string    { return artifact.digest }
func (artifact ProviderRuntimeSourceArtifact) Size() int64       { return artifact.size }

func (ProviderRuntimeSourceArtifact) String() string {
	return "ProviderRuntimeSourceArtifact{object_key:<redacted>}"
}

func (ProviderRuntimeSourceArtifact) GoString() string {
	return "ProviderRuntimeSourceArtifact{object_key:<redacted>}"
}

func (ProviderRuntimeSourceArtifact) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderRuntimeSourceArtifact{object_key:<redacted>}")
}

func (ProviderRuntimeSourceArtifact) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderRuntimeSourceArtifactInvalid
}

func validProviderRuntimeObjectKey(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || strings.HasPrefix(value, "/") ||
		strings.ContainsAny(value, "\\\x00\r\n\t") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

const (
	ProviderRuntimeStartArtifactOperation = "appdev.runtime.start"
	providerRuntimeSourceMediaType        = "application/zip"
	providerRuntimeCleanupTimeout         = 2 * time.Second
	providerRuntimeDispatchLeaseMargin    = 5 * time.Second
)

var (
	ErrProviderRuntimeInvalid     = errors.New("appdev provider runtime input is invalid")
	ErrProviderRuntimeUnavailable = errors.New("appdev provider runtime is unavailable")
	ErrProviderRuntimeConflict    = errors.New("appdev provider runtime conflicts with an active execution")
)

type ProviderRuntimeState string

const (
	ProviderRuntimeStatePending         ProviderRuntimeState = "pending"
	ProviderRuntimeStateSubmitting      ProviderRuntimeState = "submitting"
	ProviderRuntimeStateStarting        ProviderRuntimeState = "starting"
	ProviderRuntimeStateRunning         ProviderRuntimeState = "running"
	ProviderRuntimeStateSucceeded       ProviderRuntimeState = "succeeded"
	ProviderRuntimeStateFailed          ProviderRuntimeState = "failed"
	ProviderRuntimeStateCanceled        ProviderRuntimeState = "canceled"
	ProviderRuntimeStateTimedOut        ProviderRuntimeState = "timed_out"
	ProviderRuntimeStateCleanupPending  ProviderRuntimeState = "cleanup_pending"
	ProviderRuntimeStateCleanupComplete ProviderRuntimeState = "cleanup_complete"
)

type ProviderRuntimeProjection struct {
	Generation        uint64               `json:"generation"`
	State             ProviderRuntimeState `json:"state"`
	CanStart          bool                 `json:"can_start"`
	Recovering        bool                 `json:"recovering"`
	Stopping          bool                 `json:"stopping"`
	PreviewRoute      string               `json:"preview_route,omitempty"`
	recoveryOwnership ProviderRuntimeRecoveryOwnership
}

type ProviderRuntimeRecoveryOwnership uint8

const (
	ProviderRuntimeRecoveryOwnershipUnknown ProviderRuntimeRecoveryOwnership = iota
	ProviderRuntimeRecoveryOwnershipRetained
	ProviderRuntimeRecoveryOwnershipReleased
)

func (projection *ProviderRuntimeProjection) RecoveryOwnership() ProviderRuntimeRecoveryOwnership {
	if projection == nil {
		return ProviderRuntimeRecoveryOwnershipUnknown
	}
	return projection.recoveryOwnership
}

type providerRuntimeRecoveryOwnerRetainedError struct{ error }

func (err providerRuntimeRecoveryOwnerRetainedError) Unwrap() error {
	return err.error
}

func MarkProviderRuntimeRecoveryOwnerRetained(err error) error {
	if err == nil {
		err = ErrProviderRuntimeUnavailable
	}
	return providerRuntimeRecoveryOwnerRetainedError{error: err}
}

func IsProviderRuntimeRecoveryOwnerRetained(err error) bool {
	var retained providerRuntimeRecoveryOwnerRetainedError
	return errors.As(err, &retained)
}

func (projection ProviderRuntimeProjection) String() string {
	return fmt.Sprintf("ProviderRuntimeProjection{generation:%d,state:%s,can_start:%t,recovering:%t,stopping:%t}", projection.Generation, projection.State, projection.CanStart, projection.Recovering, projection.Stopping)
}
func (projection ProviderRuntimeProjection) GoString() string { return projection.String() }
func (projection ProviderRuntimeProjection) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, projection.String())
}

type ProviderRuntimeStartInput struct {
	SpaceID     string
	ProjectID   string
	ActorUserID int64
	OperationID string
	ActorID     string
}

type ProviderRuntimeOrchestratorConfig struct {
	ProviderKey           string
	ProviderScope         domainsandbox.Scope
	Policy                domainsandbox.RuntimePolicy
	Entrypoint            string
	Args                  []string
	Env                   map[string]string
	ExecutionTimeout      time.Duration
	GrantTTL              time.Duration
	ProviderLeaseDuration time.Duration
	ArtifactURLPolicy     ArtifactCapabilityURLPolicy
}

type ProviderRuntimeExecutionLedger interface {
	EnsureStart(context.Context, EnsureProviderExecutionStartRequest) (*ProviderExecutionMetadata, error)
	ClaimRecovery(context.Context, ClaimProviderExecutionRecoveryRequest) (*ClaimedProviderExecution, error)
	StartProviderSubmission(context.Context, StartProviderExecutionSubmissionRequest) (*ProviderExecutionMetadata, error)
	MarkProviderLaunchSubmitted(context.Context, MarkProviderExecutionLaunchSubmittedRequest) (*ProviderExecutionMetadata, error)
	SaveProviderSubmission(context.Context, SaveProviderExecutionSubmissionRequest) (*ProviderExecutionMetadata, error)
	AdvanceTerminal(context.Context, AdvanceProviderExecutionTerminalRequest) (*ProviderExecutionMetadata, error)
}

type providerRuntimeLaunchAborter interface {
	AbortProviderLaunch(context.Context, AbortProviderExecutionLaunchRequest) (*ProviderExecutionMetadata, error)
}

type ProviderRuntimeCheckpointService interface {
	SaveCheckpoint(context.Context, SaveProviderExecutionCheckpointRequest) (*ProviderExecutionMetadata, error)
}

type ProviderRuntimeSelection interface {
	Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error)
	Checkpoint() (applicationsandbox.ExecutionCheckpoint, error)
	Release(context.Context) error
}

type ProviderRuntimeRouter interface {
	Resolve(context.Context, applicationsandbox.ResolveProviderRequest) (ProviderRuntimeSelection, error)
}

type ProviderRuntimeLaunchLookupRequest struct {
	ProviderKey   string
	ProviderScope domainsandbox.Scope
	OperationID   string
	RequestDigest infrasandbox.ExecutionRequestDigest
	Version       ProviderRuntimeLaunchLookupVersion
	SpaceID       string
	ProjectID     string
	Generation    uint64
}

type ProviderRuntimeLaunchLookupVersion uint8

const (
	ProviderRuntimeLaunchLookupVersionCurrent ProviderRuntimeLaunchLookupVersion = iota + 1
	ProviderRuntimeLaunchLookupVersionLegacy
)

type ProviderRuntimeLaunchLookupResult struct {
	Status    infrasandbox.ExecutionLookupStatus
	Execution infrasandbox.ExecuteResult
	Selection ProviderRuntimeSelection
}

type ProviderRuntimeLaunchLookupRouter interface {
	LookupProviderExecution(context.Context, ProviderRuntimeLaunchLookupRequest) (ProviderRuntimeLaunchLookupResult, error)
}

type providerRuntimeReconciledLaunchLedger interface {
	CompleteReconciledProviderLaunch(context.Context, CompleteReconciledProviderLaunchRequest) (*ProviderExecutionMetadata, error)
}

type providerRuntimeLaunchReconciliationLedger interface {
	ClaimLaunchReconciliation(
		context.Context,
		ClaimProviderExecutionLaunchReconciliationRequest,
	) (*ClaimedProviderExecution, error)
}

type ProviderRuntimeSourceArtifactProvider interface {
	LoadProviderRuntimeSourceArtifact(context.Context, string, string) (ProviderRuntimeSourceArtifact, error)
}

type ProviderRuntimeGrantIssuer interface {
	Issue(context.Context, IssueArtifactGrantRequest) (*ArtifactGrantCapability, error)
	Revoke(context.Context, RevokeArtifactGrantRequest) error
}

type ProviderRuntimeDownloadEndpointBuilder interface {
	ArtifactDownloadEndpoint(context.Context, domainappdev.ArtifactGrantID) (string, error)
}

type ProviderRuntimeOwnerCapabilityGenerator interface {
	GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error)
}

type cryptoProviderRuntimeOwnerCapabilityGenerator struct{}

func (cryptoProviderRuntimeOwnerCapabilityGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	return domainappdev.NewProviderExecutionOwnerToken(cryptorand.Reader)
}

type SandboxProviderRuntimeRouter struct {
	Router *applicationsandbox.ProviderRouter
}

func (adapter SandboxProviderRuntimeRouter) Resolve(ctx context.Context, request applicationsandbox.ResolveProviderRequest) (ProviderRuntimeSelection, error) {
	if adapter.Router == nil {
		return nil, ErrProviderRuntimeUnavailable
	}
	return adapter.Router.Resolve(ctx, request)
}

func (adapter SandboxProviderRuntimeRouter) LookupProviderExecution(
	ctx context.Context,
	request ProviderRuntimeLaunchLookupRequest,
) (ProviderRuntimeLaunchLookupResult, error) {
	if adapter.Router == nil {
		return ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, ErrProviderRuntimeUnavailable
	}
	var lookupRequest applicationsandbox.LookupProviderExecutionRequest
	var err error
	switch request.Version {
	case ProviderRuntimeLaunchLookupVersionLegacy:
		if !request.RequestDigest.IsZero() {
			return ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, ErrProviderRuntimeInvalid
		}
		lookupRequest, err = applicationsandbox.NewLegacyLookupProviderExecutionRequest(
			request.ProviderKey, request.ProviderScope, request.SpaceID, request.ProjectID,
			request.Generation, request.OperationID,
		)
	case ProviderRuntimeLaunchLookupVersionCurrent:
		if request.RequestDigest.IsZero() || request.SpaceID != "" ||
			request.ProjectID != "" || request.Generation != 0 {
			return ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, ErrProviderRuntimeInvalid
		}
		lookupRequest, err = applicationsandbox.NewLookupProviderExecutionRequest(
			request.ProviderKey, request.ProviderScope, request.OperationID, request.RequestDigest,
		)
	default:
		return ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, ErrProviderRuntimeInvalid
	}
	if err != nil {
		return ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, ErrProviderRuntimeInvalid
	}
	result, err := adapter.Router.LookupProviderExecution(ctx, lookupRequest)
	if err != nil {
		return ProviderRuntimeLaunchLookupResult{Status: infrasandbox.ExecutionLookupUnknown}, err
	}
	return ProviderRuntimeLaunchLookupResult{
		Status: result.Status, Execution: result.Execution, Selection: result.Selection,
	}, nil
}

type ProviderRuntimeOrchestrator struct {
	ledger      ProviderRuntimeExecutionLedger
	checkpoints ProviderRuntimeCheckpointService
	router      ProviderRuntimeRouter
	sources     ProviderRuntimeSourceArtifactProvider
	grants      ProviderRuntimeGrantIssuer
	endpoints   ProviderRuntimeDownloadEndpointBuilder
	ownerToken  domainappdev.ProviderExecutionOwnerToken
	config      ProviderRuntimeOrchestratorConfig
}

func NewProviderRuntimeOrchestrator(ledger ProviderRuntimeExecutionLedger, checkpoints ProviderRuntimeCheckpointService, router ProviderRuntimeRouter, sources ProviderRuntimeSourceArtifactProvider, grants ProviderRuntimeGrantIssuer, endpoints ProviderRuntimeDownloadEndpointBuilder, ownerGenerator ProviderRuntimeOwnerCapabilityGenerator, config ProviderRuntimeOrchestratorConfig) (*ProviderRuntimeOrchestrator, error) {
	if ledger == nil || checkpoints == nil || router == nil || sources == nil || grants == nil || endpoints == nil {
		return nil, ErrProviderRuntimeInvalid
	}
	if ownerGenerator == nil {
		ownerGenerator = cryptoProviderRuntimeOwnerCapabilityGenerator{}
	}
	config.ProviderKey = strings.TrimSpace(config.ProviderKey)
	config.Entrypoint = strings.TrimSpace(config.Entrypoint)
	if domainsandbox.ValidateProviderKey(config.ProviderKey) != nil || config.ProviderScope != domainsandbox.ScopeAppDev || config.Entrypoint == "" ||
		!utf8.ValidString(config.Entrypoint) || strings.ContainsAny(config.Entrypoint, "\x00\r\n\t") || config.ExecutionTimeout <= 0 || config.ExecutionTimeout > time.Hour ||
		config.GrantTTL <= 0 || config.GrantTTL > 120*time.Second || config.ProviderLeaseDuration <= 0 || config.ProviderLeaseDuration > time.Hour {
		return nil, ErrProviderRuntimeInvalid
	}
	ownerToken, err := ownerGenerator.GenerateProviderRuntimeOwnerCapability()
	if err != nil || ownerToken.IsZero() {
		return nil, ErrProviderRuntimeUnavailable
	}
	config.Args = append([]string(nil), config.Args...)
	config.Env = cloneProviderRuntimeEnv(config.Env)
	if config.ArtifactURLPolicy == nil {
		config.ArtifactURLPolicy = NewArtifactCapabilityURLPolicy(false)
	}
	return &ProviderRuntimeOrchestrator{ledger: ledger, checkpoints: checkpoints, router: router, sources: sources, grants: grants, endpoints: endpoints, ownerToken: ownerToken, config: config}, nil
}

func (*ProviderRuntimeOrchestrator) String() string {
	return "ProviderRuntimeOrchestrator{owner:<redacted>}"
}
func (*ProviderRuntimeOrchestrator) GoString() string {
	return "ProviderRuntimeOrchestrator{owner:<redacted>}"
}
func (*ProviderRuntimeOrchestrator) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderRuntimeOrchestrator{owner:<redacted>}")
}
func (*ProviderRuntimeOrchestrator) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderRuntimeInvalid
}

func (orchestrator *ProviderRuntimeOrchestrator) Start(ctx context.Context, input ProviderRuntimeStartInput) (*ProviderRuntimeProjection, error) {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil || !validProviderRuntimeStartInput(input) {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderRuntimeInvalid
	}
	metadata, err := orchestrator.ledger.EnsureStart(ctx, EnsureProviderExecutionStartRequest{SpaceID: input.SpaceID, ProjectID: input.ProjectID, ActorUserID: input.ActorUserID, IdempotencyKey: input.OperationID, ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope, RequireNoActive: true})
	if err != nil {
		return nil, normalizeProviderRuntimeError(ctx, err)
	}
	if metadata == nil || metadata.Generation == 0 || metadata.SpaceID != input.SpaceID || metadata.ProjectID != input.ProjectID || metadata.ProviderKey != orchestrator.config.ProviderKey || metadata.ProviderScope != orchestrator.config.ProviderScope {
		return nil, ErrProviderRuntimeUnavailable
	}
	if metadata.ObservedState != domainappdev.ProviderExecutionObservedPending && metadata.ObservedState != domainappdev.ProviderExecutionObservedSubmitting {
		projection := providerRuntimeProjection(*metadata, "")
		return &projection, nil
	}
	if metadata.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
		metadata.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted {
		projection := providerRuntimeProjection(*metadata, ProviderRuntimeStateStarting)
		projection.Recovering = true
		projection.CanStart = false
		return &projection, ErrProviderRuntimeUnavailable
	}
	claim := ClaimProviderExecutionRecoveryRequest{SpaceID: metadata.SpaceID, ProjectID: metadata.ProjectID, Generation: metadata.Generation, ExpectedVersion: metadata.Version}
	if metadata.OwnerEpoch == 0 {
		claim.ProposedOwner = orchestrator.ownerToken
	} else {
		claim.ExistingOwner = orchestrator.ownerToken
		claim.ExistingOwnerEpoch = metadata.OwnerEpoch
	}
	claimed, err := orchestrator.ledger.ClaimRecovery(ctx, claim)
	if err != nil || claimed == nil {
		projection := providerRuntimeProjection(*metadata, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	owner := claimed.Owner()
	current := claimed.Metadata
	source, err := orchestrator.sources.LoadProviderRuntimeSourceArtifact(ctx, input.SpaceID, input.ProjectID)
	if err != nil {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	resolveRequest, err := applicationsandbox.NewResolveProviderRequest(orchestrator.config.ProviderKey, orchestrator.config.ProviderScope)
	if err != nil {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	selection, err := orchestrator.router.Resolve(ctx, resolveRequest)
	if err != nil || selection == nil {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	grant, reference, err := orchestrator.issueSourceGrant(ctx, input, source)
	if err != nil {
		orchestrator.cleanupPreExecute(ctx, nil, selection)
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	dispatchLeaseDuration, ok := providerRuntimeDispatchLeaseDuration(orchestrator.config.ExecutionTimeout)
	if !ok {
		orchestrator.cleanupPreExecute(ctx, grant, selection)
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	executeCtx, cancelExecute := context.WithTimeout(ctx, orchestrator.config.ExecutionTimeout)
	defer cancelExecute()
	executeDeadline, ok := executeCtx.Deadline()
	if !ok {
		orchestrator.cleanupPreExecute(ctx, grant, selection)
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	executeRequest := infrasandbox.ExecuteRequest{Scope: orchestrator.config.ProviderScope, WorkloadKind: infrasandbox.WorkloadAppDev, IdempotencyKey: providerRuntimeStableID("appdev_start", input.SpaceID, input.ProjectID, input.OperationID), Deadline: executeDeadline.UTC(), Policy: orchestrator.config.Policy, Entrypoint: orchestrator.config.Entrypoint, Args: append([]string(nil), orchestrator.config.Args...), Env: cloneProviderRuntimeEnv(orchestrator.config.Env), ArtifactReferences: []infrasandbox.ArtifactReference{reference}}
	if current.ActorUserID > 0 {
		spaceID, parseErr := strconv.ParseInt(current.SpaceID, 10, 64)
		if parseErr != nil || spaceID <= 0 || current.ID == "" {
			orchestrator.cleanupPreExecute(ctx, grant, selection)
			projection := providerRuntimeProjection(current, "")
			projection.Recovering = true
			return &projection, ErrProviderRuntimeUnavailable
		}
		executeRequest.Identity = infrasandbox.ExecutionIdentity{SpaceID: spaceID, UserID: current.ActorUserID, ProjectID: current.ProjectID, ExecutionID: current.ID}
	}
	requestDigest, digestErr := infrasandbox.DigestExecuteRequest(executeRequest)
	if digestErr != nil {
		orchestrator.cleanupPreExecute(ctx, grant, selection)
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	if current.ObservedState == domainappdev.ProviderExecutionObservedPending {
		reserved, reserveErr := orchestrator.ledger.StartProviderSubmission(ctx, StartProviderExecutionSubmissionRequest{
			Owner: owner, ExpectedVersion: current.Version, OperationID: input.OperationID,
			ProviderOperationID: executeRequest.IdempotencyKey,
			RequestDigest:       domainappdev.ProviderExecutionLaunchRequestDigest(requestDigest),
		})
		if reserveErr != nil || reserved == nil {
			orchestrator.cleanupPreExecute(ctx, grant, selection)
			projection := providerRuntimeProjection(current, "")
			projection.Recovering = true
			return &projection, normalizeProviderRuntimeError(ctx, reserveErr)
		}
		current = *reserved
		submitted, submitErr := orchestrator.ledger.MarkProviderLaunchSubmitted(ctx, MarkProviderExecutionLaunchSubmittedRequest{
			Owner: owner, ExpectedVersion: current.Version, OperationID: input.OperationID,
			DispatchLeaseDuration: dispatchLeaseDuration,
		})
		if submitErr != nil || submitted == nil {
			orchestrator.cleanupPreExecute(ctx, grant, selection)
			projection := providerRuntimeProjection(current, ProviderRuntimeStateStarting)
			projection.Recovering = true
			return &projection, normalizeProviderRuntimeError(ctx, submitErr)
		}
		current = *submitted
	}
	result, err := selection.Execute(executeCtx, executeRequest)
	if err != nil {
		if !infrasandbox.IsExecutionSubmissionUncertain(err) {
			if aborter, ok := orchestrator.ledger.(providerRuntimeLaunchAborter); ok {
				aborted, abortErr := aborter.AbortProviderLaunch(ctx, AbortProviderExecutionLaunchRequest{
					Owner: owner, ExpectedVersion: current.Version, OperationID: input.OperationID,
					ExpectedState: domainappdev.ProviderExecutionLaunchSubmitted,
				})
				if abortErr == nil && aborted != nil {
					current = *aborted
					orchestrator.cleanupPreExecute(ctx, grant, selection)
				} else {
					orchestrator.cleanupPostExecute(ctx, grant)
				}
			} else {
				orchestrator.cleanupPostExecute(ctx, grant)
			}
		} else {
			orchestrator.cleanupPostExecute(ctx, grant)
		}
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	preview, err := infrasandbox.NormalizePreviewRoute(result.PreviewRoute)
	if err != nil || !domainappdev.ValidProviderExecutionProviderID(result.ExecutionID) {
		orchestrator.cleanupPostExecute(ctx, grant)
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	saved, err := orchestrator.ledger.SaveProviderSubmission(ctx, SaveProviderExecutionSubmissionRequest{Owner: owner, ExpectedVersion: current.Version, LaunchOperationID: input.OperationID, ProviderExecutionID: result.ExecutionID, ObservedState: domainappdev.ProviderExecutionObservedRunning, ProviderLeaseDuration: orchestrator.config.ProviderLeaseDuration, PreviewRoute: preview})
	if err != nil || saved == nil {
		orchestrator.cleanupPostExecute(ctx, grant)
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	checkpoint, err := selection.Checkpoint()
	if err != nil {
		orchestrator.cleanupPostExecute(ctx, grant)
		projection := providerRuntimeProjection(*saved, providerRuntimeResultState(result.Status))
		projection.PreviewRoute, projection.Recovering = preview, true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	checkpointMetadata, err := orchestrator.checkpoints.SaveCheckpoint(ctx, SaveProviderExecutionCheckpointRequest{
		Owner: owner, ExpectedVersion: saved.Version,
		OperationID:       providerRuntimeStableID("appdev_checkpoint", input.SpaceID, input.ProjectID, input.OperationID),
		LaunchOperationID: input.OperationID,
		ProviderKey:       orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope, Checkpoint: checkpoint,
	})
	if err != nil || checkpointMetadata == nil {
		orchestrator.cleanupPostExecute(ctx, grant)
		projection := providerRuntimeProjection(*saved, providerRuntimeResultState(result.Status))
		projection.PreviewRoute, projection.Recovering = preview, true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	finalMetadata := checkpointMetadata
	if observed, terminal := providerRuntimeTerminalState(result.Status); terminal {
		finalMetadata, err = orchestrator.ledger.AdvanceTerminal(ctx, AdvanceProviderExecutionTerminalRequest{Owner: owner, ExpectedVersion: checkpointMetadata.Version, OperationID: providerRuntimeStableID("appdev_terminal", input.SpaceID, input.ProjectID, input.OperationID), ObservedState: observed, PreviewRoute: preview, SafeErrorCode: providerRuntimeTerminalErrorCode(result.Status), SafeErrorMessage: providerRuntimeTerminalErrorMessage(result.Status)})
		if err != nil || finalMetadata == nil {
			projection := providerRuntimeProjection(*checkpointMetadata, providerRuntimeResultState(result.Status))
			projection.PreviewRoute, projection.Recovering = preview, true
			return &projection, normalizeProviderRuntimeError(ctx, err)
		}
	}
	projection := providerRuntimeProjection(*finalMetadata, providerRuntimeResultState(result.Status))
	projection.PreviewRoute = preview
	return &projection, nil
}

func (orchestrator *ProviderRuntimeOrchestrator) issueSourceGrant(ctx context.Context, input ProviderRuntimeStartInput, source ProviderRuntimeSourceArtifact) (*ArtifactGrantCapability, infrasandbox.ArtifactReference, error) {
	digest, err := domainappdev.ParseArtifactGrantDigest(source.Digest())
	if err != nil {
		return nil, infrasandbox.ArtifactReference{}, ErrProviderRuntimeInvalid
	}
	capability, err := orchestrator.grants.Issue(ctx, IssueArtifactGrantRequest{Spec: domainappdev.ArtifactGrantSpec{Audience: domainappdev.ArtifactGrantAudience{SpaceID: input.SpaceID, ProjectID: input.ProjectID, ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope, Operation: ProviderRuntimeStartArtifactOperation}, Direction: domainappdev.ArtifactGrantDirectionDownload, ObjectKey: source.ObjectKey(), Digest: digest, Size: source.Size(), MaxSize: source.Size()}, TTL: orchestrator.config.GrantTTL})
	if err != nil || capability == nil {
		return nil, infrasandbox.ArtifactReference{}, ErrProviderRuntimeUnavailable
	}
	endpoint, err := orchestrator.endpoints.ArtifactDownloadEndpoint(ctx, capability.GrantID())
	if err != nil || orchestrator.config.ArtifactURLPolicy.Validate(endpoint) != nil {
		orchestrator.revokeGrant(ctx, capability)
		return nil, infrasandbox.ArtifactReference{}, ErrProviderRuntimeUnavailable
	}
	return capability, infrasandbox.ArtifactReference{Direction: infrasandbox.ArtifactDirectionDownload, URL: endpoint, Token: capability.BearerToken(), Digest: source.Digest(), Size: source.Size(), MediaType: providerRuntimeSourceMediaType, ExpiresAt: capability.ExpiresAt()}, nil
}

func (orchestrator *ProviderRuntimeOrchestrator) cleanupPreExecute(ctx context.Context, capability *ArtifactGrantCapability, selection ProviderRuntimeSelection) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerRuntimeCleanupTimeout)
	defer cancel()
	orchestrator.revokeGrant(cleanupCtx, capability)
	if selection != nil {
		_ = selection.Release(cleanupCtx)
	}
}

func (orchestrator *ProviderRuntimeOrchestrator) cleanupPostExecute(ctx context.Context, capability *ArtifactGrantCapability) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), providerRuntimeCleanupTimeout)
	defer cancel()
	orchestrator.revokeGrant(cleanupCtx, capability)
}

func (orchestrator *ProviderRuntimeOrchestrator) revokeGrant(ctx context.Context, capability *ArtifactGrantCapability) {
	if capability != nil {
		_ = orchestrator.grants.Revoke(ctx, RevokeArtifactGrantRequest{GrantID: capability.grantID, Token: capability.token, Audience: capability.audience, Direction: capability.direction})
	}
}

func providerRuntimeProjection(metadata ProviderExecutionMetadata, override ProviderRuntimeState) ProviderRuntimeProjection {
	state := override
	if state == "" {
		state = providerRuntimeObservedState(metadata.ObservedState)
	}
	return ProviderRuntimeProjection{Generation: metadata.Generation, State: state, CanStart: metadata.ObservedState == domainappdev.ProviderExecutionObservedCleanupComplete, Stopping: metadata.DesiredState == domainappdev.ProviderExecutionDesiredStop && metadata.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete, PreviewRoute: metadata.PreviewRoute}
}

func providerRuntimeObservedState(state domainappdev.ProviderExecutionObservedState) ProviderRuntimeState {
	switch state {
	case domainappdev.ProviderExecutionObservedSubmitting:
		return ProviderRuntimeStateSubmitting
	case domainappdev.ProviderExecutionObservedRunning:
		return ProviderRuntimeStateRunning
	case domainappdev.ProviderExecutionObservedSucceeded:
		return ProviderRuntimeStateSucceeded
	case domainappdev.ProviderExecutionObservedFailed:
		return ProviderRuntimeStateFailed
	case domainappdev.ProviderExecutionObservedCanceled:
		return ProviderRuntimeStateCanceled
	case domainappdev.ProviderExecutionObservedTimedOut:
		return ProviderRuntimeStateTimedOut
	case domainappdev.ProviderExecutionObservedCleanupPending:
		return ProviderRuntimeStateCleanupPending
	case domainappdev.ProviderExecutionObservedCleanupComplete:
		return ProviderRuntimeStateCleanupComplete
	default:
		return ProviderRuntimeStatePending
	}
}

func providerRuntimeResultState(status infrasandbox.ExecutionStatus) ProviderRuntimeState {
	switch status {
	case infrasandbox.ExecutionStatusAccepted:
		return ProviderRuntimeStateStarting
	case infrasandbox.ExecutionStatusRunning:
		return ProviderRuntimeStateRunning
	case infrasandbox.ExecutionStatusSucceeded:
		return ProviderRuntimeStateSucceeded
	case infrasandbox.ExecutionStatusFailed:
		return ProviderRuntimeStateFailed
	case infrasandbox.ExecutionStatusCanceled:
		return ProviderRuntimeStateCanceled
	case infrasandbox.ExecutionStatusTimedOut:
		return ProviderRuntimeStateTimedOut
	default:
		return ProviderRuntimeStateSubmitting
	}
}

func providerRuntimeTerminalState(status infrasandbox.ExecutionStatus) (domainappdev.ProviderExecutionObservedState, bool) {
	switch status {
	case infrasandbox.ExecutionStatusSucceeded:
		return domainappdev.ProviderExecutionObservedSucceeded, true
	case infrasandbox.ExecutionStatusFailed:
		return domainappdev.ProviderExecutionObservedFailed, true
	case infrasandbox.ExecutionStatusCanceled:
		return domainappdev.ProviderExecutionObservedCanceled, true
	case infrasandbox.ExecutionStatusTimedOut:
		return domainappdev.ProviderExecutionObservedTimedOut, true
	default:
		return "", false
	}
}

func providerRuntimeTerminalErrorCode(status infrasandbox.ExecutionStatus) string {
	if status == infrasandbox.ExecutionStatusSucceeded {
		return ""
	}
	return "provider_" + string(status)
}
func providerRuntimeTerminalErrorMessage(status infrasandbox.ExecutionStatus) string {
	if status == infrasandbox.ExecutionStatusSucceeded {
		return ""
	}
	return "provider execution " + string(status)
}

const ProviderRuntimeStateStopped ProviderRuntimeState = "stopped"

type ProviderRuntimeStopInput struct {
	SpaceID            string
	ProjectID          string
	OperationID        string
	ActorID            string
	ExpectedGeneration *uint64
}

type ProviderRuntimeCleanupLedger interface {
	ProviderRuntimeStatusLedger
	SetDesiredStop(context.Context, SetProviderExecutionDesiredStopRequest) (*ProviderExecutionMetadata, error)
	BeginCleanup(context.Context, BeginProviderExecutionCleanupRequest) (*ProviderExecutionMetadata, error)
	CompleteCleanup(context.Context, CompleteProviderExecutionCleanupRequest) (*ProviderExecutionMetadata, error)
}

type ProviderRuntimeCleanupSelection interface {
	ProviderRuntimeStatusSelection
	Cancel(context.Context, string) error
	Release(context.Context) error
}

func (orchestrator *ProviderRuntimeOrchestrator) Stop(ctx context.Context, input ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error) {
	return orchestrator.ContinueCleanup(ctx, input)
}

func (orchestrator *ProviderRuntimeOrchestrator) ContinueCleanup(ctx context.Context, input ProviderRuntimeStopInput) (*ProviderRuntimeProjection, error) {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil || !validProviderRuntimeStopInput(input) {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderRuntimeInvalid
	}
	ledger, ok := orchestrator.ledger.(ProviderRuntimeCleanupLedger)
	if !ok {
		return nil, ErrProviderRuntimeUnavailable
	}
	currentRecord, err := ledger.LoadCurrent(ctx, LoadCurrentProviderExecutionRequest{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID,
	})
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		if input.ExpectedGeneration != nil && *input.ExpectedGeneration != 0 {
			return nil, ErrProviderRuntimeConflict
		}
		projection := ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}
		return &projection, nil
	}
	if err != nil || currentRecord == nil {
		return nil, normalizeProviderRuntimeError(ctx, err)
	}
	current := *currentRecord
	if current.SpaceID != input.SpaceID || current.ProjectID != input.ProjectID {
		return nil, ErrProviderRuntimeUnavailable
	}
	if current.ProviderKey != orchestrator.config.ProviderKey || current.ProviderScope != orchestrator.config.ProviderScope || current.Generation == 0 {
		return nil, ErrProviderRuntimeUnavailable
	}
	if input.ExpectedGeneration != nil && current.Generation != *input.ExpectedGeneration {
		return nil, ErrProviderRuntimeConflict
	}
	if current.ObservedState == domainappdev.ProviderExecutionObservedCleanupComplete {
		projection := providerRuntimeProjection(current, ProviderRuntimeStateStopped)
		projection.CanStart = true
		return &projection, nil
	}

	var (
		ownedMetadata ProviderExecutionMetadata
		owner         ProviderExecutionOwner
		desiredStop   *ProviderExecutionMetadata
	)
	if providerRuntimeLaunchNeedsReconciliation(current.LaunchState) {
		claimed, claimErr := orchestrator.claimLaunchReconciliationOwner(ctx, current, true)
		if claimErr != nil || claimed == nil {
			projection := providerRuntimeStoppingProjection(current, true)
			return &projection, normalizeProviderRuntimeNonNilError(ctx, claimErr)
		}
		ownedMetadata, owner = claimed.Metadata, claimed.Owner()
		desiredStop = &ownedMetadata
	} else {
		ownedMetadata, owner, err = orchestrator.renewOrClaimStatusOwner(ctx, ledger, current)
		if err != nil {
			projection := providerRuntimeStoppingProjection(current, true)
			return &projection, normalizeProviderRuntimeError(ctx, err)
		}
		desiredStop, err = ledger.SetDesiredStop(ctx, SetProviderExecutionDesiredStopRequest{
			Owner: owner, ExpectedVersion: ownedMetadata.Version, OperationID: input.OperationID,
		})
		if err != nil || desiredStop == nil {
			projection := providerRuntimeStoppingProjection(ownedMetadata, true)
			return &projection, normalizeProviderRuntimeNonNilError(ctx, err)
		}
	}
	if desiredStop.ArtifactStatus == domainappdev.ProviderExecutionArtifactPublishing {
		projection := providerRuntimeStoppingProjection(*desiredStop, false)
		projection.CanStart = false
		return &projection, nil
	}
	converged, launchSelection, launchExecutionID, launchTerminal, convergeErr :=
		orchestrator.convergeProviderLaunchForStop(ctx, ledger, owner, *desiredStop)
	if convergeErr != nil || converged == nil {
		projection := providerRuntimeStoppingProjection(*desiredStop, true)
		return &projection, normalizeProviderRuntimeNonNilError(ctx, convergeErr)
	}
	desiredStop = converged
	originalObserved := desiredStop.ObservedState
	if launchTerminal {
		originalObserved = domainappdev.ProviderExecutionObservedCanceled
	}
	cleanupPending, err := ledger.BeginCleanup(ctx, BeginProviderExecutionCleanupRequest{
		Owner: owner, ExpectedVersion: desiredStop.Version,
	})
	if err != nil || cleanupPending == nil {
		projection := providerRuntimeStoppingProjection(*desiredStop, true)
		return &projection, normalizeProviderRuntimeNonNilError(ctx, err)
	}

	completeOperationID := input.OperationID
	if originalObserved == domainappdev.ProviderExecutionObservedPending &&
		!cleanupPending.HasProviderExecution && launchExecutionID == "" {
		return orchestrator.completeProviderRuntimeCleanup(ctx, ledger, owner, *cleanupPending, completeOperationID)
	}

	recoveryMetadata := *cleanupPending
	providerExecutionID := launchExecutionID
	selection := launchSelection
	if selection == nil {
		recovery, loadErr := ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
			Owner: owner, ExpectedVersion: cleanupPending.Version,
			ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
		})
		if loadErr != nil || recovery == nil {
			projection := providerRuntimeStoppingProjection(*cleanupPending, true)
			return &projection, normalizeProviderRuntimeNonNilError(ctx, loadErr)
		}
		if recovery.Metadata.SpaceID != input.SpaceID || recovery.Metadata.ProjectID != input.ProjectID || recovery.Metadata.Generation != current.Generation ||
			recovery.Metadata.ProviderKey != orchestrator.config.ProviderKey || recovery.Metadata.ProviderScope != orchestrator.config.ProviderScope {
			projection := providerRuntimeStoppingProjection(*cleanupPending, true)
			return &projection, ErrProviderRuntimeUnavailable
		}
		recoveryMetadata = recovery.Metadata
		providerExecutionID = recovery.ProviderExecutionID()
		checkpoint, hasCheckpoint := recovery.Checkpoint()
		if !hasCheckpoint || !domainappdev.ValidProviderExecutionProviderID(providerExecutionID) {
			projection := providerRuntimeStoppingProjection(recovery.Metadata, true)
			return &projection, ErrProviderRuntimeUnavailable
		}
		resumeRouter, resumeOK := orchestrator.router.(ProviderRuntimeResumeRouter)
		if !resumeOK {
			projection := providerRuntimeStoppingProjection(recovery.Metadata, true)
			return &projection, ErrProviderRuntimeUnavailable
		}
		statusSelection, resumeErr := resumeRouter.Resume(ctx, checkpoint, providerExecutionID)
		if resumeErr != nil || statusSelection == nil {
			projection := providerRuntimeStoppingProjection(recovery.Metadata, true)
			return &projection, normalizeProviderRuntimeNonNilError(ctx, resumeErr)
		}
		var cleanupOK bool
		selection, cleanupOK = statusSelection.(ProviderRuntimeCleanupSelection)
		if !cleanupOK {
			projection := providerRuntimeStoppingProjection(recovery.Metadata, true)
			return &projection, ErrProviderRuntimeUnavailable
		}
	}

	if !providerRuntimeObservedTerminal(originalObserved) {
		cancelOperationID := providerRuntimeStableID("appdev_stop_cancel", input.SpaceID, input.ProjectID, input.OperationID)
		if err := selection.Cancel(ctx, cancelOperationID); err != nil {
			projection := providerRuntimeStoppingProjection(recoveryMetadata, true)
			return &projection, normalizeProviderRuntimeError(ctx, err)
		}
		result, statusErr := selection.Status(ctx)
		if statusErr != nil {
			projection := providerRuntimeStoppingProjection(recoveryMetadata, true)
			return &projection, normalizeProviderRuntimeError(ctx, statusErr)
		}
		if result.ExecutionID != providerExecutionID {
			projection := providerRuntimeStoppingProjection(recoveryMetadata, true)
			return &projection, ErrProviderRuntimeUnavailable
		}
		switch result.Status {
		case infrasandbox.ExecutionStatusAccepted, infrasandbox.ExecutionStatusRunning:
			projection := providerRuntimeStoppingProjection(recoveryMetadata, false)
			return &projection, nil
		case infrasandbox.ExecutionStatusSucceeded, infrasandbox.ExecutionStatusFailed, infrasandbox.ExecutionStatusCanceled, infrasandbox.ExecutionStatusTimedOut:
		default:
			projection := providerRuntimeStoppingProjection(recoveryMetadata, true)
			return &projection, ErrProviderRuntimeUnavailable
		}
	}

	if err := selection.Release(ctx); err != nil {
		projection := providerRuntimeStoppingProjection(recoveryMetadata, true)
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	return orchestrator.completeProviderRuntimeCleanup(ctx, ledger, owner, recoveryMetadata, completeOperationID)
}

func (orchestrator *ProviderRuntimeOrchestrator) convergeProviderLaunchForStop(
	ctx context.Context,
	ledger ProviderRuntimeCleanupLedger,
	owner ProviderExecutionOwner,
	metadata ProviderExecutionMetadata,
) (*ProviderExecutionMetadata, ProviderRuntimeCleanupSelection, string, bool, error) {
	switch metadata.LaunchState {
	case domainappdev.ProviderExecutionLaunchPrepared,
		domainappdev.ProviderExecutionLaunchSubmitted,
		domainappdev.ProviderExecutionLaunchLegacySubmitted:
	case domainappdev.ProviderExecutionLaunchQuarantined:
		return &metadata, nil, "", false, ErrProviderRuntimeUnavailable
	default:
		return &metadata, nil, "", false, nil
	}
	recovery, err := ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
		Owner: owner, ExpectedVersion: metadata.Version,
		ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
	})
	if err != nil || recovery == nil {
		return &metadata, nil, "", false, normalizeProviderRuntimeNonNilError(ctx, err)
	}
	startOperationID, validStartOperation := recovery.startOperation()
	aborter, abortOK := orchestrator.ledger.(providerRuntimeLaunchAborter)
	if !validStartOperation || !abortOK {
		return &metadata, nil, "", false, ErrProviderRuntimeUnavailable
	}
	if metadata.LaunchState == domainappdev.ProviderExecutionLaunchPrepared {
		aborted, abortErr := aborter.AbortProviderLaunch(ctx, AbortProviderExecutionLaunchRequest{
			Owner: owner, ExpectedVersion: metadata.Version, OperationID: startOperationID,
			ExpectedState: domainappdev.ProviderExecutionLaunchPrepared,
		})
		return aborted, nil, "", false, abortErr
	}
	operationID, requestDigest, legacy, validLookup := recovery.launchLookupIdentity()
	lookupRouter, lookupOK := orchestrator.router.(ProviderRuntimeLaunchLookupRouter)
	completeLedger, completeOK := orchestrator.ledger.(providerRuntimeReconciledLaunchLedger)
	if !validLookup || !lookupOK || !completeOK {
		return &metadata, nil, "", false, ErrProviderRuntimeUnavailable
	}
	lookup, lookupErr := lookupRouter.LookupProviderExecution(
		ctx,
		providerRuntimeLaunchLookupRequest(metadata, operationID, requestDigest, legacy),
	)
	if lookupErr != nil || lookup.Status == infrasandbox.ExecutionLookupUnknown {
		return &metadata, nil, "", false, normalizeProviderRuntimeNonNilError(ctx, lookupErr)
	}
	if lookup.Status == infrasandbox.ExecutionLookupNotFound {
		aborted, abortErr := aborter.AbortProviderLaunch(ctx, AbortProviderExecutionLaunchRequest{
			Owner: owner, ExpectedVersion: metadata.Version, OperationID: startOperationID,
			ExpectedState: metadata.LaunchState,
		})
		return aborted, nil, "", false, abortErr
	}
	normalized, normalizeErr := infrasandbox.NormalizeExecutionLookupResult(
		infrasandbox.ExecutionLookupResult{Status: lookup.Status, Execution: lookup.Execution},
		orchestrator.config.Policy.MaxOutputBytes,
	)
	preview, previewErr := infrasandbox.NormalizePreviewRoute(lookup.Execution.PreviewRoute)
	if normalizeErr != nil || previewErr != nil || lookup.Selection == nil ||
		!domainappdev.ValidProviderExecutionProviderID(normalized.Execution.ExecutionID) {
		return &metadata, nil, "", false, ErrProviderRuntimeUnavailable
	}
	checkpoint, checkpointErr := lookup.Selection.Checkpoint()
	if checkpointErr != nil {
		return &metadata, nil, "", false, normalizeProviderRuntimeError(ctx, checkpointErr)
	}
	completed, completeErr := completeLedger.CompleteReconciledProviderLaunch(
		ctx,
		CompleteReconciledProviderLaunchRequest{
			Owner: owner, ExpectedVersion: metadata.Version,
			LaunchOperationID: startOperationID,
			CheckpointOperationID: providerRuntimeStableID(
				"appdev_reconciled_checkpoint", metadata.SpaceID, metadata.ProjectID,
				strconv.FormatUint(metadata.Generation, 10), startOperationID,
			),
			ProviderExecutionID:   normalized.Execution.ExecutionID,
			ProviderLeaseDuration: orchestrator.config.ProviderLeaseDuration,
			PreviewRoute:          preview, Checkpoint: checkpoint,
		},
	)
	if completeErr != nil || completed == nil {
		return &metadata, nil, "", false, normalizeProviderRuntimeNonNilError(ctx, completeErr)
	}
	selection, cleanupOK := lookup.Selection.(ProviderRuntimeCleanupSelection)
	if !cleanupOK {
		return completed, nil, normalized.Execution.ExecutionID, false, ErrProviderRuntimeUnavailable
	}
	_, terminal := providerRuntimeTerminalState(normalized.Execution.Status)
	return completed, selection, normalized.Execution.ExecutionID, terminal, nil
}

func (orchestrator *ProviderRuntimeOrchestrator) completeProviderRuntimeCleanup(ctx context.Context, ledger ProviderRuntimeCleanupLedger, owner ProviderExecutionOwner, metadata ProviderExecutionMetadata, operationID string) (*ProviderRuntimeProjection, error) {
	completed, err := ledger.CompleteCleanup(ctx, CompleteProviderExecutionCleanupRequest{
		Owner: owner, ExpectedVersion: metadata.Version, OperationID: operationID,
	})
	if err != nil || completed == nil {
		projection := providerRuntimeStoppingProjection(metadata, true)
		return &projection, normalizeProviderRuntimeNonNilError(ctx, err)
	}
	projection := providerRuntimeProjection(*completed, ProviderRuntimeStateStopped)
	projection.CanStart = true
	projection.Stopping = false
	projection.Recovering = false
	projection.PreviewRoute = ""
	return &projection, nil
}

func validProviderRuntimeStopInput(input ProviderRuntimeStopInput) bool {
	return validProviderRuntimeStatusInput(ProviderRuntimeStatusInput{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: input.OperationID, ActorID: input.ActorID,
	})
}

func providerRuntimeStoppingProjection(metadata ProviderExecutionMetadata, recovering bool) ProviderRuntimeProjection {
	projection := providerRuntimeProjection(metadata, ProviderRuntimeStateCleanupPending)
	projection.CanStart = false
	projection.Stopping = true
	projection.Recovering = recovering
	return projection
}

func providerRuntimeObservedTerminal(state domainappdev.ProviderExecutionObservedState) bool {
	switch state {
	case domainappdev.ProviderExecutionObservedSucceeded, domainappdev.ProviderExecutionObservedFailed,
		domainappdev.ProviderExecutionObservedCanceled, domainappdev.ProviderExecutionObservedTimedOut:
		return true
	default:
		return false
	}
}

func normalizeProviderRuntimeNonNilError(ctx context.Context, err error) error {
	if err == nil {
		return ErrProviderRuntimeUnavailable
	}
	return normalizeProviderRuntimeError(ctx, err)
}

const providerRuntimeRecoveryActor = "provider-runtime-recovery"

type ProviderRuntimeRecoverProjectInput struct {
	SpaceID   string
	ProjectID string
}

type ProjectRecoveryScanner interface {
	RecoverProject(context.Context, ProviderRuntimeRecoverProjectInput) (*ProviderRuntimeProjection, error)
}

type ResumeOnlyProjectRecoveryScanner interface {
	RecoverProjectResumeOnly(context.Context, ProviderRuntimeRecoverProjectInput) (*ProviderRuntimeProjection, error)
}

// RecoverProjectResumeOnly is the startup reconciler path. It may claim and
// resume a durable execution, but it never resolves a provider or calls
// Execute. Records without both a real provider handle and an encrypted
// checkpoint remain recoverable for a later authoritative reconciliation.
func (orchestrator *ProviderRuntimeOrchestrator) RecoverProjectResumeOnly(
	ctx context.Context,
	input ProviderRuntimeRecoverProjectInput,
) (projection *ProviderRuntimeProjection, resultErr error) {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil || !validProviderRuntimeRecoverProjectInput(input) {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderRuntimeInvalid
	}
	ledger, ok := orchestrator.ledger.(ProviderRuntimeRecoveryLedger)
	if !ok {
		return nil, ErrProviderRuntimeUnavailable
	}
	records, err := ledger.ListRecoverable(ctx, ListRecoverableProviderExecutionsRequest{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, Limit: 16,
	})
	if err != nil {
		return nil, normalizeProviderRuntimeError(ctx, err)
	}
	if len(records) == 0 {
		return &ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}, nil
	}
	current := records[0]
	for _, candidate := range records {
		if candidate.SpaceID != input.SpaceID || candidate.ProjectID != input.ProjectID {
			return nil, ErrProviderRuntimeUnavailable
		}
		if candidate.Generation > current.Generation {
			current = candidate
		}
	}
	if current.Generation == 0 || current.ProviderKey != orchestrator.config.ProviderKey ||
		current.ProviderScope != orchestrator.config.ProviderScope {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		projection.CanStart = false
		return &projection, ErrProviderRuntimeUnavailable
	}
	launchRecovery := !current.HasProviderExecution && !current.HasCheckpoint &&
		(current.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
			current.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted ||
			current.LaunchState == domainappdev.ProviderExecutionLaunchLegacySubmitted ||
			current.LaunchState == domainappdev.ProviderExecutionLaunchQuarantined ||
			(current.LaunchState == domainappdev.ProviderExecutionLaunchAborted &&
				current.DesiredState == domainappdev.ProviderExecutionDesiredStop))
	if (!current.HasProviderExecution || !current.HasCheckpoint) && !launchRecovery {
		projection := providerRuntimeProjection(current, ProviderRuntimeStateStarting)
		projection.Recovering = true
		projection.CanStart = false
		return &projection, ErrProviderRuntimeUnavailable
	}
	var (
		owned ProviderExecutionMetadata
		owner ProviderExecutionOwner
	)
	if providerRuntimeLaunchNeedsReconciliation(current.LaunchState) {
		claimed, claimErr := orchestrator.claimLaunchReconciliationOwner(ctx, current, false)
		if claimErr != nil || claimed == nil {
			projection := providerRuntimeProjection(current, "")
			projection.Recovering = true
			projection.CanStart = false
			return &projection, normalizeProviderRuntimeNonNilError(ctx, claimErr)
		}
		owned, owner = claimed.Metadata, claimed.Owner()
	} else {
		owned, owner, err = orchestrator.renewOrClaimStatusOwner(ctx, ledger, current)
	}
	if err != nil {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		projection.CanStart = false
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	releaseOnReturn := true
	defer func() {
		if !releaseOnReturn {
			if projection != nil && (projection.CanStart ||
				projection.State == ProviderRuntimeStateStopped ||
				projection.State == ProviderRuntimeStateCleanupComplete) {
				projection.recoveryOwnership = ProviderRuntimeRecoveryOwnershipReleased
				return
			}
			if projection != nil {
				projection.recoveryOwnership = ProviderRuntimeRecoveryOwnershipRetained
			}
			if resultErr != nil && !IsProviderRuntimeRecoveryOwnerRetained(resultErr) {
				resultErr = MarkProviderRuntimeRecoveryOwnerRetained(resultErr)
			}
			return
		}
		releaseErr := orchestrator.releaseRecoveryClaim(ctx, ledger, owner)
		if releaseErr == nil {
			if projection != nil {
				projection.recoveryOwnership = ProviderRuntimeRecoveryOwnershipReleased
			}
			return
		}
		if projection != nil {
			projection.recoveryOwnership = ProviderRuntimeRecoveryOwnershipRetained
		}
		safeReleaseErr := fmt.Errorf("release provider runtime recovery owner: %w", ErrProviderRuntimeUnavailable)
		if resultErr == nil {
			resultErr = MarkProviderRuntimeRecoveryOwnerRetained(safeReleaseErr)
			return
		}
		resultErr = MarkProviderRuntimeRecoveryOwnerRetained(errors.Join(resultErr, safeReleaseErr))
	}()
	operationID := providerRuntimeStableID(
		"appdev_resume_only", input.SpaceID, input.ProjectID, strconv.FormatUint(current.Generation, 10),
	)
	if owned.LaunchState == domainappdev.ProviderExecutionLaunchPrepared ||
		owned.LaunchState == domainappdev.ProviderExecutionLaunchSubmitted ||
		owned.LaunchState == domainappdev.ProviderExecutionLaunchLegacySubmitted ||
		owned.LaunchState == domainappdev.ProviderExecutionLaunchQuarantined {
		if owned.LaunchState == domainappdev.ProviderExecutionLaunchQuarantined {
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, ErrProviderRuntimeUnavailable
		}
		recovery, loadErr := ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
			Owner: owner, ExpectedVersion: owned.Version,
			ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
		})
		if loadErr != nil || recovery == nil {
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, loadErr)
		}
		aborter, abortOK := orchestrator.ledger.(providerRuntimeLaunchAborter)
		if !abortOK {
			return nil, ErrProviderRuntimeUnavailable
		}
		if owned.LaunchState == domainappdev.ProviderExecutionLaunchPrepared {
			aborted, abortErr := aborter.AbortProviderLaunch(ctx, AbortProviderExecutionLaunchRequest{
				Owner: owner, ExpectedVersion: owned.Version,
				OperationID:   recovery.startOperationID,
				ExpectedState: domainappdev.ProviderExecutionLaunchPrepared,
			})
			if abortErr != nil || aborted == nil {
				recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
				recoveryProjection.Recovering = true
				recoveryProjection.CanStart = false
				return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, abortErr)
			}
			if owned.DesiredState == domainappdev.ProviderExecutionDesiredStop {
				releaseOnReturn = false
				generation := aborted.Generation
				return orchestrator.ContinueCleanup(ctx, ProviderRuntimeStopInput{
					SpaceID: input.SpaceID, ProjectID: input.ProjectID,
					OperationID: operationID, ActorID: providerRuntimeRecoveryActor,
					ExpectedGeneration: &generation,
				})
			}
			projection := providerRuntimeProjection(*aborted, ProviderRuntimeStatePending)
			projection.CanStart = false
			return &projection, nil
		}
		launchOperationID, requestDigest, legacy, identityOK := recovery.launchLookupIdentity()
		if !identityOK {
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, ErrProviderRuntimeUnavailable
		}
		lookupRouter, lookupOK := orchestrator.router.(ProviderRuntimeLaunchLookupRouter)
		completeLedger, completeOK := orchestrator.ledger.(providerRuntimeReconciledLaunchLedger)
		if !lookupOK || !completeOK {
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, ErrProviderRuntimeUnavailable
		}
		lookupResult, lookupErr := lookupRouter.LookupProviderExecution(
			ctx,
			providerRuntimeLaunchLookupRequest(owned, launchOperationID, requestDigest, legacy),
		)
		if lookupErr != nil || lookupResult.Status == infrasandbox.ExecutionLookupUnknown {
			releaseOnReturn = false
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, lookupErr)
		}
		if lookupResult.Status == infrasandbox.ExecutionLookupNotFound {
			aborted, abortErr := aborter.AbortProviderLaunch(ctx, AbortProviderExecutionLaunchRequest{
				Owner: owner, ExpectedVersion: owned.Version,
				OperationID:   recovery.startOperationID,
				ExpectedState: owned.LaunchState,
			})
			if abortErr != nil || aborted == nil {
				recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
				recoveryProjection.Recovering = true
				recoveryProjection.CanStart = false
				return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, abortErr)
			}
			if owned.DesiredState == domainappdev.ProviderExecutionDesiredStop {
				releaseOnReturn = false
				generation := aborted.Generation
				return orchestrator.ContinueCleanup(ctx, ProviderRuntimeStopInput{
					SpaceID: input.SpaceID, ProjectID: input.ProjectID,
					OperationID: operationID, ActorID: providerRuntimeRecoveryActor,
					ExpectedGeneration: &generation,
				})
			}
			projection := providerRuntimeProjection(*aborted, ProviderRuntimeStatePending)
			projection.CanStart = false
			return &projection, nil
		}
		normalizedLookup, normalizeErr := infrasandbox.NormalizeExecutionLookupResult(
			infrasandbox.ExecutionLookupResult{
				Status: lookupResult.Status, Execution: lookupResult.Execution,
			},
			orchestrator.config.Policy.MaxOutputBytes,
		)
		preview, previewErr := infrasandbox.NormalizePreviewRoute(lookupResult.Execution.PreviewRoute)
		if normalizeErr != nil || previewErr != nil || lookupResult.Selection == nil ||
			!domainappdev.ValidProviderExecutionProviderID(lookupResult.Execution.ExecutionID) {
			releaseOnReturn = false
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, ErrProviderRuntimeUnavailable
		}
		checkpoint, checkpointErr := lookupResult.Selection.Checkpoint()
		if checkpointErr != nil {
			releaseOnReturn = false
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, normalizeProviderRuntimeError(ctx, checkpointErr)
		}
		completed, completeErr := completeLedger.CompleteReconciledProviderLaunch(
			ctx,
			CompleteReconciledProviderLaunchRequest{
				Owner: owner, ExpectedVersion: owned.Version,
				LaunchOperationID: recovery.startOperationID,
				CheckpointOperationID: providerRuntimeStableID(
					"appdev_reconciled_checkpoint", input.SpaceID, input.ProjectID,
					strconv.FormatUint(owned.Generation, 10), recovery.startOperationID,
				),
				ProviderExecutionID:   normalizedLookup.Execution.ExecutionID,
				ProviderLeaseDuration: orchestrator.config.ProviderLeaseDuration,
				PreviewRoute:          preview, Checkpoint: checkpoint,
			},
		)
		releaseOnReturn = false
		if completeErr != nil || completed == nil {
			recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
			recoveryProjection.Recovering = true
			recoveryProjection.CanStart = false
			return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, completeErr)
		}
		if owned.DesiredState == domainappdev.ProviderExecutionDesiredStop {
			generation := completed.Generation
			return orchestrator.ContinueCleanup(ctx, ProviderRuntimeStopInput{
				SpaceID: input.SpaceID, ProjectID: input.ProjectID,
				OperationID: operationID, ActorID: providerRuntimeRecoveryActor,
				ExpectedGeneration: &generation,
			})
		}
		final := completed
		if terminalState, terminal := providerRuntimeTerminalState(normalizedLookup.Execution.Status); terminal {
			final, completeErr = orchestrator.ledger.AdvanceTerminal(ctx, AdvanceProviderExecutionTerminalRequest{
				Owner: owner, ExpectedVersion: completed.Version,
				OperationID: providerRuntimeStableID(
					"appdev_reconciled_terminal", input.SpaceID, input.ProjectID,
					strconv.FormatUint(owned.Generation, 10), recovery.startOperationID,
				),
				ObservedState: terminalState, PreviewRoute: preview,
				SafeErrorCode:    providerRuntimeTerminalErrorCode(normalizedLookup.Execution.Status),
				SafeErrorMessage: providerRuntimeTerminalErrorMessage(normalizedLookup.Execution.Status),
			})
			if completeErr != nil || final == nil {
				recoveryProjection := providerRuntimeProjection(*completed, providerRuntimeResultState(normalizedLookup.Execution.Status))
				recoveryProjection.Recovering = true
				return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, completeErr)
			}
		}
		projection := providerRuntimeProjection(*final, providerRuntimeResultState(normalizedLookup.Execution.Status))
		projection.PreviewRoute = preview
		return &projection, nil
	}
	if owned.DesiredState == domainappdev.ProviderExecutionDesiredStop ||
		owned.ObservedState == domainappdev.ProviderExecutionObservedCleanupPending ||
		providerRuntimeObservedTerminal(owned.ObservedState) {
		generation := owned.Generation
		cleanupProjection, cleanupErr := orchestrator.ContinueCleanup(ctx, ProviderRuntimeStopInput{
			SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: operationID,
			ActorID: providerRuntimeRecoveryActor, ExpectedGeneration: &generation,
		})
		return providerRuntimeRecoveryProjection(current.Generation, cleanupProjection, cleanupErr)
	}
	recovery, loadErr := ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
		Owner: owner, ExpectedVersion: owned.Version,
		ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
	})
	if loadErr != nil || recovery == nil || recovery.ProviderExecutionID() == "" || !recovery.HasCheckpoint() {
		recoveryProjection := providerRuntimeProjection(owned, ProviderRuntimeStateStarting)
		recoveryProjection.Recovering = true
		recoveryProjection.CanStart = false
		return &recoveryProjection, normalizeProviderRuntimeNonNilError(ctx, loadErr)
	}
	statusProjection, statusErr := orchestrator.status(ctx, ProviderRuntimeStatusInput{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: operationID,
		ActorID: providerRuntimeRecoveryActor,
	}, true)
	recoveredProjection, recoveredErr := providerRuntimeRecoveryProjection(
		current.Generation,
		statusProjection,
		statusErr,
	)
	if recoveredErr == nil && recoveredProjection != nil &&
		!recoveredProjection.CanStart &&
		recoveredProjection.State != ProviderRuntimeStateStopped &&
		recoveredProjection.State != ProviderRuntimeStateCleanupComplete &&
		!providerRuntimeProjectionTerminal(recoveredProjection.State) {
		releaseOnReturn = false
	}
	return recoveredProjection, recoveredErr
}

func (orchestrator *ProviderRuntimeOrchestrator) releaseRecoveryClaim(
	parent context.Context,
	ledger ProviderRuntimeRecoveryLedger,
	owner ProviderExecutionOwner,
) error {
	if orchestrator == nil || ledger == nil || owner.generation == 0 || owner.epoch == 0 {
		return ErrProviderRuntimeUnavailable
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	current, err := ledger.LoadCurrent(cleanupCtx, LoadCurrentProviderExecutionRequest{
		SpaceID: owner.spaceID, ProjectID: owner.projectID,
	})
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		return nil
	}
	if err != nil || current == nil {
		return normalizeProviderRuntimeNonNilError(cleanupCtx, err)
	}
	if current.Generation != owner.generation || current.OwnerEpoch != owner.epoch {
		return nil
	}
	_, err = ledger.ReleaseOwner(cleanupCtx, ReleaseProviderExecutionOwnerRequest{
		Owner:           owner,
		ExpectedVersion: current.Version,
		OperationID: providerRuntimeStableID(
			"appdev_resume_only_release",
			owner.spaceID,
			owner.projectID,
			strconv.FormatUint(owner.generation, 10),
			strconv.FormatUint(owner.epoch, 10),
		),
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, domainappdev.ErrProviderExecutionConflict) ||
		errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) ||
		errors.Is(err, domainappdev.ErrProviderExecutionVersionConflict) {
		return nil
	}
	return normalizeProviderRuntimeNonNilError(cleanupCtx, err)
}

func providerRuntimeProjectionTerminal(state ProviderRuntimeState) bool {
	switch state {
	case ProviderRuntimeStateSucceeded, ProviderRuntimeStateFailed,
		ProviderRuntimeStateCanceled, ProviderRuntimeStateTimedOut:
		return true
	default:
		return false
	}
}

func (orchestrator *ProviderRuntimeOrchestrator) RecoverProject(ctx context.Context, input ProviderRuntimeRecoverProjectInput) (*ProviderRuntimeProjection, error) {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil || !validProviderRuntimeRecoverProjectInput(input) {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderRuntimeInvalid
	}
	ledger, ok := orchestrator.ledger.(ProviderRuntimeRecoveryLedger)
	if !ok {
		return nil, ErrProviderRuntimeUnavailable
	}
	records, err := ledger.ListRecoverable(ctx, ListRecoverableProviderExecutionsRequest{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, Limit: 16,
	})
	if err != nil {
		return nil, normalizeProviderRuntimeError(ctx, err)
	}
	if len(records) == 0 {
		projection := ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}
		return &projection, nil
	}
	current := records[0]
	for _, candidate := range records {
		if candidate.SpaceID != input.SpaceID || candidate.ProjectID != input.ProjectID {
			return nil, ErrProviderRuntimeUnavailable
		}
		if candidate.Generation > current.Generation {
			current = candidate
		}
	}
	if current.Generation == 0 || current.ProviderKey != orchestrator.config.ProviderKey || current.ProviderScope != orchestrator.config.ProviderScope {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		projection.CanStart = false
		return &projection, ErrProviderRuntimeUnavailable
	}

	ownedMetadata, owner, err := orchestrator.renewOrClaimStatusOwner(ctx, ledger, current)
	if err != nil {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		projection.CanStart = false
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	operationID := providerRuntimeStableID("appdev_recover", input.SpaceID, input.ProjectID, fmt.Sprintf("%d", current.Generation))

	if ownedMetadata.DesiredState == domainappdev.ProviderExecutionDesiredStop ||
		ownedMetadata.ObservedState == domainappdev.ProviderExecutionObservedCleanupPending ||
		providerRuntimeObservedTerminal(ownedMetadata.ObservedState) {
		projection, cleanupErr := orchestrator.ContinueCleanup(ctx, ProviderRuntimeStopInput{
			SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: operationID, ActorID: providerRuntimeRecoveryActor,
		})
		return providerRuntimeRecoveryProjection(current.Generation, projection, cleanupErr)
	}

	switch ownedMetadata.ObservedState {
	case domainappdev.ProviderExecutionObservedRunning:
		projection, statusErr := orchestrator.Status(ctx, ProviderRuntimeStatusInput{
			SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: operationID, ActorID: providerRuntimeRecoveryActor,
		})
		return providerRuntimeRecoveryProjection(current.Generation, projection, statusErr)
	case domainappdev.ProviderExecutionObservedPending, domainappdev.ProviderExecutionObservedSubmitting:
		recovery, loadErr := ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
			Owner: owner, ExpectedVersion: ownedMetadata.Version,
			ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
		})
		if loadErr != nil || recovery == nil {
			projection := providerRuntimeProjection(ownedMetadata, ProviderRuntimeStateStarting)
			projection.Recovering = true
			projection.CanStart = false
			return &projection, normalizeProviderRuntimeNonNilError(ctx, loadErr)
		}
		if recovery.Metadata.SpaceID != input.SpaceID || recovery.Metadata.ProjectID != input.ProjectID ||
			recovery.Metadata.Generation != current.Generation || recovery.Metadata.ProviderKey != orchestrator.config.ProviderKey ||
			recovery.Metadata.ProviderScope != orchestrator.config.ProviderScope {
			projection := providerRuntimeProjection(ownedMetadata, ProviderRuntimeStateStarting)
			projection.Recovering = true
			projection.CanStart = false
			return &projection, ErrProviderRuntimeUnavailable
		}
		startOperationID, valid := recovery.startOperation()
		if !valid {
			projection := providerRuntimeProjection(recovery.Metadata, ProviderRuntimeStateStarting)
			projection.Recovering = true
			projection.CanStart = false
			return &projection, ErrProviderRuntimeUnavailable
		}
		projection, startErr := orchestrator.Start(ctx, ProviderRuntimeStartInput{
			SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: startOperationID, ActorID: providerRuntimeRecoveryActor,
		})
		return providerRuntimeRecoveryProjection(current.Generation, projection, startErr)
	default:
		projection := providerRuntimeProjection(ownedMetadata, "")
		projection.Recovering = true
		projection.CanStart = false
		return &projection, ErrProviderRuntimeUnavailable
	}
}

func validProviderRuntimeRecoverProjectInput(input ProviderRuntimeRecoverProjectInput) bool {
	return domainappdev.ValidProviderExecutionSpaceID(input.SpaceID) && domainappdev.ValidProviderExecutionProjectID(input.ProjectID)
}

func providerRuntimeRecoveryProjection(expectedGeneration uint64, projection *ProviderRuntimeProjection, err error) (*ProviderRuntimeProjection, error) {
	if projection == nil {
		if err == nil {
			err = ErrProviderRuntimeUnavailable
		}
		return nil, err
	}
	if projection.Generation != expectedGeneration && !(projection.Generation == 0 && projection.State == ProviderRuntimeStateStopped && projection.CanStart) {
		projection.Recovering = true
		projection.CanStart = false
		return projection, ErrProviderRuntimeUnavailable
	}
	if err != nil {
		projection.Recovering = true
		projection.CanStart = false
	}
	return projection, err
}

func providerRuntimeStableID(prefix string, values ...string) string {
	hasher := sha256.New()
	for _, value := range values {
		_, _ = hasher.Write([]byte(value))
		_, _ = hasher.Write([]byte{0})
	}
	return prefix + "_" + hex.EncodeToString(hasher.Sum(nil))
}

func providerRuntimeDispatchLeaseDuration(executeTimeout time.Duration) (time.Duration, bool) {
	if executeTimeout <= 0 || executeTimeout > time.Duration(1<<63-1)-providerRuntimeDispatchLeaseMargin {
		return 0, false
	}
	return executeTimeout + providerRuntimeDispatchLeaseMargin, true
}

func validProviderRuntimeStartInput(input ProviderRuntimeStartInput) bool {
	if !domainappdev.ValidProviderExecutionSpaceID(input.SpaceID) || !domainappdev.ValidProviderExecutionProjectID(input.ProjectID) || strings.TrimSpace(input.ActorID) == "" || len(input.ActorID) > 128 || !utf8.ValidString(input.ActorID) || strings.ContainsAny(input.ActorID, "\x00\r\n\t") {
		return false
	}
	_, err := domainappdev.HashProviderExecutionOperationID(input.OperationID)
	return err == nil
}

func validProviderRuntimeDownloadEndpoint(value string) bool {
	return NewArtifactCapabilityURLPolicy(false).Validate(value) == nil
}

func cloneProviderRuntimeEnv(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func normalizeProviderRuntimeError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) || errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) || errors.Is(err, domainappdev.ErrProviderExecutionVersionConflict) || errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		return ErrProviderRuntimeConflict
	}
	return ErrProviderRuntimeUnavailable
}

type ProviderRuntimeStatusInput struct {
	SpaceID     string
	ProjectID   string
	OperationID string
	ActorID     string
}

type ProviderRuntimeStatusLedger interface {
	LoadCurrent(context.Context, LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error)
	RenewOwner(context.Context, RenewProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error)
	LoadRecovery(context.Context, LoadProviderExecutionRecoveryRequest) (*ProviderExecutionRecovery, error)
	ObserveActive(context.Context, ObserveProviderExecutionActiveRequest) (*ProviderExecutionMetadata, error)
}

type ProviderRuntimeRecoveryLedger interface {
	ProviderRuntimeStatusLedger
	ListRecoverable(context.Context, ListRecoverableProviderExecutionsRequest) ([]ProviderExecutionMetadata, error)
	ReleaseOwner(context.Context, ReleaseProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error)
}

type ProviderRuntimeStatusSelection interface {
	KeepAlive(context.Context) error
	Status(context.Context) (infrasandbox.ExecuteResult, error)
	Checkpoint() (applicationsandbox.ExecutionCheckpoint, error)
}

type ProviderRuntimeResumeRouter interface {
	Resume(context.Context, applicationsandbox.ExecutionCheckpoint, string) (ProviderRuntimeStatusSelection, error)
}

func (adapter SandboxProviderRuntimeRouter) Resume(ctx context.Context, checkpoint applicationsandbox.ExecutionCheckpoint, executionID string) (ProviderRuntimeStatusSelection, error) {
	if adapter.Router == nil {
		return nil, ErrProviderRuntimeUnavailable
	}
	return adapter.Router.Resume(ctx, checkpoint, executionID)
}

func (orchestrator *ProviderRuntimeOrchestrator) Status(ctx context.Context, input ProviderRuntimeStatusInput) (*ProviderRuntimeProjection, error) {
	return orchestrator.status(ctx, input, false)
}

func (orchestrator *ProviderRuntimeOrchestrator) status(
	ctx context.Context,
	input ProviderRuntimeStatusInput,
	resumeSubmitting bool,
) (*ProviderRuntimeProjection, error) {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil || !validProviderRuntimeStatusInput(input) {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrProviderRuntimeInvalid
	}
	ledger, ok := orchestrator.ledger.(ProviderRuntimeStatusLedger)
	if !ok {
		return nil, ErrProviderRuntimeUnavailable
	}
	currentRecord, err := ledger.LoadCurrent(ctx, LoadCurrentProviderExecutionRequest{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID,
	})
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		return &ProviderRuntimeProjection{State: ProviderRuntimeStateStopped, CanStart: true}, nil
	}
	if err != nil || currentRecord == nil {
		return nil, normalizeProviderRuntimeError(ctx, err)
	}
	current := *currentRecord
	if current.SpaceID != input.SpaceID || current.ProjectID != input.ProjectID {
		return nil, ErrProviderRuntimeUnavailable
	}
	if current.ProviderKey != orchestrator.config.ProviderKey || current.ProviderScope != orchestrator.config.ProviderScope || current.Generation == 0 {
		return nil, ErrProviderRuntimeUnavailable
	}
	if !resumeSubmitting &&
		(current.ObservedState == domainappdev.ProviderExecutionObservedPending ||
			current.ObservedState == domainappdev.ProviderExecutionObservedSubmitting) {
		projection := providerRuntimeProjection(current, ProviderRuntimeStateStarting)
		projection.Recovering = true
		return &projection, nil
	}
	resumableState := providerRuntimeObservedState(current.ObservedState) == ProviderRuntimeStateRunning ||
		(resumeSubmitting &&
			(current.ObservedState == domainappdev.ProviderExecutionObservedPending ||
				current.ObservedState == domainappdev.ProviderExecutionObservedSubmitting))
	if current.DesiredState == domainappdev.ProviderExecutionDesiredStop ||
		current.ObservedState == domainappdev.ProviderExecutionObservedCleanupPending ||
		!resumableState {
		projection := providerRuntimeProjection(current, "")
		return &projection, nil
	}

	ownedMetadata, owner, err := orchestrator.renewOrClaimStatusOwner(ctx, ledger, current)
	if err != nil {
		projection := providerRuntimeProjection(current, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	recovery, err := ledger.LoadRecovery(ctx, LoadProviderExecutionRecoveryRequest{
		Owner: owner, ExpectedVersion: ownedMetadata.Version,
		ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
	})
	if err != nil || recovery == nil {
		projection := providerRuntimeProjection(ownedMetadata, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	if recovery.Metadata.SpaceID != input.SpaceID || recovery.Metadata.ProjectID != input.ProjectID || recovery.Metadata.Generation != current.Generation ||
		recovery.Metadata.ProviderKey != orchestrator.config.ProviderKey || recovery.Metadata.ProviderScope != orchestrator.config.ProviderScope {
		projection := providerRuntimeProjection(ownedMetadata, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	providerExecutionID := recovery.ProviderExecutionID()
	checkpoint, hasCheckpoint := recovery.Checkpoint()
	if !hasCheckpoint || !domainappdev.ValidProviderExecutionProviderID(providerExecutionID) {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	resumeRouter, ok := orchestrator.router.(ProviderRuntimeResumeRouter)
	if !ok {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	selection, err := resumeRouter.Resume(ctx, checkpoint, providerExecutionID)
	if err != nil || selection == nil {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	if err := selection.KeepAlive(ctx); err != nil {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	result, err := selection.Status(ctx)
	if err != nil {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, normalizeProviderRuntimeError(ctx, err)
	}
	if result.ExecutionID != providerExecutionID {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	preview, err := infrasandbox.NormalizePreviewRoute(result.PreviewRoute)
	if err != nil {
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
	if preview == "" {
		preview = recovery.Metadata.PreviewRoute
	}

	switch result.Status {
	case infrasandbox.ExecutionStatusAccepted, infrasandbox.ExecutionStatusRunning:
		observed, observeErr := ledger.ObserveActive(ctx, ObserveProviderExecutionActiveRequest{
			Owner: owner, ExpectedVersion: recovery.Metadata.Version, ProviderExecutionID: providerExecutionID,
			ProviderLeaseDuration: orchestrator.config.ProviderLeaseDuration, PreviewRoute: preview,
		})
		if observeErr != nil || observed == nil {
			projection := providerRuntimeProjection(recovery.Metadata, ProviderRuntimeStateRunning)
			projection.Recovering = true
			return &projection, normalizeProviderRuntimeError(ctx, observeErr)
		}
		refreshed, checkpointErr := selection.Checkpoint()
		if checkpointErr != nil {
			projection := providerRuntimeProjection(*observed, ProviderRuntimeStateRunning)
			projection.PreviewRoute, projection.Recovering = preview, true
			return &projection, normalizeProviderRuntimeError(ctx, checkpointErr)
		}
		checkpointMetadata, checkpointErr := orchestrator.checkpoints.SaveCheckpoint(ctx, SaveProviderExecutionCheckpointRequest{
			Owner: owner, ExpectedVersion: observed.Version,
			OperationID: providerRuntimeStableID("appdev_status_checkpoint", input.SpaceID, input.ProjectID, input.OperationID),
			ProviderKey: orchestrator.config.ProviderKey, ProviderScope: orchestrator.config.ProviderScope,
			Checkpoint: refreshed,
		})
		if checkpointErr != nil || checkpointMetadata == nil {
			projection := providerRuntimeProjection(*observed, ProviderRuntimeStateRunning)
			projection.PreviewRoute, projection.Recovering = preview, true
			return &projection, normalizeProviderRuntimeError(ctx, checkpointErr)
		}
		projection := providerRuntimeProjection(*checkpointMetadata, ProviderRuntimeStateRunning)
		projection.PreviewRoute = preview
		return &projection, nil
	case infrasandbox.ExecutionStatusSucceeded, infrasandbox.ExecutionStatusFailed, infrasandbox.ExecutionStatusCanceled, infrasandbox.ExecutionStatusTimedOut:
		observedState, _ := providerRuntimeTerminalState(result.Status)
		terminal, terminalErr := orchestrator.ledger.AdvanceTerminal(ctx, AdvanceProviderExecutionTerminalRequest{
			Owner: owner, ExpectedVersion: recovery.Metadata.Version,
			OperationID:   providerRuntimeStableID("appdev_status_terminal", input.SpaceID, input.ProjectID, input.OperationID),
			ObservedState: observedState, PreviewRoute: preview,
			SafeErrorCode: providerRuntimeTerminalErrorCode(result.Status), SafeErrorMessage: providerRuntimeTerminalErrorMessage(result.Status),
		})
		if terminalErr != nil || terminal == nil {
			projection := providerRuntimeProjection(recovery.Metadata, "")
			projection.Recovering = true
			return &projection, normalizeProviderRuntimeError(ctx, terminalErr)
		}
		projection := providerRuntimeProjection(*terminal, providerRuntimeResultState(result.Status))
		projection.PreviewRoute = preview
		return &projection, nil
	default:
		projection := providerRuntimeProjection(recovery.Metadata, "")
		projection.Recovering = true
		return &projection, ErrProviderRuntimeUnavailable
	}
}

func (orchestrator *ProviderRuntimeOrchestrator) renewOrClaimStatusOwner(ctx context.Context, ledger ProviderRuntimeStatusLedger, metadata ProviderExecutionMetadata) (ProviderExecutionMetadata, ProviderExecutionOwner, error) {
	if metadata.OwnerEpoch > 0 {
		owner := providerRuntimeOwnerFromMetadata(metadata, orchestrator.ownerToken)
		renewed, err := ledger.RenewOwner(ctx, RenewProviderExecutionOwnerRequest{Owner: owner, ExpectedVersion: metadata.Version})
		if err == nil && renewed != nil {
			return *renewed, providerRuntimeOwnerFromMetadata(*renewed, orchestrator.ownerToken), nil
		}
		if !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
			return ProviderExecutionMetadata{}, ProviderExecutionOwner{}, err
		}
	}
	claim := ClaimProviderExecutionRecoveryRequest{
		SpaceID: metadata.SpaceID, ProjectID: metadata.ProjectID, Generation: metadata.Generation,
		ExpectedVersion: metadata.Version,
	}
	if metadata.OwnerEpoch == 0 {
		claim.ProposedOwner = orchestrator.ownerToken
	} else {
		claim.ExistingOwner = orchestrator.ownerToken
		claim.ExistingOwnerEpoch = metadata.OwnerEpoch
	}
	claimed, err := orchestrator.ledger.ClaimRecovery(ctx, claim)
	if err != nil || claimed == nil {
		return ProviderExecutionMetadata{}, ProviderExecutionOwner{}, err
	}
	return claimed.Metadata, claimed.Owner(), nil
}

func providerRuntimeLaunchNeedsReconciliation(state domainappdev.ProviderExecutionLaunchState) bool {
	return state == domainappdev.ProviderExecutionLaunchPrepared ||
		state == domainappdev.ProviderExecutionLaunchSubmitted ||
		state == domainappdev.ProviderExecutionLaunchLegacySubmitted
}

func providerRuntimeLaunchLookupVersion(legacy bool) ProviderRuntimeLaunchLookupVersion {
	if legacy {
		return ProviderRuntimeLaunchLookupVersionLegacy
	}
	return ProviderRuntimeLaunchLookupVersionCurrent
}

func providerRuntimeLaunchLookupRequest(
	metadata ProviderExecutionMetadata,
	operationID string,
	requestDigest infrasandbox.ExecutionRequestDigest,
	legacy bool,
) ProviderRuntimeLaunchLookupRequest {
	request := ProviderRuntimeLaunchLookupRequest{
		ProviderKey: metadata.ProviderKey, ProviderScope: metadata.ProviderScope,
		OperationID: operationID, RequestDigest: requestDigest,
		Version: providerRuntimeLaunchLookupVersion(legacy),
	}
	if legacy {
		request.SpaceID = metadata.SpaceID
		request.ProjectID = metadata.ProjectID
		request.Generation = metadata.Generation
	}
	return request
}

func (orchestrator *ProviderRuntimeOrchestrator) claimLaunchReconciliationOwner(
	ctx context.Context,
	metadata ProviderExecutionMetadata,
	setDesiredStop bool,
) (*ClaimedProviderExecution, error) {
	ledger, ok := orchestrator.ledger.(providerRuntimeLaunchReconciliationLedger)
	if !ok {
		return nil, ErrProviderRuntimeUnavailable
	}
	request := ClaimProviderExecutionLaunchReconciliationRequest{
		SpaceID: metadata.SpaceID, ProjectID: metadata.ProjectID,
		Generation: metadata.Generation, ExpectedVersion: metadata.Version,
		ExpectedState: metadata.LaunchState, SetDesiredStop: setDesiredStop,
	}
	if metadata.OwnerEpoch == 0 {
		request.ProposedOwner = orchestrator.ownerToken
	} else {
		request.ExistingOwner = orchestrator.ownerToken
		request.ExistingOwnerEpoch = metadata.OwnerEpoch
	}
	return ledger.ClaimLaunchReconciliation(ctx, request)
}

func providerRuntimeOwnerFromMetadata(metadata ProviderExecutionMetadata, token domainappdev.ProviderExecutionOwnerToken) ProviderExecutionOwner {
	return ProviderExecutionOwner{
		spaceID: metadata.SpaceID, projectID: metadata.ProjectID, generation: metadata.Generation,
		providerKey: metadata.ProviderKey, providerScope: metadata.ProviderScope, token: token, epoch: metadata.OwnerEpoch,
	}
}

// ReleaseRecoveryOwner releases only this orchestrator instance's owner
// capability. It is used by the bounded production recovery scheduler during
// shutdown and cannot release a replacement owner's lease.
func (orchestrator *ProviderRuntimeOrchestrator) ReleaseRecoveryOwner(
	ctx context.Context,
	input ProviderRuntimeRecoverProjectInput,
) error {
	if orchestrator == nil || ctx == nil || ctx.Err() != nil ||
		!domainappdev.ValidProviderExecutionSpaceID(input.SpaceID) ||
		!domainappdev.ValidProviderExecutionProjectID(input.ProjectID) {
		return normalizeProviderRuntimeNonNilError(ctx, ErrProviderRuntimeInvalid)
	}
	ledger, ok := orchestrator.ledger.(interface {
		LoadCurrent(context.Context, LoadCurrentProviderExecutionRequest) (*ProviderExecutionMetadata, error)
		ReleaseOwner(context.Context, ReleaseProviderExecutionOwnerRequest) (*ProviderExecutionMetadata, error)
	})
	if !ok {
		return ErrProviderRuntimeUnavailable
	}
	metadata, err := ledger.LoadCurrent(ctx, LoadCurrentProviderExecutionRequest{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID,
	})
	if errors.Is(err, domainappdev.ErrProviderExecutionNotFound) {
		return nil
	}
	if err != nil || metadata == nil || metadata.OwnerEpoch == 0 {
		return normalizeProviderRuntimeNonNilError(ctx, err)
	}
	operationID := "scheduler-owner-release-" + strconv.FormatUint(metadata.Generation, 10)
	_, err = ledger.ReleaseOwner(ctx, ReleaseProviderExecutionOwnerRequest{
		Owner:           providerRuntimeOwnerFromMetadata(*metadata, orchestrator.ownerToken),
		ExpectedVersion: metadata.Version,
		OperationID:     operationID,
	})
	if errors.Is(err, domainappdev.ErrProviderExecutionConflict) ||
		errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		return nil
	}
	return normalizeProviderRuntimeNonNilError(ctx, err)
}

func validProviderRuntimeStatusInput(input ProviderRuntimeStatusInput) bool {
	return validProviderRuntimeStartInput(ProviderRuntimeStartInput{
		SpaceID: input.SpaceID, ProjectID: input.ProjectID, OperationID: input.OperationID, ActorID: input.ActorID,
	})
}

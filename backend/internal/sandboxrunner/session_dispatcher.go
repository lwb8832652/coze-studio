// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	"github.com/google/uuid"
)

const sessionDispatcherRecoveryReason = "aio_runtime_restarted"

func sessionPersistenceTime(value time.Time) time.Time {
	return value.UTC().Truncate(time.Millisecond)
}

// SessionLeaseManager serializes every operation that targets the same
// logical Runtime Session across Runner processes. Acquire uses an identity
// digest before a stable Session ID exists.
type SessionLeaseManager interface {
	Acquire(ctx context.Context, deploymentID, resourceID string) (SessionLeaseHandle, error)
}

type SessionLeaseHandle interface {
	Context() context.Context
	Renew(context.Context) error
	Owned(context.Context) error
	Release(context.Context) error
}

// SessionRuntimeAdapter is the verified direct-SDK boundary. The factory must
// derive a fresh workspace mapper from ref on each call; a dispatcher never
// stores a process-global mapper or a physical workspace path.
type SessionRuntimeAdapter interface {
	infrasandbox.SandboxSession
	Prepare(context.Context) error
	Create(context.Context) error
}

type SessionAdapterFactory interface {
	NewSessionAdapter(ref domainsandbox.SessionRef, upstreamShellID, controlShellID string) (SessionRuntimeAdapter, error)
	CleanupShell(ctx context.Context, upstreamShellID string) error
}

type AIOSessionAdapterFactory struct {
	upstream        aio.SessionUpstreamClient
	clock           func() time.Time
	allowedEnvNames []string
}

func NewAIOSessionAdapterFactory(upstream aio.SessionUpstreamClient, clock func() time.Time, allowedEnvNames []string) (*AIOSessionAdapterFactory, error) {
	if upstream == nil || clock == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	// Let the real adapter validate the allowlist when a verified row is used;
	// retain an owned copy so later configuration mutation cannot broaden it.
	return &AIOSessionAdapterFactory{upstream: upstream, clock: clock, allowedEnvNames: append([]string(nil), allowedEnvNames...)}, nil
}

func (factory *AIOSessionAdapterFactory) NewSessionAdapter(ref domainsandbox.SessionRef, shellID, controlShellID string) (SessionRuntimeAdapter, error) {
	if factory == nil || !validSessionOpaqueShellID(shellID) {
		return nil, domainsandbox.ErrInvalidInput
	}
	return aio.NewSessionAdapter(ref, shellID, factory.upstream,
		aio.WithControlShellID(controlShellID), aio.WithSessionClock(factory.clock),
		aio.WithAllowedEnvironmentNames(factory.allowedEnvNames...))
}

func (factory *AIOSessionAdapterFactory) CleanupShell(ctx context.Context, shellID string) error {
	if factory == nil || ctx == nil || !validSessionOpaqueShellID(shellID) {
		return domainsandbox.ErrInvalidInput
	}
	err := factory.upstream.Cleanup(ctx, shellID)
	if aio.ReasonCode(err) == aio.ReasonUpstreamNotFound {
		return nil
	}
	return err
}

type SessionGenerationSource interface {
	GetAIOGeneration(context.Context, string) (domainsandbox.AIOGenerationState, error)
}

type SessionIDSource interface {
	NewSessionID() (string, error)
	NewShellID() (string, error)
}

type SessionDispatcherConfig struct {
	Repository     domainsandbox.RuntimeSessionRepository
	Leases         SessionLeaseManager
	Generation     SessionGenerationSource
	AdapterFactory SessionAdapterFactory
	IDs            SessionIDSource
	Clock          func() time.Time
	SessionTTL     time.Duration
}

type SessionDispatcher struct {
	repository     domainsandbox.RuntimeSessionRepository
	leases         SessionLeaseManager
	generation     SessionGenerationSource
	adapterFactory SessionAdapterFactory
	ids            SessionIDSource
	clock          func() time.Time
	sessionTTL     time.Duration
}

type RandomSessionIDSource struct {
	random io.Reader
	mu     sync.Mutex
}

func NewRandomSessionIDSource(randomSource io.Reader) (*RandomSessionIDSource, error) {
	if randomSource == nil {
		randomSource = rand.Reader
	}
	return &RandomSessionIDSource{random: randomSource}, nil
}

func (source *RandomSessionIDSource) NewSessionID() (string, error) {
	if source == nil || source.random == nil {
		return "", domainsandbox.ErrConfigurationInvalid
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	value, err := uuid.NewRandomFromReader(source.random)
	if err != nil {
		return "", domainsandbox.ErrUnavailable
	}
	return value.String(), nil
}

func (source *RandomSessionIDSource) NewShellID() (string, error) {
	if source == nil || source.random == nil {
		return "", domainsandbox.ErrConfigurationInvalid
	}
	var value [16]byte
	source.mu.Lock()
	_, err := io.ReadFull(source.random, value[:])
	source.mu.Unlock()
	if err != nil {
		return "", domainsandbox.ErrUnavailable
	}
	return "newx-session-" + hex.EncodeToString(value[:]), nil
}

func NewSessionDispatcher(config SessionDispatcherConfig) (*SessionDispatcher, error) {
	if config.Repository == nil || config.Leases == nil || config.Generation == nil ||
		config.AdapterFactory == nil || config.IDs == nil || config.Clock == nil ||
		config.SessionTTL <= 0 || config.SessionTTL > domainsandbox.MaxSessionLifetime {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &SessionDispatcher{
		repository: config.Repository, leases: config.Leases, generation: config.Generation,
		adapterFactory: config.AdapterFactory, ids: config.IDs, clock: config.Clock, sessionTTL: config.SessionTTL,
	}, nil
}

func (dispatcher *SessionDispatcher) Acquire(ctx context.Context, request infrasandbox.AcquireSessionRequest) (session infrasandbox.SandboxSession, resultErr error) {
	request, err := infrasandbox.NormalizeAcquireSessionRequest(request)
	if err != nil || dispatcher == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	lease, err := dispatcher.leases.Acquire(ctx, request.Key.DeploymentID, sessionIdentityResource(request.Key))
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = finishSessionLease(ctx, lease, resultErr)
		if resultErr != nil {
			session = nil
		}
	}()
	workCtx, cancel := linkedSessionLeaseContext(ctx, lease.Context())
	defer cancel()

	generation, err := dispatcher.loadGeneration(workCtx, request.Key.DeploymentID)
	if err != nil {
		return nil, err
	}
	sessionID, err := dispatcher.ids.NewSessionID()
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	now := sessionPersistenceTime(dispatcher.clock())
	row, err := dispatcher.repository.AcquireRuntimeSession(workCtx, domainsandbox.AcquireRuntimeSessionInput{
		Key: request.Key, CandidateSessionID: sessionID, RuntimeGeneration: generation.Generation,
		ExpiresAt: now.Add(dispatcher.sessionTTL), Now: now,
	})
	if err != nil {
		return nil, err
	}
	// Do not trust an Acquire return value as the upstream authorization fact.
	// Reload under the same lease before constructing paths or calling AIO.
	row, err = dispatcher.repository.GetRuntimeSession(workCtx, row.Ref)
	if err != nil {
		return nil, err
	}
	if err := validateAcquiredSessionRow(row, request.Key, generation.Generation); err != nil {
		return nil, err
	}
	if row.UpstreamShellID != "" {
		if err := lease.Owned(workCtx); err != nil {
			return nil, err
		}
		if !row.ExpiresAt.After(sessionPersistenceTime(dispatcher.clock())) {
			if err := dispatcher.cleanupExpiredSession(workCtx, row, lease); err != nil {
				return nil, err
			}
			return nil, domainsandbox.ErrUnavailable
		}
		return dispatcher.boundSession(row, generation.SentinelID)
	}

	candidate, err := dispatcher.ids.NewShellID()
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, err
	}
	adapter, err := dispatcher.adapterFactory.NewSessionAdapter(row.Ref, candidate, generation.SentinelID)
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	if err := adapter.Prepare(workCtx); err != nil {
		return nil, err
	}
	if err := adapter.Create(workCtx); err != nil {
		dispatcher.markRecoveringBestEffort(workCtx, row)
		return nil, err
	}
	if err := lease.Owned(workCtx); err != nil {
		_ = dispatcher.adapterFactory.CleanupShell(context.WithoutCancel(workCtx), candidate)
		return nil, err
	}
	bound, bindErr := dispatcher.repository.BindRuntimeSessionCAS(workCtx, domainsandbox.BindRuntimeSessionInput{
		Ref: row.Ref, ExpectedVersion: row.Version, UpstreamShellID: candidate,
		ExpiresAt: now.Add(dispatcher.sessionTTL), Now: now,
	})
	if bindErr == nil {
		return dispatcher.boundSession(bound, generation.SentinelID)
	}
	if !errors.Is(bindErr, domainsandbox.ErrVersionConflict) {
		// A transport/database error cannot prove whether Bind committed. Keep the
		// candidate: deleting it could destroy the persisted winner.
		dispatcher.markRecoveringBestEffort(workCtx, row)
		return nil, bindErr
	}
	if err := dispatcher.adapterFactory.CleanupShell(context.WithoutCancel(workCtx), candidate); err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	winner, err := dispatcher.repository.GetRuntimeSession(workCtx, row.Ref)
	if err != nil || validateAcquiredSessionRow(winner, request.Key, generation.Generation) != nil || winner.UpstreamShellID == "" || winner.UpstreamShellID == candidate {
		return nil, domainsandbox.ErrUnavailable
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, err
	}
	return dispatcher.boundSession(winner, generation.SentinelID)
}

func (dispatcher *SessionDispatcher) Get(ctx context.Context, ref domainsandbox.SessionRef) (session infrasandbox.SandboxSession, resultErr error) {
	if dispatcher == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref {
		return nil, domainsandbox.ErrInvalidInput
	}
	lease, err := dispatcher.leases.Acquire(ctx, ref.Key.DeploymentID, ref.SessionID)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = finishSessionLease(ctx, lease, resultErr)
		if resultErr != nil {
			session = nil
		}
	}()
	workCtx, cancel := linkedSessionLeaseContext(ctx, lease.Context())
	defer cancel()
	row, generation, err := dispatcher.loadActiveRow(workCtx, ref, lease)
	if err != nil {
		return nil, err
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, err
	}
	return dispatcher.boundSession(row, generation.SentinelID)
}

func (dispatcher *SessionDispatcher) Release(ctx context.Context, ref domainsandbox.SessionRef) error {
	return dispatcher.transitionAndCleanup(ctx, ref, domainsandbox.SessionActionRelease)
}

func (dispatcher *SessionDispatcher) Destroy(ctx context.Context, ref domainsandbox.SessionRef) error {
	return dispatcher.transitionAndCleanup(ctx, ref, domainsandbox.SessionActionDestroy)
}

func (dispatcher *SessionDispatcher) Recover(ctx context.Context, ref domainsandbox.SessionRef) (session infrasandbox.SandboxSession, resultErr error) {
	if dispatcher == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref {
		return nil, domainsandbox.ErrInvalidInput
	}
	lease, err := dispatcher.leases.Acquire(ctx, ref.Key.DeploymentID, ref.SessionID)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = finishSessionLease(ctx, lease, resultErr)
		if resultErr != nil {
			session = nil
		}
	}()
	workCtx, cancel := linkedSessionLeaseContext(ctx, lease.Context())
	defer cancel()
	generation, err := dispatcher.loadGeneration(workCtx, ref.Key.DeploymentID)
	if err != nil {
		return nil, err
	}
	row, err := dispatcher.repository.GetRuntimeSession(workCtx, ref)
	if err != nil {
		return nil, err
	}
	if row.Ref.Key != ref.Key || row.Ref.SessionID != ref.SessionID || row.State != domainsandbox.SessionStateRecovering ||
		(row.UpstreamShellID != "" && row.RecoveryReason != domainsandbox.SessionOperationFenceReason) {
		return nil, domainsandbox.ErrUnavailable
	}
	if row.UpstreamShellID != "" && row.Ref.RuntimeGeneration == generation.Generation {
		if err := lease.Owned(workCtx); err != nil {
			return nil, err
		}
		if err := dispatcher.adapterFactory.CleanupShell(workCtx, row.UpstreamShellID); err != nil {
			return nil, err
		}
		if err := lease.Owned(workCtx); err != nil {
			return nil, err
		}
	}
	candidate, err := dispatcher.ids.NewShellID()
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, err
	}
	if candidate == row.UpstreamShellID {
		return nil, domainsandbox.ErrUnavailable
	}
	nextRef := row.Ref
	nextRef.RuntimeGeneration = generation.Generation
	adapter, err := dispatcher.adapterFactory.NewSessionAdapter(nextRef, candidate, generation.SentinelID)
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	if err := adapter.Prepare(workCtx); err != nil {
		return nil, err
	}
	if err := adapter.Create(workCtx); err != nil {
		return nil, err
	}
	if err := lease.Owned(workCtx); err != nil {
		_ = dispatcher.adapterFactory.CleanupShell(context.WithoutCancel(workCtx), candidate)
		return nil, err
	}
	now := sessionPersistenceTime(dispatcher.clock())
	recovered, err := dispatcher.repository.TransitionRuntimeSessionCAS(workCtx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: row.Ref, ExpectedVersion: row.Version, Action: domainsandbox.SessionActionRecover,
		NextRuntimeGeneration: generation.Generation, UpstreamShellID: candidate,
		ExpiresAt: now.Add(dispatcher.sessionTTL), Now: now,
	})
	if err != nil {
		if errors.Is(err, domainsandbox.ErrVersionConflict) {
			_ = dispatcher.adapterFactory.CleanupShell(context.WithoutCancel(workCtx), candidate)
		}
		return nil, err
	}
	return dispatcher.boundSession(recovered, generation.SentinelID)
}

func (dispatcher *SessionDispatcher) transitionAndCleanup(ctx context.Context, ref domainsandbox.SessionRef, action domainsandbox.SessionAction) (resultErr error) {
	if dispatcher == nil {
		return domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref {
		return domainsandbox.ErrInvalidInput
	}
	lease, err := dispatcher.leases.Acquire(ctx, ref.Key.DeploymentID, ref.SessionID)
	if err != nil {
		return err
	}
	defer func() { resultErr = finishSessionLease(ctx, lease, resultErr) }()
	workCtx, cancel := linkedSessionLeaseContext(ctx, lease.Context())
	defer cancel()
	row, generation, err := dispatcher.loadVerifiedRow(workCtx, ref)
	if err != nil {
		return err
	}
	if err := lease.Owned(workCtx); err != nil {
		return err
	}
	cleanupRow := row
	if row.UpstreamShellID != "" && row.Ref.RuntimeGeneration == generation.Generation {
		cleanupRow, err = dispatcher.persistOperationFence(workCtx, row)
		if err != nil {
			return err
		}
		if err := lease.Owned(workCtx); err != nil {
			return err
		}
		if err := dispatcher.adapterFactory.CleanupShell(workCtx, cleanupRow.UpstreamShellID); err != nil {
			return err
		}
		if err := lease.Owned(workCtx); err != nil {
			return err
		}
	}
	_, err = dispatcher.repository.TransitionRuntimeSessionCAS(workCtx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: cleanupRow.Ref, ExpectedVersion: cleanupRow.Version, Action: action, Now: sessionPersistenceTime(dispatcher.clock()),
	})
	return err
}

func (dispatcher *SessionDispatcher) loadActiveRow(ctx context.Context, ref domainsandbox.SessionRef, lease SessionLeaseHandle) (domainsandbox.RuntimeSession, domainsandbox.AIOGenerationState, error) {
	row, generation, err := dispatcher.loadVerifiedRow(ctx, ref)
	if err != nil {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, err
	}
	if row.Ref.RuntimeGeneration != generation.Generation {
		if row.State == domainsandbox.SessionStateActive || row.State == domainsandbox.SessionStateReleased {
			if err := lease.Owned(ctx); err != nil {
				return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, err
			}
			_, _ = dispatcher.repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
				Ref: row.Ref, ExpectedVersion: row.Version, Action: domainsandbox.SessionActionMarkRecovering,
				RecoveryReason: sessionDispatcherRecoveryReason, Now: dispatcher.clock().UTC(),
			})
		}
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, domainsandbox.ErrUnavailable
	}
	if row.State != domainsandbox.SessionStateActive || row.UpstreamShellID == "" {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, domainsandbox.ErrUnavailable
	}
	if !row.ExpiresAt.After(sessionPersistenceTime(dispatcher.clock())) {
		if err := dispatcher.cleanupExpiredSession(ctx, row, lease); err != nil {
			return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, err
		}
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, domainsandbox.ErrUnavailable
	}
	return row, generation, nil
}

func (dispatcher *SessionDispatcher) cleanupExpiredSession(ctx context.Context, row domainsandbox.RuntimeSession, lease SessionLeaseHandle) error {
	if err := lease.Owned(ctx); err != nil {
		return err
	}
	fenced, err := dispatcher.persistOperationFence(ctx, row)
	if err != nil {
		return err
	}
	if err := dispatcher.adapterFactory.CleanupShell(ctx, fenced.UpstreamShellID); err != nil {
		return err
	}
	if err := lease.Owned(ctx); err != nil {
		return err
	}
	_, err = dispatcher.repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: fenced.Ref, ExpectedVersion: fenced.Version, Action: domainsandbox.SessionActionRelease, Now: sessionPersistenceTime(dispatcher.clock()),
	})
	return err
}

func (dispatcher *SessionDispatcher) persistOperationFence(ctx context.Context, row domainsandbox.RuntimeSession) (domainsandbox.RuntimeSession, error) {
	if row.State == domainsandbox.SessionStateRecovering && row.RecoveryReason == domainsandbox.SessionOperationFenceReason && validSessionOpaqueShellID(row.UpstreamShellID) {
		return row, nil
	}
	if row.State != domainsandbox.SessionStateActive || !validSessionOpaqueShellID(row.UpstreamShellID) {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrUnavailable
	}
	fenced, err := dispatcher.repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: row.Ref, ExpectedVersion: row.Version, Action: domainsandbox.SessionActionBeginOperation,
		UpstreamShellID: row.UpstreamShellID, RecoveryReason: domainsandbox.SessionOperationFenceReason,
		Now: sessionPersistenceTime(dispatcher.clock()),
	})
	if err != nil {
		return domainsandbox.RuntimeSession{}, err
	}
	if fenced.Ref != row.Ref || fenced.State != domainsandbox.SessionStateRecovering ||
		fenced.UpstreamShellID != row.UpstreamShellID || fenced.RecoveryReason != domainsandbox.SessionOperationFenceReason ||
		fenced.Version <= row.Version {
		return domainsandbox.RuntimeSession{}, domainsandbox.ErrUnavailable
	}
	return fenced, nil
}

func (dispatcher *SessionDispatcher) completeOperationFence(ctx context.Context, row domainsandbox.RuntimeSession) error {
	now := sessionPersistenceTime(dispatcher.clock())
	completed, err := dispatcher.repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: row.Ref, ExpectedVersion: row.Version, Action: domainsandbox.SessionActionCompleteOperation,
		UpstreamShellID: row.UpstreamShellID, RecoveryReason: domainsandbox.SessionOperationFenceReason,
		ExpiresAt: now.Add(dispatcher.sessionTTL), Now: now,
	})
	if err != nil {
		return err
	}
	if completed.Ref != row.Ref || completed.State != domainsandbox.SessionStateActive ||
		completed.UpstreamShellID != row.UpstreamShellID || completed.RecoveryReason != "" ||
		completed.Version <= row.Version || completed.LastActivityAt != now || completed.ExpiresAt != now.Add(dispatcher.sessionTTL) {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func (dispatcher *SessionDispatcher) loadVerifiedRow(ctx context.Context, ref domainsandbox.SessionRef) (domainsandbox.RuntimeSession, domainsandbox.AIOGenerationState, error) {
	if dispatcher == nil {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, domainsandbox.ErrInvalidInput
	}
	generation, err := dispatcher.loadGeneration(ctx, ref.Key.DeploymentID)
	if err != nil {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, err
	}
	row, err := dispatcher.repository.GetRuntimeSession(ctx, ref)
	if err != nil {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, err
	}
	if row.Ref != ref || row.Ref.Key != ref.Key {
		return domainsandbox.RuntimeSession{}, domainsandbox.AIOGenerationState{}, domainsandbox.ErrUnavailable
	}
	return row, generation, nil
}

func (dispatcher *SessionDispatcher) loadGeneration(ctx context.Context, deploymentID string) (domainsandbox.AIOGenerationState, error) {
	state, err := dispatcher.generation.GetAIOGeneration(ctx, deploymentID)
	if err != nil {
		return domainsandbox.AIOGenerationState{}, domainsandbox.ErrUnavailable
	}
	normalized, err := domainsandbox.NormalizeAIOGenerationState(state)
	if err != nil || normalized != state || state.DeploymentID != deploymentID || state.Generation == 0 || state.SentinelID == "" {
		return domainsandbox.AIOGenerationState{}, domainsandbox.ErrUnavailable
	}
	return state, nil
}

func (dispatcher *SessionDispatcher) boundSession(row domainsandbox.RuntimeSession, controlShellID string) (infrasandbox.SandboxSession, error) {
	if validateSessionRow(row, row.Ref.Key, row.Ref.RuntimeGeneration, true) != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	return &dispatcherSession{dispatcher: dispatcher, ref: row.Ref, controlShellID: controlShellID}, nil
}

func validateAcquiredSessionRow(row domainsandbox.RuntimeSession, key domainsandbox.SessionKey, generation uint64) error {
	ref, err := domainsandbox.NormalizeSessionRef(row.Ref)
	if err != nil || ref != row.Ref || row.Ref.Key != key || row.Ref.RuntimeGeneration != generation ||
		row.State != domainsandbox.SessionStateActive || row.Version < domainsandbox.InitialVersion {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func validateSessionRow(row domainsandbox.RuntimeSession, key domainsandbox.SessionKey, generation uint64, requireShell bool) error {
	if err := validateAcquiredSessionRow(row, key, generation); err != nil || requireShell && !validSessionOpaqueShellID(row.UpstreamShellID) {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func validSessionOpaqueShellID(value string) bool {
	if value == "" || len(value) > domainsandbox.MaxSessionIdentifierBytes || value == "." || value == ".." ||
		strings.TrimSpace(value) != value || strings.HasPrefix(value, "newx-generation-") {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func (dispatcher *SessionDispatcher) markRecoveringBestEffort(ctx context.Context, row domainsandbox.RuntimeSession) {
	_, _ = dispatcher.repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: row.Ref, ExpectedVersion: row.Version, Action: domainsandbox.SessionActionMarkRecovering,
		RecoveryReason: sessionDispatcherRecoveryReason, Now: dispatcher.clock().UTC(),
	})
}

func sessionIdentityResource(key domainsandbox.SessionKey) string {
	// Length-delimited canonical fields prevent ambiguous concatenation. The
	// digest is an opaque identifier and contains no physical path.
	hash := sha256.New()
	_, _ = io.WriteString(hash, key.DeploymentID)
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, strconv.FormatInt(key.ProviderID, 10))
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, strconv.FormatInt(key.SpaceID, 10))
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, strconv.FormatInt(key.UserID, 10))
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, key.ThreadID)
	_, _ = hash.Write([]byte{0})
	_, _ = io.WriteString(hash, string(key.Profile))
	return "identity-" + hex.EncodeToString(hash.Sum(nil))
}

type dispatcherSession struct {
	dispatcher     *SessionDispatcher
	ref            domainsandbox.SessionRef
	controlShellID string
}

func (session *dispatcherSession) Ref() domainsandbox.SessionRef { return session.ref }

func (session *dispatcherSession) withAdapter(ctx context.Context, call func(context.Context, SessionRuntimeAdapter) (any, error)) (any, error) {
	dispatcher := session.dispatcher
	lease, err := dispatcher.leases.Acquire(ctx, session.ref.Key.DeploymentID, session.ref.SessionID)
	if err != nil {
		return nil, err
	}
	workCtx, cancel := linkedSessionLeaseContext(ctx, lease.Context())
	defer cancel()
	row, generation, err := dispatcher.loadActiveRow(workCtx, session.ref, lease)
	if err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	fenced, err := dispatcher.persistOperationFence(workCtx, row)
	if err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	adapter, err := dispatcher.adapterFactory.NewSessionAdapter(fenced.Ref, fenced.UpstreamShellID, generation.SentinelID)
	if err != nil {
		return nil, finishSessionLease(ctx, lease, domainsandbox.ErrUnavailable)
	}
	result, err := call(workCtx, adapter)
	if err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	if err := lease.Owned(workCtx); err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	if err := dispatcher.completeOperationFence(workCtx, fenced); err != nil {
		return nil, finishSessionLease(ctx, lease, err)
	}
	if err := finishSessionLease(ctx, lease, nil); err != nil {
		return nil, err
	}
	return result, nil
}

func finishSessionLease(ctx context.Context, lease SessionLeaseHandle, operationErr error) error {
	releaseErr := lease.Release(context.WithoutCancel(ctx))
	if releaseErr == nil {
		return operationErr
	}
	if operationErr == nil {
		return releaseErr
	}
	return errors.Join(operationErr, releaseErr)
}

func (session *dispatcherSession) Exec(ctx context.Context, request infrasandbox.ExecRequest) (infrasandbox.ExecutionStream, error) {
	result, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return adapter.Exec(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return result.(infrasandbox.ExecutionStream), nil
}

func (session *dispatcherSession) Read(ctx context.Context, request infrasandbox.ReadRequest) (infrasandbox.FileContent, error) {
	result, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return adapter.Read(ctx, request)
	})
	if err != nil {
		return infrasandbox.FileContent{}, err
	}
	return result.(infrasandbox.FileContent), nil
}

func (session *dispatcherSession) Write(ctx context.Context, request infrasandbox.WriteRequest) error {
	_, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return nil, adapter.Write(ctx, request)
	})
	return err
}

func (session *dispatcherSession) List(ctx context.Context, request infrasandbox.ListRequest) ([]infrasandbox.FileEntry, error) {
	result, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return adapter.List(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return result.([]infrasandbox.FileEntry), nil
}

func (session *dispatcherSession) Glob(ctx context.Context, request infrasandbox.GlobRequest) ([]infrasandbox.FileEntry, error) {
	result, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return adapter.Glob(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return result.([]infrasandbox.FileEntry), nil
}

func (session *dispatcherSession) Grep(ctx context.Context, request infrasandbox.GrepRequest) ([]infrasandbox.GrepMatch, error) {
	result, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return adapter.Grep(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return result.([]infrasandbox.GrepMatch), nil
}

func (session *dispatcherSession) Replace(ctx context.Context, request infrasandbox.ReplaceRequest) error {
	_, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return nil, adapter.Replace(ctx, request)
	})
	return err
}

func (session *dispatcherSession) Download(ctx context.Context, request infrasandbox.DownloadRequest) (io.ReadCloser, error) {
	result, err := session.withAdapter(ctx, func(ctx context.Context, adapter SessionRuntimeAdapter) (any, error) {
		return adapter.Download(ctx, request)
	})
	if err != nil {
		return nil, err
	}
	return result.(io.ReadCloser), nil
}

func linkedSessionLeaseContext(parent, lease context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if lease == nil {
		cancel()
		return ctx, cancel
	}
	stop := context.AfterFunc(lease, cancel)
	return ctx, func() { stop(); cancel() }
}

var _ infrasandbox.SandboxSessionManager = (*SessionDispatcher)(nil)
var _ infrasandbox.SandboxSession = (*dispatcherSession)(nil)

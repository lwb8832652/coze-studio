// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package aio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	lifecycleSentinelPrefix = "newx-generation-"
	lifecycleExecDir        = "/mnt/user-data"
)

var (
	ErrLifecycleConfiguration     = errors.New("invalid AIO lifecycle configuration")
	ErrLifecycleUnknown           = errors.New("AIO lifecycle state is unknown")
	ErrLifecycleCandidateRejected = errors.New("AIO lifecycle candidate was not confirmed")
	ErrLifecycleCandidateCleanup  = errors.New("AIO lifecycle candidate cleanup failed")
)

type LifecycleState string

const (
	LifecycleStateDisabled   LifecycleState = "disabled"
	LifecycleStateUnknown    LifecycleState = "unknown"
	LifecycleStateRecovering LifecycleState = "recovering"
	LifecycleStateReady      LifecycleState = "ready"
)

// LifecycleSnapshot is safe to project through Runner status. In particular,
// it deliberately omits the reserved upstream sentinel identifier.
type LifecycleSnapshot struct {
	Enabled    bool
	Ready      bool
	State      LifecycleState
	Generation uint64
}

func (snapshot LifecycleSnapshot) String() string {
	return fmt.Sprintf("aio.LifecycleSnapshot{Enabled:%t, Ready:%t, State:%q, Generation:%d}", snapshot.Enabled, snapshot.Ready, snapshot.State, snapshot.Generation)
}

func (snapshot LifecycleSnapshot) GoString() string { return snapshot.String() }

type LifecycleUpstream interface {
	Health(context.Context) error
	ListSessions(context.Context) (*sandboxapi.ResponseActiveShellSessionsResult, error)
	Create(context.Context, *sandboxapi.ShellCreateSessionRequest) (*sandboxapi.ResponseShellCreateSessionResponse, error)
	View(context.Context, *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error)
	Cleanup(context.Context, string) error
}

type LifecycleConfig struct {
	Enabled      bool
	DeploymentID string
	Repository   domainsandbox.AIOGenerationRepository
	Upstream     LifecycleUpstream
	Random       io.Reader
}

// LifecycleSupervisor observes an AIO process owned by the deploy layer. It
// has no container lifecycle capability; MySQL is the only generation
// linearization point.
type LifecycleSupervisor struct {
	enabled      bool
	deploymentID string
	repository   domainsandbox.AIOGenerationRepository
	upstream     LifecycleUpstream
	random       io.Reader

	observeMu sync.Mutex
	stateMu   sync.RWMutex
	snapshot  LifecycleSnapshot
}

func NewLifecycleSupervisor(config LifecycleConfig) (*LifecycleSupervisor, error) {
	if !config.Enabled {
		return &LifecycleSupervisor{snapshot: LifecycleSnapshot{State: LifecycleStateDisabled}}, nil
	}
	deploymentID, err := domainsandbox.NormalizeAIOGenerationDeploymentID(config.DeploymentID)
	if err != nil || deploymentID != config.DeploymentID || config.Repository == nil || config.Upstream == nil {
		return nil, ErrLifecycleConfiguration
	}
	randomSource := config.Random
	if randomSource == nil {
		randomSource = rand.Reader
	}
	return &LifecycleSupervisor{
		enabled: true, deploymentID: deploymentID, repository: config.Repository, upstream: config.Upstream,
		random:   randomSource,
		snapshot: LifecycleSnapshot{Enabled: true, State: LifecycleStateUnknown},
	}, nil
}

func (supervisor *LifecycleSupervisor) Snapshot() LifecycleSnapshot {
	if supervisor == nil {
		return LifecycleSnapshot{State: LifecycleStateUnknown}
	}
	supervisor.stateMu.RLock()
	defer supervisor.stateMu.RUnlock()
	return supervisor.snapshot
}

func (supervisor *LifecycleSupervisor) Observe(ctx context.Context) (LifecycleSnapshot, error) {
	if supervisor == nil {
		return LifecycleSnapshot{State: LifecycleStateUnknown}, ErrLifecycleConfiguration
	}
	if !supervisor.enabled {
		return supervisor.Snapshot(), nil
	}
	supervisor.observeMu.Lock()
	defer supervisor.observeMu.Unlock()

	if err := supervisor.upstream.Health(ctx); err != nil {
		return supervisor.publishUnknown(supervisor.Snapshot().Generation), ErrLifecycleUnknown
	}
	persisted, err := supervisor.repository.GetAIOGeneration(ctx, supervisor.deploymentID)
	if err != nil || !supervisor.validGenerationState(persisted) {
		return supervisor.publishUnknown(supervisor.Snapshot().Generation), ErrLifecycleUnknown
	}
	sessions, err := supervisor.listSessions(ctx)
	if err != nil {
		return supervisor.publishUnknown(persisted.Generation), ErrLifecycleUnknown
	}
	if persisted.Generation != 0 {
		present, valid := lifecycleSessionPresence(sessions, persisted.SentinelID)
		if !valid {
			return supervisor.publishUnknown(persisted.Generation), ErrLifecycleUnknown
		}
		if present {
			return supervisor.publish(LifecycleSnapshot{Enabled: true, Ready: true, State: LifecycleStateReady, Generation: persisted.Generation}), nil
		}
	}

	supervisor.publish(LifecycleSnapshot{Enabled: true, State: LifecycleStateRecovering, Generation: persisted.Generation})
	candidate, err := supervisor.newSentinelID()
	if err != nil {
		return supervisor.Snapshot(), ErrLifecycleUnknown
	}
	if _, collision := sessions[candidate]; collision {
		return supervisor.Snapshot(), ErrLifecycleUnknown
	}
	if err := supervisor.createAndVerifyCandidate(ctx, candidate); err != nil {
		if cleanupErr := supervisor.cleanupCandidate(ctx, candidate); cleanupErr != nil {
			return supervisor.Snapshot(), cleanupErr
		}
		return supervisor.Snapshot(), ErrLifecycleCandidateRejected
	}
	next, replaced, err := supervisor.repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID:        supervisor.deploymentID,
		ExpectedSentinelID:  persisted.SentinelID,
		CandidateSentinelID: candidate,
	})
	if err != nil {
		// A database error does not prove whether the transaction committed.
		// Retain the candidate until a later observation establishes ownership;
		// deleting it here could remove the persisted winner.
		return supervisor.publishUnknown(persisted.Generation), ErrLifecycleUnknown
	}
	if replaced {
		if !supervisor.validGenerationState(next) || next.SentinelID != candidate || next.Generation != persisted.Generation+1 {
			// The candidate may already be the database winner. Never clean it on
			// an inconsistent success response.
			return supervisor.publishUnknown(persisted.Generation), ErrLifecycleUnknown
		}
		return supervisor.publish(LifecycleSnapshot{Enabled: true, Ready: true, State: LifecycleStateReady, Generation: next.Generation}), nil
	}
	if next.SentinelID == candidate {
		// A false CAS result is not enough evidence that this process still owns
		// the candidate. Fail closed instead of risking deletion of a winner.
		return supervisor.publishUnknown(persisted.Generation), ErrLifecycleUnknown
	}
	if err := supervisor.cleanupCandidate(ctx, candidate); err != nil {
		return supervisor.Snapshot(), err
	}
	return supervisor.adoptWinner(ctx, persisted.Generation)
}

func (supervisor *LifecycleSupervisor) adoptWinner(ctx context.Context, previousGeneration uint64) (LifecycleSnapshot, error) {
	winner, err := supervisor.repository.GetAIOGeneration(ctx, supervisor.deploymentID)
	if err != nil || !supervisor.validGenerationState(winner) || winner.Generation == 0 || winner.Generation < previousGeneration {
		return supervisor.publishUnknown(previousGeneration), ErrLifecycleUnknown
	}
	sessions, err := supervisor.listSessions(ctx)
	if err != nil {
		return supervisor.publishUnknown(winner.Generation), ErrLifecycleUnknown
	}
	present, valid := lifecycleSessionPresence(sessions, winner.SentinelID)
	if !valid {
		return supervisor.publishUnknown(winner.Generation), ErrLifecycleUnknown
	}
	if !present {
		return supervisor.publish(LifecycleSnapshot{Enabled: true, State: LifecycleStateRecovering, Generation: winner.Generation}), ErrLifecycleUnknown
	}
	return supervisor.publish(LifecycleSnapshot{Enabled: true, Ready: true, State: LifecycleStateReady, Generation: winner.Generation}), nil
}

func (supervisor *LifecycleSupervisor) createAndVerifyCandidate(ctx context.Context, candidate string) error {
	preserveSymlinks := false
	response, err := supervisor.upstream.Create(ctx, &sandboxapi.ShellCreateSessionRequest{
		Id: &candidate, ExecDir: stringPointerAdapter(lifecycleExecDir), PreserveSymlinks: &preserveSymlinks,
	})
	if err != nil || response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		response.Data.SessionId != candidate || response.Data.WorkingDir != lifecycleExecDir {
		return ErrLifecycleCandidateRejected
	}
	view, err := supervisor.upstream.View(ctx, &sandboxapi.ShellViewRequest{Id: candidate})
	if err != nil || view == nil || view.Success == nil || !*view.Success || view.Data == nil || view.Data.SessionId != candidate {
		return ErrLifecycleCandidateRejected
	}
	return nil
}

func (supervisor *LifecycleSupervisor) cleanupCandidate(ctx context.Context, candidate string) error {
	if err := supervisor.upstream.Cleanup(ctx, candidate); err != nil && ReasonCode(err) != ReasonUpstreamNotFound {
		return ErrLifecycleCandidateCleanup
	}
	return nil
}

func (supervisor *LifecycleSupervisor) listSessions(ctx context.Context) (map[string]*sandboxapi.ShellSessionInfo, error) {
	response, err := supervisor.upstream.ListSessions(ctx)
	if err != nil || response == nil || response.Success == nil || !*response.Success || response.Data == nil || response.Data.Sessions == nil {
		return nil, ErrLifecycleUnknown
	}
	return response.Data.Sessions, nil
}

func lifecycleSessionPresence(sessions map[string]*sandboxapi.ShellSessionInfo, sentinelID string) (bool, bool) {
	session, present := sessions[sentinelID]
	if !present {
		return false, true
	}
	if session == nil || session.WorkingDir != lifecycleExecDir {
		return false, false
	}
	return true, true
}

func (supervisor *LifecycleSupervisor) validGenerationState(state domainsandbox.AIOGenerationState) bool {
	normalized, err := domainsandbox.NormalizeAIOGenerationState(state)
	if err != nil || normalized != state {
		return false
	}
	return state.Generation == 0 || state.DeploymentID == supervisor.deploymentID
}

func (supervisor *LifecycleSupervisor) newSentinelID() (string, error) {
	var bytes [16]byte
	if _, err := io.ReadFull(supervisor.random, bytes[:]); err != nil {
		return "", err
	}
	return lifecycleSentinelPrefix + hex.EncodeToString(bytes[:]), nil
}

func (supervisor *LifecycleSupervisor) publishUnknown(generation uint64) LifecycleSnapshot {
	return supervisor.publish(LifecycleSnapshot{Enabled: true, State: LifecycleStateUnknown, Generation: generation})
}

func (supervisor *LifecycleSupervisor) publish(snapshot LifecycleSnapshot) LifecycleSnapshot {
	supervisor.stateMu.Lock()
	supervisor.snapshot = snapshot
	supervisor.stateMu.Unlock()
	return snapshot
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

var ErrLifecycleConflict = errors.New("sandbox lifecycle conflict")

type LifecycleConfig struct {
	Driver       sandboxruntime.Driver
	Settings     domainsandbox.SchedulerSettings
	DeploymentID string
	Now          func() time.Time
}

type LifecycleRequest struct {
	ExecutionID          string
	Scope                domainsandbox.Scope
	Identity             sandboxidentity.Request
	ImageDigest          string
	PolicyVersion        string
	SchedulerVersion     uint64
	CredentialGeneration string
}

type LifecycleLease struct {
	ExecutionID string
	ContainerID string
	Reused      bool
}

type LifecycleSnapshot struct {
	Idle        int
	Active      int
	Quarantined int
}

type Lifecycle struct {
	driver       sandboxruntime.Driver
	settings     domainsandbox.SchedulerSettings
	deploymentID string
	now          func() time.Time

	mu          sync.Mutex
	idle        map[string][]idleContainer
	active      map[string]activeContainer
	quarantined map[string]struct{}
}

type idleContainer struct {
	container sandboxruntime.Container
	reuseKey  string
	expiresAt time.Time
}

type activeContainer struct {
	container sandboxruntime.Container
	reuseKey  string
	ttl       time.Duration
}

func NewLifecycle(config LifecycleConfig) (*Lifecycle, error) {
	settings, err := domainsandbox.NormalizeSchedulerSettings(config.Settings)
	if err != nil || config.Driver == nil || !validKeyID(config.DeploymentID) || config.Now == nil {
		return nil, ErrConfiguration
	}
	return &Lifecycle{driver: config.Driver, settings: settings, deploymentID: config.DeploymentID, now: config.Now, idle: map[string][]idleContainer{}, active: map[string]activeContainer{}, quarantined: map[string]struct{}{}}, nil
}

func (lifecycle *Lifecycle) ReuseKey(request LifecycleRequest) (string, time.Duration, error) {
	if lifecycle == nil || !validLifecycleRequest(request) {
		return "", 0, ErrProtocol
	}
	workload, ok := lifecycle.settings.Workloads[request.Scope]
	if !ok {
		return "", 0, ErrProtocol
	}
	var key string
	switch request.Scope {
	case domainsandbox.ScopeAgent:
		key = "agent:" + strconv.FormatInt(request.Identity.SpaceID, 10) + ":" + strconv.FormatInt(request.Identity.UserID, 10)
	case domainsandbox.ScopeAppDev:
		if request.Identity.ProjectID == "" {
			return "", 0, ErrProtocol
		}
		key = "appdev:" + strconv.FormatInt(request.Identity.SpaceID, 10) + ":" + request.Identity.ProjectID
	case domainsandbox.ScopeMCPStdio:
		if request.Identity.SessionID == "" {
			return "", 0, ErrProtocol
		}
		key = "mcp_stdio:" + strconv.FormatInt(request.Identity.SpaceID, 10) + ":" + request.Identity.SessionID
	case domainsandbox.ScopePlugin:
		key = "plugin:" + strconv.FormatInt(request.Identity.SpaceID, 10) + ":" + request.ExecutionID
	default:
		return "", 0, ErrProtocol
	}
	return key, time.Duration(workload.IdleTTLSeconds) * time.Second, nil
}

func (lifecycle *Lifecycle) Acquire(ctx context.Context, request LifecycleRequest) (LifecycleLease, error) {
	if lifecycle == nil || ctx == nil {
		return LifecycleLease{}, ErrProtocol
	}
	key, ttl, err := lifecycle.ReuseKey(request)
	if err != nil {
		return LifecycleLease{}, err
	}
	lifecycle.mu.Lock()
	expired := lifecycle.expireIdleLocked()
	keyHash := lifecycleHash(key)
	var stale []sandboxruntime.Container
	if ttl > 0 {
		var candidate *idleContainer
		candidate, stale = lifecycle.takeIdleLocked(keyHash)
		if candidate != nil {
			lifecycle.mu.Unlock()
			lifecycle.destroyStale(ctx, expired)
			lifecycle.destroyStale(ctx, stale)
			if !lifecycle.matches(candidate.container.Spec, keyHash, request) || lifecycle.driver.PrepareForReuse(ctx, candidate.container.ID) != nil || lifecycle.driver.Health(ctx, candidate.container.ID) != nil {
				lifecycle.destroyAndQuarantine(ctx, candidate.container.ID)
				return lifecycle.Acquire(ctx, request)
			}
			lifecycle.mu.Lock()
			lifecycle.active[request.ExecutionID] = activeContainer{container: candidate.container, reuseKey: keyHash, ttl: ttl}
			lifecycle.mu.Unlock()
			return LifecycleLease{ExecutionID: request.ExecutionID, ContainerID: candidate.container.ID, Reused: true}, nil
		}
	}
	lifecycle.mu.Unlock()
	lifecycle.destroyStale(ctx, expired)
	lifecycle.destroyStale(ctx, stale)
	container, err := lifecycle.driver.Create(ctx, lifecycle.specification(key, request))
	if err != nil || container.ID == "" {
		return LifecycleLease{}, ErrUnavailable
	}
	lifecycle.mu.Lock()
	lifecycle.active[request.ExecutionID] = activeContainer{container: container, reuseKey: keyHash, ttl: ttl}
	lifecycle.mu.Unlock()
	return LifecycleLease{ExecutionID: request.ExecutionID, ContainerID: container.ID}, nil
}

func (lifecycle *Lifecycle) Release(ctx context.Context, executionID string) error {
	if lifecycle == nil || ctx == nil || !validExecutionID(executionID) {
		return ErrProtocol
	}
	lifecycle.mu.Lock()
	active, ok := lifecycle.active[executionID]
	if ok {
		delete(lifecycle.active, executionID)
	}
	lifecycle.mu.Unlock()
	if !ok {
		return ErrLifecycleConflict
	}
	if active.ttl == 0 {
		if err := lifecycle.driver.Destroy(ctx, active.container.ID); err != nil {
			return lifecycle.quarantine(active.container.ID)
		}
		return nil
	}
	lifecycle.mu.Lock()
	lifecycle.idle[active.reuseKey] = append(lifecycle.idle[active.reuseKey], idleContainer{container: active.container, reuseKey: active.reuseKey, expiresAt: lifecycle.now().Add(active.ttl)})
	lifecycle.mu.Unlock()
	return nil
}

func (lifecycle *Lifecycle) Cancel(ctx context.Context, executionID string) error {
	if lifecycle == nil || ctx == nil || !validExecutionID(executionID) {
		return ErrProtocol
	}
	lifecycle.mu.Lock()
	active, ok := lifecycle.active[executionID]
	lifecycle.mu.Unlock()
	if !ok {
		return ErrLifecycleConflict
	}
	if err := lifecycle.driver.Terminate(ctx, active.container.ID); err != nil {
		return lifecycle.quarantine(active.container.ID)
	}
	stopped, err := lifecycle.driver.WaitStopped(ctx, active.container.ID, time.Duration(lifecycle.settings.CancelGraceSeconds)*time.Second)
	if err != nil {
		return lifecycle.quarantine(active.container.ID)
	}
	if !stopped {
		if err := lifecycle.driver.ForceKill(ctx, active.container.ID); err != nil {
			return lifecycle.quarantine(active.container.ID)
		}
	}
	return lifecycle.Release(ctx, executionID)
}

func (lifecycle *Lifecycle) Recover(ctx context.Context) error {
	if lifecycle == nil || ctx == nil {
		return ErrProtocol
	}
	containers, err := lifecycle.driver.List(ctx)
	if err != nil {
		return ErrUnavailable
	}
	for _, container := range containers {
		if container.ID == "" || container.Spec.DeploymentID != lifecycle.deploymentID || container.Spec.ReuseKeyHash == "" || container.State != sandboxruntime.ContainerStateIdle || !lifecycle.validContainerSpec(container.Spec) {
			if container.ID != "" {
				if err := lifecycle.driver.Destroy(ctx, container.ID); err != nil {
					_ = lifecycle.quarantine(container.ID)
				}
			}
			continue
		}
		lifecycle.mu.Lock()
		lifecycle.idle[container.Spec.ReuseKeyHash] = append(lifecycle.idle[container.Spec.ReuseKeyHash], idleContainer{container: container, reuseKey: container.Spec.ReuseKeyHash, expiresAt: lifecycle.now().Add(lifecycle.idleTTLForScope(container.Spec.Scope))})
		lifecycle.mu.Unlock()
	}
	return nil
}

func (lifecycle *Lifecycle) Snapshot() LifecycleSnapshot {
	if lifecycle == nil {
		return LifecycleSnapshot{}
	}
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	idle := 0
	for _, containers := range lifecycle.idle {
		idle += len(containers)
	}
	return LifecycleSnapshot{Idle: idle, Active: len(lifecycle.active), Quarantined: len(lifecycle.quarantined)}
}

func (lifecycle *Lifecycle) takeIdleLocked(key string) (*idleContainer, []sandboxruntime.Container) {
	containers := lifecycle.idle[key]
	stale := make([]sandboxruntime.Container, 0)
	for len(containers) > 0 {
		candidate := containers[0]
		containers = containers[1:]
		lifecycle.idle[key] = containers
		if !candidate.expiresAt.After(lifecycle.now()) {
			stale = append(stale, candidate.container)
			continue
		}
		return &candidate, stale
	}
	return nil, stale
}

func (lifecycle *Lifecycle) expireIdleLocked() []sandboxruntime.Container {
	expired := make([]sandboxruntime.Container, 0)
	for key, containers := range lifecycle.idle {
		kept := containers[:0]
		for _, candidate := range containers {
			if candidate.expiresAt.After(lifecycle.now()) {
				kept = append(kept, candidate)
				continue
			}
			expired = append(expired, candidate.container)
		}
		lifecycle.idle[key] = kept
	}
	return expired
}

func (lifecycle *Lifecycle) destroyStale(ctx context.Context, containers []sandboxruntime.Container) {
	for _, container := range containers {
		lifecycle.destroyAndQuarantine(ctx, container.ID)
	}
}

func (lifecycle *Lifecycle) destroyAndQuarantine(ctx context.Context, containerID string) {
	_ = lifecycle.driver.Destroy(ctx, containerID)
	_ = lifecycle.quarantine(containerID)
}

func (lifecycle *Lifecycle) specification(key string, request LifecycleRequest) sandboxruntime.Specification {
	return sandboxruntime.Specification{ReuseKeyHash: lifecycleHash(key), Scope: string(request.Scope), ImageDigest: request.ImageDigest, PolicyVersion: request.PolicyVersion, SchedulerVersion: request.SchedulerVersion, CredentialGeneration: request.CredentialGeneration, DeploymentID: lifecycle.deploymentID}
}

func (lifecycle *Lifecycle) matches(specification sandboxruntime.Specification, key string, request LifecycleRequest) bool {
	return specification.ReuseKeyHash == key && specification.Scope == string(request.Scope) && specification.ImageDigest == request.ImageDigest &&
		specification.PolicyVersion == request.PolicyVersion && specification.SchedulerVersion == request.SchedulerVersion &&
		specification.CredentialGeneration == request.CredentialGeneration && specification.DeploymentID == lifecycle.deploymentID
}

func (lifecycle *Lifecycle) validContainerSpec(specification sandboxruntime.Specification) bool {
	return validKeyID(specification.ReuseKeyHash) && validIdentifier(specification.Scope) && validDigestImage(specification.ImageDigest) && validKeyID(specification.PolicyVersion) && specification.SchedulerVersion > 0 && validKeyID(specification.CredentialGeneration)
}

func (lifecycle *Lifecycle) idleTTLForScope(scope string) time.Duration {
	workload, ok := lifecycle.settings.Workloads[domainsandbox.Scope(scope)]
	if !ok {
		return 0
	}
	return time.Duration(workload.IdleTTLSeconds) * time.Second
}

func (lifecycle *Lifecycle) quarantine(containerID string) error {
	if containerID == "" {
		return ErrUnavailable
	}
	lifecycle.mu.Lock()
	lifecycle.quarantineLocked(containerID)
	lifecycle.mu.Unlock()
	return ErrUnavailable
}

func (lifecycle *Lifecycle) quarantineLocked(containerID string) {
	lifecycle.quarantined[containerID] = struct{}{}
}

func lifecycleHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validLifecycleRequest(request LifecycleRequest) bool {
	return validExecutionID(request.ExecutionID) && request.Identity.SpaceID > 0 && request.Identity.UserID > 0 && request.Identity.ExecutionID == request.ExecutionID && validDigestImage(request.ImageDigest) && validKeyID(request.PolicyVersion) && request.SchedulerVersion > 0 && validKeyID(request.CredentialGeneration)
}

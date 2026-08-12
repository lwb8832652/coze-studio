// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// Package runtime defines the narrow, rootless-runtime boundary used by the
// Sandbox Runner. Implementations must not expose host command execution.
package runtime

import (
	"context"
	"time"
)

type ContainerState string

const (
	ContainerStateRunning ContainerState = "running"
	ContainerStateIdle    ContainerState = "idle"
	ContainerStateStopped ContainerState = "stopped"
)

// Specification contains only reviewed runtime labels. ReuseKeyHash is
// intentionally opaque; callers must never put raw tenant identifiers here.
type Specification struct {
	ReuseKeyHash         string
	Scope                string
	ImageDigest          string
	PolicyVersion        string
	SchedulerVersion     uint64
	CredentialGeneration string
	DeploymentID         string
	CPUMilli             int
	MemoryLimitMB        int
	PIDLimit             int
	AllowNetwork         bool
}

type Container struct {
	ID    string
	Spec  Specification
	State ContainerState
}

// Driver has exactly the lifecycle operations the Runner needs. It omits any
// arbitrary exec, mount, socket, or host-shell capability by design.
type Driver interface {
	Create(context.Context, Specification) (Container, error)
	PrepareForReuse(context.Context, string) error
	Health(context.Context, string) error
	Terminate(context.Context, string) error
	WaitStopped(context.Context, string, time.Duration) (bool, error)
	ForceKill(context.Context, string) error
	Destroy(context.Context, string) error
	List(context.Context) ([]Container, error)
}

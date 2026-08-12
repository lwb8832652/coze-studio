// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"time"

	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	ExecutionStateAccepted infrasandbox.ExecutionStatus = infrasandbox.ExecutionStatusAccepted
	ExecutionStateRunning  infrasandbox.ExecutionStatus = infrasandbox.ExecutionStatusRunning
)

type StoredExecution struct {
	ExecutionID   string
	RequestDigest string
	Scope         string
	WorkloadKind  string
	Deadline      time.Time
	State         infrasandbox.ExecutionStatus
	AcceptedAt    time.Time
	UpdatedAt     time.Time
}

// ExecutionStore is the durable Runner execution state boundary. Implementors
// must never expose request envelopes or signed identity outside trusted code.
type ExecutionStore interface {
	Accept(context.Context, ExecuteCommand) (StoredExecution, bool, error)
	Get(context.Context, string) (StoredExecution, error)
	Transition(context.Context, string, infrasandbox.ExecutionStatus) (StoredExecution, error)
	Recover(context.Context) ([]StoredExecution, error)
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
)

// WorkloadExecutor is deliberately the smallest capability required by the
// dispatcher. Lifecycle owns container IDs and reuse; callers only submit a
// fixed adapter plus the already authenticated canonical request.
type WorkloadExecutor interface {
	Execute(context.Context, LifecycleRequest, sandboxruntime.Adapter, []byte, int64) (sandboxruntime.AdapterResult, error)
}

// ResultFinisher releases scheduler capacity only after it has atomically
// recorded a terminal result.
type ResultFinisher interface {
	FinishResult(context.Context, infrasandbox.ExecuteResult) error
}

type WorkloadDispatcherConfig struct {
	Executor             WorkloadExecutor
	ExecutionImage       string
	CredentialGeneration string
	Finisher             ResultFinisher
	Now                  func() time.Time
}

type WorkloadDispatcher struct {
	executor             WorkloadExecutor
	executionImage       string
	credentialGeneration string
	finisher             ResultFinisher
	now                  func() time.Time
}

func NewWorkloadDispatcher(config WorkloadDispatcherConfig) (*WorkloadDispatcher, error) {
	if config.Executor == nil || config.Finisher == nil || !validDigestImage(config.ExecutionImage) || !validKeyID(config.CredentialGeneration) {
		return nil, ErrConfiguration
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	return &WorkloadDispatcher{executor: config.Executor, executionImage: config.ExecutionImage, credentialGeneration: config.CredentialGeneration, finisher: config.Finisher, now: config.Now}, nil
}

// Dispatch is intentionally asynchronous: the scheduler has already marked
// the execution running and must remain available to accept/status other work.
func (dispatcher *WorkloadDispatcher) Dispatch(_ context.Context, execution ScheduledExecution) error {
	if dispatcher == nil || !validScheduledExecution(execution) {
		return ErrProtocol
	}
	go dispatcher.execute(execution)
	return nil
}

func (dispatcher *WorkloadDispatcher) execute(execution ScheduledExecution) {
	command := execution.Command
	adapter, ok := adapterForCommand(command)
	if !ok || command.Policy.AllowNetwork {
		_ = dispatcher.finisher.FinishResult(context.Background(), infrasandbox.ExecuteResult{ExecutionID: execution.ExecutionID, Status: infrasandbox.ExecutionStatusFailed})
		return
	}
	deadline := command.Deadline
	timeoutDeadline := dispatcher.now().Add(time.Duration(command.Policy.TimeoutSeconds) * time.Second)
	if timeoutDeadline.Before(deadline) {
		deadline = timeoutDeadline
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	result, err := dispatcher.executor.Execute(ctx, LifecycleRequest{
		ExecutionID: execution.ExecutionID, Scope: command.Scope, Identity: command.Identity,
		ImageDigest: dispatcher.executionImage, PolicyVersion: policyVersion(command), SchedulerVersion: execution.ConfigurationVersion,
		CredentialGeneration: dispatcher.credentialGeneration, AllowNetwork: false,
	}, adapter, command.RawBody, command.Policy.MaxOutputBytes)
	finished := infrasandbox.ExecuteResult{ExecutionID: execution.ExecutionID, Status: infrasandbox.ExecutionStatusFailed}
	if err == nil {
		exitCode := result.ExitCode
		finished.ExitCode, finished.Stdout, finished.Stderr = &exitCode, string(result.Stdout), string(result.Stderr)
		if result.ExitCode == 0 {
			finished.Status = infrasandbox.ExecutionStatusSucceeded
		}
	} else if ctx.Err() != nil {
		finished.Status = infrasandbox.ExecutionStatusTimedOut
	}
	_ = dispatcher.finisher.FinishResult(context.Background(), finished)
}

func adapterForCommand(command ExecuteCommand) (sandboxruntime.Adapter, bool) {
	switch command.Scope {
	case domainsandbox.ScopeAgent:
		return sandboxruntime.AdapterAgentCode, command.Entrypoint == "agent/code/run"
	case domainsandbox.ScopePlugin:
		return sandboxruntime.AdapterPluginCode, command.Entrypoint == infrasandbox.PluginCodeRunnerEntrypoint
	default:
		return "", false
	}
}

func validScheduledExecution(execution ScheduledExecution) bool {
	command := execution.Command
	// The Runner creates execution.ExecutionID only after the control plane has
	// signed the business identity. They are intentionally distinct identifiers:
	// the encrypted stored execution and scheduled item bind them together.
	return validExecutionID(execution.ExecutionID) && execution.ConfigurationVersion > 0 && validExecutionID(command.Identity.ExecutionID) &&
		command.Identity.SpaceID > 0 && command.Identity.UserID > 0 && !command.Deadline.IsZero() && command.Policy.TimeoutSeconds > 0 &&
		command.Policy.MaxOutputBytes > 0 && len(command.RawBody) > 0 && len(command.RawBody) <= maxRequestBytes
}

func policyVersion(command ExecuteCommand) string {
	// Container reuse must never cross a changed runtime policy. Hash the policy
	// alone: canonical request bodies can contain user input and must not become
	// a Docker label or a reuse-generation value.
	encoded, _ := json.Marshal(command.Policy)
	digest := sha256.Sum256(encoded)
	return "policy-" + hex.EncodeToString(digest[:])
}

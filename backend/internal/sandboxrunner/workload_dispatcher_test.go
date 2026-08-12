// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
)

func TestWorkloadDispatcherExecutesOnlyFixedAdapterAndFinalizesResult(t *testing.T) {
	executor := &recordingWorkloadExecutor{result: sandboxruntime.AdapterResult{ExitCode: 0, Stdout: []byte("safe result")}}
	finisher := &recordingResultFinisher{results: make(chan infrasandbox.ExecuteResult, 1)}
	dispatcher, err := NewWorkloadDispatcher(WorkloadDispatcherConfig{
		Executor: executor, ExecutionImage: "registry.example/newx/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CredentialGeneration: "credential-v1", Finisher: finisher,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := schedulerCommand(time.Now(), "dispatch-agent", 11, 12, domainsandbox.ScopeAgent)
	command.Entrypoint = "agent/code/run"
	command.Policy.TimeoutSeconds = 30
	command.Policy.MaxOutputBytes = 4096
	command.RawBody = []byte(`{"canonical":true}`)
	command.Identity.ExecutionID = "exec-dispatch-agent"
	if err := dispatcher.Dispatch(context.Background(), ScheduledExecution{ExecutionID: "exec-dispatch-agent", Command: command, ConfigurationVersion: 7}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-finisher.results:
		if result.ExecutionID != "exec-dispatch-agent" || result.Status != infrasandbox.ExecutionStatusSucceeded || result.ExitCode == nil || *result.ExitCode != 0 || result.Stdout != "safe result" {
			t.Fatalf("result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not finish execution")
	}
	if executor.adapter != sandboxruntime.AdapterAgentCode || executor.request.AllowNetwork || executor.request.SchedulerVersion != 7 || executor.maxOutput != 4096 {
		t.Fatalf("executor call = %#v, adapter=%q, max=%d", executor.request, executor.adapter, executor.maxOutput)
	}
}

func TestWorkloadDispatcherKeepsSignedBusinessExecutionIDDistinctFromRunnerID(t *testing.T) {
	executor := &recordingWorkloadExecutor{result: sandboxruntime.AdapterResult{ExitCode: 0}}
	finisher := &recordingResultFinisher{results: make(chan infrasandbox.ExecuteResult, 1)}
	dispatcher, err := NewWorkloadDispatcher(WorkloadDispatcherConfig{
		Executor: executor, ExecutionImage: "registry.example/newx/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CredentialGeneration: "credential-v1", Finisher: finisher,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := schedulerCommand(time.Now(), "dispatch-distinct-ids", 11, 12, domainsandbox.ScopeAgent)
	command.Entrypoint = "agent/code/run"
	command.Policy.TimeoutSeconds, command.Policy.MaxOutputBytes = 30, 4096
	command.RawBody = []byte(`{"canonical":true}`)
	command.Identity.ExecutionID = "signed-business-execution"
	execution := ScheduledExecution{ExecutionID: "exec-runner-internal", Command: command, ConfigurationVersion: 7}
	if err := dispatcher.Dispatch(context.Background(), execution); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	select {
	case result := <-finisher.results:
		if result.ExecutionID != execution.ExecutionID || executor.request.ExecutionID != execution.ExecutionID || executor.request.Identity.ExecutionID != command.Identity.ExecutionID {
			t.Fatalf("result/request = %#v/%#v", result, executor.request)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not execute the bounded dual-ID workload")
	}
}

func TestWorkloadDispatcherFailsClosedForNetworkRequestWithoutEnforcer(t *testing.T) {
	executor := &recordingWorkloadExecutor{}
	finisher := &recordingResultFinisher{results: make(chan infrasandbox.ExecuteResult, 1)}
	dispatcher, err := NewWorkloadDispatcher(WorkloadDispatcherConfig{
		Executor: executor, ExecutionImage: "registry.example/newx/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CredentialGeneration: "credential-v1", Finisher: finisher,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := schedulerCommand(time.Now(), "dispatch-network", 11, 12, domainsandbox.ScopePlugin)
	command.Entrypoint = infrasandbox.PluginCodeRunnerEntrypoint
	command.Policy.AllowNetwork = true
	command.Policy.NetworkAllowlist = []string{"example.com"}
	command.Policy.TimeoutSeconds = 30
	command.Policy.MaxOutputBytes = 4096
	command.RawBody = []byte(`{"canonical":true}`)
	command.Identity.ExecutionID = "exec-dispatch-network"
	if err := dispatcher.Dispatch(context.Background(), ScheduledExecution{ExecutionID: "exec-dispatch-network", Command: command, ConfigurationVersion: 7}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-finisher.results:
		if result.Status != infrasandbox.ExecutionStatusFailed || result.ExitCode != nil || executor.calls != 0 {
			t.Fatalf("result/executor = %#v/%#v", result, executor)
		}
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not reject network execution")
	}
}

func TestWorkloadDispatcherMapsOnlyAdaptersPresentInRuntimeImage(t *testing.T) {
	for _, test := range []struct {
		name       string
		scope      domainsandbox.Scope
		entrypoint string
		adapter    sandboxruntime.Adapter
	}{
		{name: "agent", scope: domainsandbox.ScopeAgent, entrypoint: "agent/code/run", adapter: sandboxruntime.AdapterAgentCode},
		{name: "plugin", scope: domainsandbox.ScopePlugin, entrypoint: infrasandbox.PluginCodeRunnerEntrypoint, adapter: sandboxruntime.AdapterPluginCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := schedulerCommand(time.Now(), "dispatch-"+test.name, 11, 12, test.scope)
			command.Entrypoint = test.entrypoint
			adapter, ok := adapterForCommand(command)
			if !ok || adapter != test.adapter {
				t.Fatalf("adapterForCommand() = %q, %t", adapter, ok)
			}
			command.Entrypoint = "bin/sh"
			if _, ok := adapterForCommand(command); ok {
				t.Fatal("unreviewed command was accepted")
			}
		})
	}
	for _, test := range []struct {
		scope      domainsandbox.Scope
		entrypoint string
	}{
		{scope: domainsandbox.ScopeMCPStdio, entrypoint: "mcp/stdio/invoke"},
		{scope: domainsandbox.ScopeAppDev, entrypoint: "appdev/runtime"},
	} {
		command := schedulerCommand(time.Now(), "unsupported-"+string(test.scope), 11, 12, test.scope)
		command.Entrypoint = test.entrypoint
		if adapter, ok := adapterForCommand(command); ok {
			t.Fatalf("unavailable scope %q mapped to adapter %q", test.scope, adapter)
		}
	}
}

func TestPolicyVersionChangesWithPolicyWithoutLeakingRequestPayload(t *testing.T) {
	command := schedulerCommand(time.Now(), "policy-version", 11, 12, domainsandbox.ScopePlugin)
	command.Policy.TimeoutSeconds, command.Policy.MaxOutputBytes = 30, 4096
	command.RawBody = []byte(`{"contains":"do-not-use-as-policy-version"}`)
	first := policyVersion(command)
	command.Policy.TimeoutSeconds = 31
	second := policyVersion(command)
	if first == second || !strings.HasPrefix(first, "policy-") || strings.Contains(first, "do-not-use") {
		t.Fatalf("policy versions = %q/%q", first, second)
	}
}

type recordingWorkloadExecutor struct {
	calls     int
	request   LifecycleRequest
	adapter   sandboxruntime.Adapter
	input     []byte
	maxOutput int64
	result    sandboxruntime.AdapterResult
	err       error
}

func (executor *recordingWorkloadExecutor) Execute(_ context.Context, request LifecycleRequest, adapter sandboxruntime.Adapter, input []byte, maxOutput int64) (sandboxruntime.AdapterResult, error) {
	executor.calls++
	executor.request, executor.adapter, executor.input, executor.maxOutput = request, adapter, append([]byte(nil), input...), maxOutput
	return executor.result, executor.err
}

type recordingResultFinisher struct {
	results chan infrasandbox.ExecuteResult
}

func (finisher *recordingResultFinisher) FinishResult(_ context.Context, result infrasandbox.ExecuteResult) error {
	finisher.results <- result
	return nil
}

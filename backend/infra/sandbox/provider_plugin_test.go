// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestNormalizeExecuteRequestAcceptsPluginScopeAndWorkload(t *testing.T) {
	request := validExecuteRequest()
	request.Scope = domainsandbox.ScopePlugin
	request.WorkloadKind = WorkloadPlugin
	request.Entrypoint = PluginCodeRunnerEntrypoint

	normalized, err := normalizeExecuteRequest(request, time.Now())
	if err != nil {
		t.Fatalf("normalizeExecuteRequest() error = %v", err)
	}
	if normalized.Scope != domainsandbox.ScopePlugin ||
		normalized.WorkloadKind != WorkloadPlugin ||
		normalized.Entrypoint != PluginCodeRunnerEntrypoint {
		t.Fatalf("normalized request identity = %#v", normalized)
	}
}

func TestNormalizeExecuteRequestRejectsMismatchedPluginScopeAndWorkload(t *testing.T) {
	tests := []struct {
		name     string
		scope    domainsandbox.Scope
		workload WorkloadKind
	}{
		{
			name:     "plugin scope with agent workload",
			scope:    domainsandbox.ScopePlugin,
			workload: WorkloadAgent,
		},
		{
			name:     "agent scope with plugin workload",
			scope:    domainsandbox.ScopeAgent,
			workload: WorkloadPlugin,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validExecuteRequest()
			request.Scope = test.scope
			request.WorkloadKind = test.workload
			request.Entrypoint = "plugin/code/run"

			if _, err := normalizeExecuteRequest(request, time.Now()); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("normalizeExecuteRequest() error = %v, want %v", err, domainsandbox.ErrInvalidInput)
			}
		})
	}
}

func TestNormalizeExecuteRequestRejectsNonCanonicalPluginEntrypoint(t *testing.T) {
	for _, entrypoint := range []string{"agent/code/run", "plugin/code/debug", "plugin/run"} {
		t.Run(entrypoint, func(t *testing.T) {
			request := validExecuteRequest()
			request.Scope = domainsandbox.ScopePlugin
			request.WorkloadKind = WorkloadPlugin
			request.Entrypoint = entrypoint

			if _, err := normalizeExecuteRequest(request, time.Now()); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("normalizeExecuteRequest() error = %v, want %v", err, domainsandbox.ErrInvalidInput)
			}
		})
	}
}

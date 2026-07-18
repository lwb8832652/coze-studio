// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSandboxRuntimeAuditPersistsOnlyBoundedCorrelationMetadata(t *testing.T) {
	repository := &sandboxRuntimeAuditRepositoryFake{}
	recorder := NewProviderRuntimeAuditRecorder(
		repository,
		func(context.Context) int64 { return 840582614 },
	)
	rawCorrelation := "execution-secret-user-840582614-provider-request"

	recorder.RecordSandboxRuntimeAudit(context.Background(), SandboxRuntimeAuditObservation{
		ProviderID: 17, Scope: domainsandbox.ScopeAgent,
		Outcome: "success", CorrelationID: rawCorrelation,
	})

	if len(repository.inputs) != 1 {
		t.Fatalf("audit writes = %d", len(repository.inputs))
	}
	input := repository.inputs[0]
	if input.ProviderID != 17 || input.ActorUserID != 840582614 ||
		input.Action != sandboxRuntimeAuditActionExecute || input.Result != "success" {
		t.Fatalf("audit input = %#v", input)
	}
	if !strings.HasPrefix(input.RequestID, "corr_") || len(input.RequestID) != 29 {
		t.Fatalf("correlation ref = %q", input.RequestID)
	}
	if strings.Contains(input.RequestID, rawCorrelation) ||
		strings.Contains(input.RequestID, "840582614") ||
		strings.Contains(input.RequestID, "execution-secret") {
		t.Fatalf("raw correlation leaked: %q", input.RequestID)
	}
	if len(input.Metadata) != 1 || input.Metadata[domainsandbox.AuditMetadataKeyScope] != string(domainsandbox.ScopeAgent) {
		t.Fatalf("audit metadata = %#v", input.Metadata)
	}
}

func TestSandboxRuntimeAuditSkipsMissingAuthenticatedActor(t *testing.T) {
	repository := &sandboxRuntimeAuditRepositoryFake{}
	recorder := NewProviderRuntimeAuditRecorder(
		repository,
		func(context.Context) int64 { return 0 },
	)

	recorder.RecordSandboxRuntimeAudit(context.Background(), SandboxRuntimeAuditObservation{
		ProviderID: 17, Scope: domainsandbox.ScopeAgent,
		Outcome: "success", CorrelationID: "request-1",
	})

	if len(repository.inputs) != 0 {
		t.Fatalf("unauthenticated audit writes = %d", len(repository.inputs))
	}
}

type sandboxRuntimeAuditRepositoryFake struct {
	inputs []domainsandbox.AppendProviderAuditEventInput
}

func (f *sandboxRuntimeAuditRepositoryFake) AppendProviderAuditEvent(
	_ context.Context,
	input domainsandbox.AppendProviderAuditEventInput,
) (*domainsandbox.ProviderAuditEvent, error) {
	f.inputs = append(f.inputs, input)
	return &domainsandbox.ProviderAuditEvent{
		ID: 1, ProviderID: input.ProviderID, ActorUserID: input.ActorUserID,
		Action: input.Action, Result: input.Result, RequestID: input.RequestID,
		Metadata: input.Metadata,
	}, nil
}

func (*sandboxRuntimeAuditRepositoryFake) ListProviderAuditEvents(
	context.Context,
	domainsandbox.ProviderAuditListRequest,
) ([]*domainsandbox.ProviderAuditEvent, int64, error) {
	return nil, 0, errors.New("not implemented")
}

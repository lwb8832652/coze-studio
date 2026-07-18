// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const sandboxRuntimeAuditActionExecute = "runtime.execute"

type SandboxRuntimeAuditObservation struct {
	ProviderID    int64
	Scope         domainsandbox.Scope
	Outcome       string
	CorrelationID string
}

type SandboxRuntimeAuditRecorder interface {
	RecordSandboxRuntimeAudit(context.Context, SandboxRuntimeAuditObservation)
}

type SandboxRuntimeAuditActorResolver func(context.Context) int64

type ProviderRuntimeAuditRecorder struct {
	repository    domainsandbox.ProviderAuditRepository
	actorResolver SandboxRuntimeAuditActorResolver
}

func NewProviderRuntimeAuditRecorder(
	repository domainsandbox.ProviderAuditRepository,
	actorResolver SandboxRuntimeAuditActorResolver,
) *ProviderRuntimeAuditRecorder {
	if repository == nil || actorResolver == nil {
		return nil
	}
	return &ProviderRuntimeAuditRecorder{
		repository:    repository,
		actorResolver: actorResolver,
	}
}

func (r *ProviderRuntimeAuditRecorder) RecordSandboxRuntimeAudit(
	ctx context.Context,
	observation SandboxRuntimeAuditObservation,
) {
	if r == nil || r.repository == nil || r.actorResolver == nil || ctx == nil ||
		observation.ProviderID <= 0 || observation.CorrelationID == "" {
		return
	}
	actorUserID := r.actorResolver(ctx)
	if actorUserID <= 0 {
		return
	}
	outcome := observation.Outcome
	if outcome != "success" && outcome != "failure" {
		outcome = "failure"
	}
	input, err := domainsandbox.NormalizeAppendProviderAuditEventInput(
		domainsandbox.AppendProviderAuditEventInput{
			ProviderID:  observation.ProviderID,
			ActorUserID: actorUserID,
			Action:      sandboxRuntimeAuditActionExecute,
			Result:      outcome,
			RequestID:   sandboxRuntimeAuditCorrelationRef(observation.CorrelationID),
			Metadata: map[string]string{
				domainsandbox.AuditMetadataKeyScope: string(observation.Scope),
			},
		},
	)
	if err != nil {
		return
	}
	if _, err = r.repository.AppendProviderAuditEvent(ctx, input); err != nil {
		logs.CtxWarnf(ctx, "[sandbox-runtime-audit] append failed")
	}
}

func sandboxRuntimeAuditCorrelationRef(correlationID string) string {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("sandbox-runtime-correlation-v1:" + correlationID))
	return fmt.Sprintf("corr_%x", digest[:12])
}

/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"errors"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type GuardrailAuditRecorder interface {
	RecordGuardrailDecision(
		ctx context.Context,
		request GuardrailRequest,
		decision GuardrailDecision,
	) error
}

type ApplicationGuardrailAuditRecorderOptions struct {
	Repository domainrepo.GuardrailAuditRepository
	IDGen      idgen.IDGenerator
	NowMillis  func() int64
}

type ApplicationGuardrailAuditRecorder struct {
	repository domainrepo.GuardrailAuditRepository
	idGen      idgen.IDGenerator
	nowMillis  func() int64
}

func NewApplicationGuardrailAuditRecorder(
	options ApplicationGuardrailAuditRecorderOptions,
) *ApplicationGuardrailAuditRecorder {
	nowMillis := options.NowMillis
	if nowMillis == nil {
		nowMillis = func() int64 { return time.Now().UnixMilli() }
	}

	return &ApplicationGuardrailAuditRecorder{
		repository: options.Repository,
		idGen:      options.IDGen,
		nowMillis:  nowMillis,
	}
}

func (r *ApplicationGuardrailAuditRecorder) RecordGuardrailDecision(
	ctx context.Context,
	request GuardrailRequest,
	decision GuardrailDecision,
) error {
	if !validGuardrailAuditRequest(request) ||
		r == nil ||
		r.repository == nil ||
		r.idGen == nil {
		return errors.New("guardrail audit record failed")
	}
	id, err := r.idGen.GenID(ctx)
	if err != nil || id <= 0 {
		return errors.New("guardrail audit record failed")
	}
	decision = normalizeGuardrailDecision(decision)
	event := &domainentity.GuardrailAuditEvent{
		ID:         id,
		SpaceID:    request.SpaceID,
		ThreadID:   request.ThreadID,
		RunID:      request.RunID,
		ActorID:    request.UserID,
		EventType:  guardrailAuditEventType(decision.Action),
		TargetType: sanitizeGuardrailIdentifier(string(request.TargetType), 32),
		TargetID:   sanitizeGuardrailAuditTargetID(request.TargetID),
		Operation:  sanitizeGuardrailIdentifier(request.Operation, 64),
		Source:     sanitizeGuardrailIdentifier(request.Source, 64),
		Action:     string(decision.Action),
		FailMode:   string(request.FailMode),
		Provider:   decision.Provider,
		ReasonCode: decision.ReasonCode,
		RuleIDs:    strings.Join(decision.RuleIDs, ","),
		CreatedAt:  r.now(),
	}
	if event.TargetType == "" {
		event.TargetType = "unknown"
	}
	if event.Operation == "" {
		event.Operation = "unknown"
	}
	if event.Source == "" {
		event.Source = "unknown"
	}
	if event.FailMode == "" {
		event.FailMode = string(GuardrailFailClosed)
	}
	if err := r.repository.CreateGuardrailAuditEvent(ctx, event); err != nil {
		return errors.New("guardrail audit record failed")
	}

	return nil
}

func (r *ApplicationGuardrailAuditRecorder) now() int64 {
	if r == nil || r.nowMillis == nil {
		return time.Now().UnixMilli()
	}

	return r.nowMillis()
}

func validGuardrailAuditRequest(request GuardrailRequest) bool {
	return request.SpaceID > 0 &&
		request.ThreadID > 0 &&
		request.RunID > 0 &&
		request.UserID > 0
}

func guardrailAuditEventType(action GuardrailAction) string {
	if guardrailActionRank(action) == 0 {
		action = GuardrailActionDeny
	}
	return "guardrail.decision." + string(action)
}

func sanitizeGuardrailAuditTargetID(targetID string) string {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return ""
	}
	if containsUnsafeGuardrailValue(targetID) {
		return "redacted"
	}
	targetID = sanitizeGuardrailIdentifier(targetID, 128)
	if targetID == "" {
		return "redacted"
	}
	return targetID
}

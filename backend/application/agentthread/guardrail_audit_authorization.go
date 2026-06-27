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
	"fmt"

	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

var ErrGuardrailAuditAccessDenied = errors.New("guardrail audit access denied")

type GuardrailAuditAccessOperation string

const (
	GuardrailAuditAccessOperationList   GuardrailAuditAccessOperation = "list"
	GuardrailAuditAccessOperationExport GuardrailAuditAccessOperation = "export"
)

type GuardrailAuditAccessRequest struct {
	ThreadID  int64
	RunID     int64
	ViewerID  int64
	Operation GuardrailAuditAccessOperation
}

type GuardrailAuditAuthorizer interface {
	AuthorizeGuardrailAuditAccess(ctx context.Context, req GuardrailAuditAccessRequest) error
}

type ThreadOwnerGuardrailAuditAuthorizer struct {
	ThreadSVC domainservice.ThreadService
}

func NewThreadOwnerGuardrailAuditAuthorizer(
	threadSVC domainservice.ThreadService,
) *ThreadOwnerGuardrailAuditAuthorizer {
	return &ThreadOwnerGuardrailAuditAuthorizer{ThreadSVC: threadSVC}
}

func (a *ThreadOwnerGuardrailAuditAuthorizer) AuthorizeGuardrailAuditAccess(
	ctx context.Context,
	req GuardrailAuditAccessRequest,
) error {
	if req.ThreadID <= 0 || req.ViewerID <= 0 {
		return ErrGuardrailAuditAccessDenied
	}
	if a == nil || a.ThreadSVC == nil {
		return fmt.Errorf("guardrail audit authorizer thread service is not configured")
	}

	thread, err := a.ThreadSVC.GetThread(ctx, req.ThreadID)
	if err != nil {
		return err
	}
	if thread == nil || thread.CreatorID != req.ViewerID {
		return ErrGuardrailAuditAccessDenied
	}

	return nil
}

func (s *ApplicationService) authorizeGuardrailAuditAccess(
	ctx context.Context,
	req GuardrailAuditAccessRequest,
) error {
	if s == nil || s.GuardrailAuditAuthorizer == nil {
		return nil
	}

	return s.GuardrailAuditAuthorizer.AuthorizeGuardrailAuditAccess(ctx, req)
}

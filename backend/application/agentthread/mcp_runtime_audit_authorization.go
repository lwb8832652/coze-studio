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

var ErrMCPRuntimeAuditAccessDenied = errors.New("mcp runtime audit access denied")

type MCPRuntimeAuditAccessOperation string

const (
	MCPRuntimeAuditAccessOperationList MCPRuntimeAuditAccessOperation = "list"
)

type MCPRuntimeAuditAccessRequest struct {
	ThreadID  int64
	RunID     int64
	ViewerID  int64
	Operation MCPRuntimeAuditAccessOperation
}

type MCPRuntimeAuditAuthorizer interface {
	AuthorizeMCPRuntimeAuditAccess(ctx context.Context, req MCPRuntimeAuditAccessRequest) error
}

type ThreadOwnerMCPRuntimeAuditAuthorizer struct {
	ThreadSVC domainservice.ThreadService
}

func NewThreadOwnerMCPRuntimeAuditAuthorizer(
	threadSVC domainservice.ThreadService,
) *ThreadOwnerMCPRuntimeAuditAuthorizer {
	return &ThreadOwnerMCPRuntimeAuditAuthorizer{ThreadSVC: threadSVC}
}

func (a *ThreadOwnerMCPRuntimeAuditAuthorizer) AuthorizeMCPRuntimeAuditAccess(
	ctx context.Context,
	req MCPRuntimeAuditAccessRequest,
) error {
	if req.ThreadID <= 0 || req.ViewerID <= 0 {
		return ErrMCPRuntimeAuditAccessDenied
	}
	if a == nil || a.ThreadSVC == nil {
		return fmt.Errorf("mcp runtime audit authorizer thread service is not configured")
	}

	thread, err := a.ThreadSVC.GetThread(ctx, req.ThreadID)
	if err != nil {
		return err
	}
	if thread == nil || thread.CreatorID != req.ViewerID {
		return ErrMCPRuntimeAuditAccessDenied
	}

	return nil
}

func (s *ApplicationService) authorizeMCPRuntimeAuditAccess(
	ctx context.Context,
	req MCPRuntimeAuditAccessRequest,
) error {
	if s == nil || s.MCPRuntimeAuditAuthorizer == nil {
		return nil
	}

	return s.MCPRuntimeAuditAuthorizer.AuthorizeMCPRuntimeAuditAccess(ctx, req)
}

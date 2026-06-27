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

var ErrMemoryAccessDenied = errors.New("memory access denied")

type MemoryAccessOperation string

const (
	MemoryAccessOperationList    MemoryAccessOperation = "list"
	MemoryAccessOperationExport  MemoryAccessOperation = "export"
	MemoryAccessOperationImport  MemoryAccessOperation = "import"
	MemoryAccessOperationUpdate  MemoryAccessOperation = "update"
	MemoryAccessOperationDelete  MemoryAccessOperation = "delete"
	MemoryAccessOperationClear   MemoryAccessOperation = "clear"
	MemoryAccessOperationRestore MemoryAccessOperation = "restore"
	MemoryAccessOperationAudit   MemoryAccessOperation = "audit"
)

type MemoryAccessRequest struct {
	ThreadID  int64
	MemoryID  int64
	ViewerID  int64
	Operation MemoryAccessOperation
}

type MemoryAuthorizer interface {
	AuthorizeMemoryAccess(ctx context.Context, req MemoryAccessRequest) error
}

type ThreadOwnerMemoryAuthorizer struct {
	ThreadSVC domainservice.ThreadService
}

func NewThreadOwnerMemoryAuthorizer(
	threadSVC domainservice.ThreadService,
) *ThreadOwnerMemoryAuthorizer {
	return &ThreadOwnerMemoryAuthorizer{ThreadSVC: threadSVC}
}

func (a *ThreadOwnerMemoryAuthorizer) AuthorizeMemoryAccess(
	ctx context.Context,
	req MemoryAccessRequest,
) error {
	if req.ThreadID <= 0 || req.ViewerID <= 0 {
		return ErrMemoryAccessDenied
	}
	if a == nil || a.ThreadSVC == nil {
		return fmt.Errorf("memory authorizer thread service is not configured")
	}

	thread, err := a.ThreadSVC.GetThread(ctx, req.ThreadID)
	if err != nil {
		return err
	}
	if thread == nil || thread.CreatorID != req.ViewerID {
		return ErrMemoryAccessDenied
	}

	return nil
}

func (s *ApplicationService) authorizeMemoryAccess(
	ctx context.Context,
	req MemoryAccessRequest,
) error {
	if s == nil || s.MemoryAuthorizer == nil {
		return nil
	}

	return s.MemoryAuthorizer.AuthorizeMemoryAccess(ctx, req)
}

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

var ErrArtifactAccessDenied = errors.New("artifact access denied")

type ArtifactAccessOperation string

const (
	ArtifactAccessOperationList    ArtifactAccessOperation = "list"
	ArtifactAccessOperationRead    ArtifactAccessOperation = "read"
	ArtifactAccessOperationDelete  ArtifactAccessOperation = "delete"
	ArtifactAccessOperationRestore ArtifactAccessOperation = "restore"
	ArtifactAccessOperationReview  ArtifactAccessOperation = "review"
)

type ArtifactAccessRequest struct {
	ThreadID   int64
	ArtifactID int64
	SpaceID    int64
	ViewerID   int64
	Operation  ArtifactAccessOperation
}

type ArtifactAuthorizer interface {
	AuthorizeArtifactAccess(ctx context.Context, req ArtifactAccessRequest) error
}

type ThreadOwnerArtifactAuthorizer struct {
	ThreadSVC domainservice.ThreadService
}

func NewThreadOwnerArtifactAuthorizer(
	threadSVC domainservice.ThreadService,
) *ThreadOwnerArtifactAuthorizer {
	return &ThreadOwnerArtifactAuthorizer{ThreadSVC: threadSVC}
}

func (a *ThreadOwnerArtifactAuthorizer) AuthorizeArtifactAccess(
	ctx context.Context,
	req ArtifactAccessRequest,
) error {
	if req.ThreadID <= 0 || req.ViewerID <= 0 {
		return ErrArtifactAccessDenied
	}
	if a == nil || a.ThreadSVC == nil {
		return fmt.Errorf("artifact authorizer thread service is not configured")
	}

	thread, err := a.ThreadSVC.GetThread(ctx, req.ThreadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return ErrArtifactAccessDenied
	}
	if thread.CreatorID == req.ViewerID {
		return nil
	}

	return ErrArtifactAccessDenied
}

func (s *ApplicationService) authorizeArtifactAccess(
	ctx context.Context,
	req ArtifactAccessRequest,
) error {
	if s == nil || s.ArtifactAuthorizer == nil {
		return nil
	}

	return s.ArtifactAuthorizer.AuthorizeArtifactAccess(ctx, req)
}

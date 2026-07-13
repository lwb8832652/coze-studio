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
	"sync"

	appctxutil "github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

var ErrThreadAccessDenied = errors.New("thread access denied")
var ErrThreadAuthorizationUnavailable = errors.New("thread authorization unavailable")

type ThreadAccessRequest struct {
	ViewerID int64
	SpaceID  int64
	ThreadID int64
	RunID    int64
}

type ThreadAuthorizer interface {
	AuthorizeThreadAccess(context.Context, ThreadAccessRequest) error
}

type WorkspaceAccessRequest struct {
	ViewerID int64
	SpaceID  int64
}

type WorkspaceAuthorizer interface {
	AuthorizeWorkspaceAccess(context.Context, WorkspaceAccessRequest) error
}

type UserSpaceReader interface {
	IsSpaceMember(context.Context, int64, int64) (bool, error)
}

type UserSpaceWorkspaceAuthorizer struct {
	UserSpaceReader UserSpaceReader
}

func NewUserSpaceWorkspaceAuthorizer(
	userSpaceReader UserSpaceReader,
) *UserSpaceWorkspaceAuthorizer {
	return &UserSpaceWorkspaceAuthorizer{UserSpaceReader: userSpaceReader}
}

func (a *UserSpaceWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	ctx context.Context,
	req WorkspaceAccessRequest,
) error {
	if req.ViewerID <= 0 || req.SpaceID <= 0 {
		return ErrThreadAccessDenied
	}
	if a == nil || a.UserSpaceReader == nil {
		return ErrThreadAuthorizationUnavailable
	}

	member, err := a.UserSpaceReader.IsSpaceMember(ctx, req.SpaceID, req.ViewerID)
	if err != nil {
		return fmt.Errorf(
			"%w: workspace membership lookup failed: %w",
			ErrThreadAuthorizationUnavailable,
			err,
		)
	}
	if member {
		return nil
	}

	return ErrThreadAccessDenied
}

type ThreadOwnerAuthorizer struct {
	ThreadSVC domainservice.ThreadService
}

func NewThreadOwnerAuthorizer(threadSVC domainservice.ThreadService) *ThreadOwnerAuthorizer {
	return &ThreadOwnerAuthorizer{ThreadSVC: threadSVC}
}

func (a *ThreadOwnerAuthorizer) AuthorizeThreadAccess(
	ctx context.Context,
	req ThreadAccessRequest,
) error {
	if req.ViewerID <= 0 || req.SpaceID <= 0 || req.ThreadID <= 0 || req.RunID < 0 {
		return ErrThreadAccessDenied
	}
	if a == nil || a.ThreadSVC == nil {
		return fmt.Errorf("thread authorizer service is not configured")
	}

	thread, err := loadThreadAccessThread(ctx, a.ThreadSVC, req.ThreadID)
	if err != nil || thread == nil {
		return ErrThreadAccessDenied
	}
	if thread.CreatorID != req.ViewerID || thread.SpaceID != req.SpaceID {
		return ErrThreadAccessDenied
	}
	if req.RunID == 0 {
		return nil
	}

	run, err := loadThreadAccessRun(ctx, a.ThreadSVC, req.RunID)
	if err != nil || run == nil || run.ThreadID != req.ThreadID {
		return ErrThreadAccessDenied
	}

	return nil
}

type threadAccessContextKey struct{}

type threadAccessContextState struct {
	mu                   sync.Mutex
	request              ThreadAccessRequest
	threads              map[int64]*domainentity.Thread
	runs                 map[int64]*domainentity.Run
	authorizedThreads    []ThreadAccessRequest
	authorizedWorkspaces []WorkspaceAccessRequest
}

func WithThreadAccessRequest(ctx context.Context, req ThreadAccessRequest) context.Context {
	return context.WithValue(ctx, threadAccessContextKey{}, &threadAccessContextState{
		request: req,
		threads: make(map[int64]*domainentity.Thread),
		runs:    make(map[int64]*domainentity.Run),
	})
}

func threadAccessRequestFromContext(ctx context.Context) (ThreadAccessRequest, bool) {
	if ctx == nil {
		return ThreadAccessRequest{}, false
	}
	value := ctx.Value(threadAccessContextKey{})
	if state, ok := value.(*threadAccessContextState); ok && state != nil {
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.request, true
	}
	req, ok := value.(ThreadAccessRequest)
	return req, ok
}

func ensureThreadAccessContextState(ctx context.Context) (context.Context, *threadAccessContextState) {
	if ctx != nil {
		if state, ok := ctx.Value(threadAccessContextKey{}).(*threadAccessContextState); ok && state != nil {
			return ctx, state
		}
	} else {
		ctx = context.Background()
	}
	state := &threadAccessContextState{
		threads: make(map[int64]*domainentity.Thread),
		runs:    make(map[int64]*domainentity.Run),
	}
	return context.WithValue(ctx, threadAccessContextKey{}, state), state
}

func (s *threadAccessContextState) threadAccessAuthorized(req ThreadAccessRequest) bool {
	if s == nil || req.ViewerID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, authorized := range s.authorizedThreads {
		if authorized.ViewerID != req.ViewerID {
			continue
		}
		if req.ThreadID > 0 && authorized.ThreadID != req.ThreadID {
			continue
		}
		if req.RunID > 0 && authorized.RunID != req.RunID {
			continue
		}
		if req.SpaceID > 0 && authorized.SpaceID != req.SpaceID {
			continue
		}
		if req.ThreadID > 0 || req.RunID > 0 {
			return true
		}
	}
	return false
}

func (s *threadAccessContextState) markThreadAccessAuthorized(req ThreadAccessRequest) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorizedThreads = append(s.authorizedThreads, req)
}

func (s *threadAccessContextState) workspaceAccessAuthorized(req WorkspaceAccessRequest) bool {
	if s == nil || req.ViewerID <= 0 || req.SpaceID <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, authorized := range s.authorizedWorkspaces {
		if authorized == req {
			return true
		}
	}
	return false
}

func (s *threadAccessContextState) markWorkspaceAccessAuthorized(req WorkspaceAccessRequest) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorizedWorkspaces = append(s.authorizedWorkspaces, req)
}

func loadThreadAccessThread(
	ctx context.Context,
	threadSVC domainservice.ThreadService,
	threadID int64,
) (*domainentity.Thread, error) {
	var state *threadAccessContextState
	if ctx != nil {
		state, _ = ctx.Value(threadAccessContextKey{}).(*threadAccessContextState)
	}
	if state != nil {
		state.mu.Lock()
		thread := state.threads[threadID]
		state.mu.Unlock()
		if thread != nil {
			return thread, nil
		}
	}
	thread, err := threadSVC.GetThread(ctx, threadID)
	if err == nil && thread != nil && state != nil {
		state.mu.Lock()
		state.threads[threadID] = thread
		state.mu.Unlock()
	}
	return thread, err
}

func loadThreadAccessRun(
	ctx context.Context,
	threadSVC domainservice.ThreadService,
	runID int64,
) (*domainentity.Run, error) {
	var state *threadAccessContextState
	if ctx != nil {
		state, _ = ctx.Value(threadAccessContextKey{}).(*threadAccessContextState)
	}
	if state != nil {
		state.mu.Lock()
		run := state.runs[runID]
		state.mu.Unlock()
		if run != nil {
			return run, nil
		}
	}
	run, err := threadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: runID})
	if err == nil && run != nil && state != nil {
		state.mu.Lock()
		state.runs[runID] = run
		state.mu.Unlock()
	}
	return run, err
}

func publicThreadAccessRequestFromContext(ctx context.Context) (ThreadAccessRequest, bool) {
	access, marked := threadAccessRequestFromContext(ctx)
	if viewerID := authenticatedThreadViewerID(ctx); viewerID > 0 {
		access.ViewerID = viewerID
		return access, true
	}
	return access, marked
}

func isPublicThreadAccessContext(ctx context.Context) bool {
	_, public := publicThreadAccessRequestFromContext(ctx)
	return public
}

func authenticatedThreadViewerID(ctx context.Context) int64 {
	if uid := appctxutil.GetUIDFromCtx(ctx); uid != nil {
		return *uid
	}
	if apiKey := appctxutil.GetApiAuthFromCtx(ctx); apiKey != nil {
		return apiKey.UserID
	}
	return 0
}

func (s *ApplicationService) AuthorizeThreadAccess(
	ctx context.Context,
	req ThreadAccessRequest,
) error {
	if s == nil || s.ThreadAuthorizer == nil || s.WorkspaceAuthorizer == nil || req.ViewerID <= 0 ||
		req.SpaceID < 0 || req.ThreadID < 0 || req.RunID < 0 {
		return ErrThreadAccessDenied
	}
	if s.ThreadSVC == nil {
		return ErrThreadAccessDenied
	}
	ctx, state := ensureThreadAccessContextState(ctx)
	if state.threadAccessAuthorized(req) {
		return nil
	}

	if req.ThreadID == 0 {
		if req.RunID <= 0 {
			return ErrThreadAccessDenied
		}
		run, err := loadThreadAccessRun(ctx, s.ThreadSVC, req.RunID)
		if err != nil || run == nil || run.ThreadID <= 0 {
			return ErrThreadAccessDenied
		}
		req.ThreadID = run.ThreadID
	}
	if req.SpaceID == 0 {
		thread, err := loadThreadAccessThread(ctx, s.ThreadSVC, req.ThreadID)
		if err != nil || thread == nil || thread.SpaceID <= 0 {
			return ErrThreadAccessDenied
		}
		req.SpaceID = thread.SpaceID
	}

	if err := s.ThreadAuthorizer.AuthorizeThreadAccess(ctx, req); err != nil {
		return err
	}
	if err := s.AuthorizeWorkspaceAccess(ctx, WorkspaceAccessRequest{
		ViewerID: req.ViewerID,
		SpaceID:  req.SpaceID,
	}); err != nil {
		return err
	}
	state.markThreadAccessAuthorized(req)
	return nil
}

func (s *ApplicationService) AuthorizeWorkspaceAccess(
	ctx context.Context,
	req WorkspaceAccessRequest,
) error {
	if s == nil || s.WorkspaceAuthorizer == nil || req.ViewerID <= 0 || req.SpaceID <= 0 {
		return ErrThreadAccessDenied
	}
	ctx, state := ensureThreadAccessContextState(ctx)
	if state.workspaceAccessAuthorized(req) {
		return nil
	}
	if err := s.WorkspaceAuthorizer.AuthorizeWorkspaceAccess(ctx, req); err != nil {
		return err
	}
	state.markWorkspaceAccessAuthorized(req)
	return nil
}

func (s *ApplicationService) authorizeThreadAccessFromContext(
	ctx context.Context,
	resource ThreadAccessRequest,
) error {
	access, public := publicThreadAccessRequestFromContext(ctx)
	if !public {
		return nil
	}
	if resource.ThreadID > 0 && access.ThreadID > 0 && resource.ThreadID != access.ThreadID {
		return ErrThreadAccessDenied
	}
	if resource.RunID > 0 && access.RunID > 0 && resource.RunID != access.RunID {
		return ErrThreadAccessDenied
	}
	if resource.SpaceID > 0 && access.SpaceID > 0 && resource.SpaceID != access.SpaceID {
		return ErrThreadAccessDenied
	}
	if resource.ThreadID > 0 {
		access.ThreadID = resource.ThreadID
	}
	if resource.RunID > 0 {
		access.RunID = resource.RunID
	}
	if resource.SpaceID > 0 {
		access.SpaceID = resource.SpaceID
	}

	return s.AuthorizeThreadAccess(ctx, access)
}

func (s *ApplicationService) publicThreadViewerID(ctx context.Context) (int64, bool, error) {
	access, public := publicThreadAccessRequestFromContext(ctx)
	if !public {
		return 0, false, nil
	}
	if s == nil || s.ThreadAuthorizer == nil || s.WorkspaceAuthorizer == nil || access.ViewerID <= 0 {
		return 0, true, ErrThreadAccessDenied
	}

	return access.ViewerID, true, nil
}

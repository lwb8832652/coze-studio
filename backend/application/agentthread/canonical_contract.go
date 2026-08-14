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

	"gorm.io/gorm"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

var (
	ErrUnsupportedPublicStateChannel = domainservice.ErrUnsupportedPublicStateChannel
	ErrPublicThreadStateConflict     = domainservice.ErrPublicThreadStateConflict
)

type CanonicalPage struct {
	Offset int32
	Limit  int32
}

type SearchThreadsRequest struct {
	SpaceID   int64
	UserID    int64
	IDs       []int64
	Status    *ThreadStatus
	Metadata  map[string]any
	SortBy    string
	SortOrder string
	Page      CanonicalPage
}

type SearchThreadsResponse struct {
	Threads []*ThreadSummary
	Total   int64
}

type SearchRunsRequest struct {
	ThreadID    int64
	ParentRunID *int64
	Status      *RunStatus
	Page        CanonicalPage
}

type SearchRunsResponse struct {
	Runs  []*RunSummary
	Total int64
}

type ListRunEventsByCursorRequest struct {
	ThreadID     int64
	RunID        int64
	AfterEventID int64
	EventTypes   []string
	Limit        int32
}

type ListRunEventsByCursorResponse struct {
	Events  []*RunEventSummary
	Total   int64
	HasMore bool
}

type ListCheckpointsBeforeRequest struct {
	ThreadID           int64
	BeforeCheckpointID int64
	Limit              int32
}

type ListCheckpointsBeforeResponse struct {
	Checkpoints []*CheckpointSummary
	HasMore     bool
}

type PatchThreadRequest struct {
	ThreadID      int64
	Title         *string
	MetadataPatch map[string]any
	UpdatedAt     int64
}

type PatchThreadResponse struct {
	Thread *ThreadSummary
}

type DeleteThreadIfIdleRequest struct {
	ThreadID int64
}

type DeleteThreadIfIdleResponse struct {
	Deleted bool
}

type UpdatePublicThreadStateRequest struct {
	ThreadID         int64
	BaseCheckpointID int64
	Values           map[string]any
	AsNode           string
}

type UpdatePublicThreadStateResponse struct {
	Checkpoint *CheckpointSummary
}

func (s *ApplicationService) SearchThreads(
	ctx context.Context,
	req *SearchThreadsRequest,
) (*SearchThreadsResponse, error) {
	querySVC, err := s.requireCanonicalQueryService()
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("search threads request is required")
	}
	userID := req.UserID
	if viewerID, public, err := s.publicThreadViewerID(ctx); err != nil {
		return nil, err
	} else if public {
		if req.SpaceID <= 0 {
			return nil, ErrThreadAccessDenied
		}
		if err := s.AuthorizeWorkspaceAccess(ctx, WorkspaceAccessRequest{
			ViewerID: viewerID,
			SpaceID:  req.SpaceID,
		}); err != nil {
			return nil, err
		}
		userID = viewerID
	}

	var status *domainentity.ThreadStatus
	if req.Status != nil {
		mapped := domainentity.ThreadStatus(*req.Status)
		status = &mapped
	}
	threads, total, err := querySVC.SearchThreads(ctx, &domainservice.SearchThreadsRequest{
		SpaceID: req.SpaceID, UserID: userID,
		IDs: append([]int64(nil), req.IDs...), Status: status,
		Metadata: req.Metadata, SortBy: req.SortBy, SortOrder: req.SortOrder,
		Page: domainservice.CanonicalPage{Offset: req.Page.Offset, Limit: req.Page.Limit},
	})
	if err != nil {
		return nil, err
	}
	resp := &SearchThreadsResponse{Threads: make([]*ThreadSummary, 0, len(threads)), Total: total}
	for _, thread := range threads {
		resp.Threads = append(resp.Threads, DomainThreadToSummary(thread))
	}
	return resp, nil
}

func (s *ApplicationService) SearchRuns(
	ctx context.Context,
	req *SearchRunsRequest,
) (*SearchRunsResponse, error) {
	querySVC, err := s.requireCanonicalQueryService()
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("search runs request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}
	var status *domainentity.RunStatus
	if req.Status != nil {
		mapped := domainentity.RunStatus(*req.Status)
		status = &mapped
	}
	runs, total, err := querySVC.SearchRuns(ctx, &domainservice.SearchRunsRequest{
		ThreadID: req.ThreadID, ParentRunID: req.ParentRunID, Status: status,
		Page: domainservice.CanonicalPage{Offset: req.Page.Offset, Limit: req.Page.Limit},
	})
	if err != nil {
		return nil, err
	}
	resp := &SearchRunsResponse{Runs: make([]*RunSummary, 0, len(runs)), Total: total}
	for _, run := range runs {
		summary := DomainRunToSummary(run)
		if err := s.hydratePublicAdaptiveExecution(ctx, summary); err != nil {
			return nil, err
		}
		resp.Runs = append(resp.Runs, summary)
	}
	return resp, nil
}

func (s *ApplicationService) ListRunEventsByCursor(
	ctx context.Context,
	req *ListRunEventsByCursorRequest,
) (*ListRunEventsByCursorResponse, error) {
	querySVC, err := s.requireCanonicalQueryService()
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list run events by cursor request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}
	events, total, hasMore, err := querySVC.ListRunEventsByCursor(
		ctx,
		&domainservice.ListRunEventsByCursorRequest{
			ThreadID: req.ThreadID, RunID: req.RunID, AfterEventID: req.AfterEventID,
			EventTypes: append([]string(nil), req.EventTypes...), Limit: req.Limit,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListRunEventsByCursorResponse{
		Events: make([]*RunEventSummary, 0, len(events)), Total: total, HasMore: hasMore,
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainRunEventToSummary(event))
	}
	return resp, nil
}

func (s *ApplicationService) ListCheckpointsBefore(
	ctx context.Context,
	req *ListCheckpointsBeforeRequest,
) (*ListCheckpointsBeforeResponse, error) {
	querySVC, err := s.requireCanonicalQueryService()
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list checkpoints before request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}
	checkpoints, hasMore, err := querySVC.ListCheckpointsBefore(
		ctx,
		&domainservice.ListCheckpointsBeforeRequest{
			ThreadID: req.ThreadID, BeforeCheckpointID: req.BeforeCheckpointID, Limit: req.Limit,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListCheckpointsBeforeResponse{
		Checkpoints: make([]*CheckpointSummary, 0, len(checkpoints)), HasMore: hasMore,
	}
	for _, checkpoint := range checkpoints {
		resp.Checkpoints = append(resp.Checkpoints, DomainCheckpointToSummary(checkpoint))
	}
	return resp, nil
}

func (s *ApplicationService) PatchThread(
	ctx context.Context,
	req *PatchThreadRequest,
) (*PatchThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("patch thread request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}
	patchSVC, ok := s.ThreadSVC.(domainservice.CanonicalPatchService)
	if !ok {
		return nil, fmt.Errorf("canonical agent thread patch service is not configured")
	}
	thread, err := patchSVC.PatchThread(ctx, &domainservice.PatchThreadRequest{
		ThreadID: req.ThreadID, Title: req.Title,
		MetadataPatch: req.MetadataPatch, UpdatedAt: req.UpdatedAt,
	})
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty patched thread")
	}
	return &PatchThreadResponse{Thread: DomainThreadToSummary(thread)}, nil
}

func (s *ApplicationService) DeleteThreadIfIdle(
	ctx context.Context,
	req *DeleteThreadIfIdleRequest,
) (*DeleteThreadIfIdleResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("delete thread if idle request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}
	deleteSVC, ok := s.ThreadSVC.(domainservice.CanonicalDeleteService)
	if !ok {
		return nil, fmt.Errorf("canonical agent thread delete service is not configured")
	}
	deleted, err := deleteSVC.DeleteThreadIfIdle(ctx, &domainservice.DeleteThreadIfIdleRequest{
		ThreadID: req.ThreadID,
	})
	if err != nil {
		return nil, err
	}
	return &DeleteThreadIfIdleResponse{Deleted: deleted}, nil
}

func (s *ApplicationService) UpdatePublicThreadState(
	ctx context.Context,
	req *UpdatePublicThreadStateRequest,
) (*UpdatePublicThreadStateResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, domainservice.InvalidArgumentErrorf("update public thread state request is required")
	}
	if req.ThreadID <= 0 {
		return nil, domainservice.InvalidArgumentErrorf("thread id is required")
	}
	if req.BaseCheckpointID < 0 {
		return nil, domainservice.InvalidArgumentErrorf("base checkpoint id cannot be negative")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}
	publicStateSVC, ok := s.ThreadSVC.(domainservice.CanonicalPublicStateService)
	if !ok {
		return nil, fmt.Errorf("canonical public thread state service is not configured")
	}
	checkpoint, err := publicStateSVC.UpdatePublicThreadState(ctx, &domainservice.UpdatePublicThreadStateRequest{
		ThreadID: req.ThreadID, BaseCheckpointID: req.BaseCheckpointID,
		Values: req.Values, AsNode: req.AsNode,
	})
	if err != nil {
		return nil, canonicalPublicStateResourceError(ctx, err)
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("agent thread service returned empty public state checkpoint")
	}
	return &UpdatePublicThreadStateResponse{
		Checkpoint: DomainCheckpointToSummary(checkpoint),
	}, nil
}

func canonicalPublicStateResourceError(ctx context.Context, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return canonicalPublicStateNotFound(ctx)
	}
	return err
}

func canonicalPublicStateNotFound(ctx context.Context) error {
	if isPublicThreadAccessContext(ctx) {
		return ErrThreadAccessDenied
	}
	return gorm.ErrRecordNotFound
}

func (s *ApplicationService) requireCanonicalQueryService() (domainservice.CanonicalQueryService, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	querySVC, ok := s.ThreadSVC.(domainservice.CanonicalQueryService)
	if !ok {
		return nil, fmt.Errorf("canonical agent thread query service is not configured")
	}
	return querySVC, nil
}

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
	"fmt"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

var SVC = new(ApplicationService)

type ApplicationService struct {
	ThreadSVC domainservice.ThreadService
}

func (s *ApplicationService) CreateThread(ctx context.Context, req *CreateThreadRequest) (*CreateThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create thread request is required")
	}

	thread, err := s.ThreadSVC.CreateThread(ctx, &domainservice.CreateThreadRequest{
		SpaceID:      req.SpaceID,
		UserID:       req.UserID,
		AgentID:      req.AgentID,
		Title:        req.Title,
		Source:       domainentity.ThreadSource(req.Source),
		LegacyTaskID: req.LegacyTaskID,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty thread")
	}

	return &CreateThreadResponse{Thread: DomainThreadToSummary(thread)}, nil
}

func (s *ApplicationService) GetThread(ctx context.Context, req *GetThreadRequest) (*GetThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get thread request is required")
	}

	thread, err := s.ThreadSVC.GetThread(ctx, req.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty thread")
	}

	return &GetThreadResponse{Thread: DomainThreadToSummary(thread)}, nil
}

func (s *ApplicationService) ListThreads(ctx context.Context, req *ListThreadsRequest) (*ListThreadsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list threads request is required")
	}

	var status *domainentity.ThreadStatus
	if req.Status != nil {
		mapped := domainentity.ThreadStatus(*req.Status)
		status = &mapped
	}

	threads, total, err := s.ThreadSVC.ListThreads(ctx, &domainservice.ListThreadsRequest{
		SpaceID:  req.SpaceID,
		UserID:   req.UserID,
		Status:   status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListThreadsResponse{
		Threads: make([]*ThreadSummary, 0, len(threads)),
		Total:   total,
	}
	for _, thread := range threads {
		resp.Threads = append(resp.Threads, DomainThreadToSummary(thread))
	}

	return resp, nil
}

func (s *ApplicationService) requireThreadSVC() error {
	if s == nil || s.ThreadSVC == nil {
		return fmt.Errorf("agent thread service is not initialized")
	}

	return nil
}

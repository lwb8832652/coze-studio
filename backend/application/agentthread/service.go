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

func (s *ApplicationService) AppendMessage(ctx context.Context, req *AppendMessageRequest) (*AppendMessageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("append message request is required")
	}

	message, err := s.ThreadSVC.AppendMessage(ctx, &domainservice.AppendMessageRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Role:     domainentity.MessageRole(req.Role),
		Content:  req.Content,
		Metadata: req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, fmt.Errorf("agent thread service returned empty message")
	}

	return &AppendMessageResponse{Message: DomainMessageToSummary(message)}, nil
}

func (s *ApplicationService) ListMessages(ctx context.Context, req *ListMessagesRequest) (*ListMessagesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list messages request is required")
	}

	messages, total, err := s.ThreadSVC.ListMessages(ctx, &domainservice.ListMessagesRequest{
		ThreadID: req.ThreadID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListMessagesResponse{
		Messages: make([]*MessageSummary, 0, len(messages)),
		Total:    total,
	}
	for _, message := range messages {
		resp.Messages = append(resp.Messages, DomainMessageToSummary(message))
	}

	return resp, nil
}

func (s *ApplicationService) CreateRun(ctx context.Context, req *CreateRunRequest) (*CreateRunResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create run request is required")
	}

	run, err := s.ThreadSVC.CreateRun(ctx, &domainservice.CreateRunRequest{
		ThreadID:          req.ThreadID,
		AssistantID:       req.AssistantID,
		Status:            domainentity.RunStatus(req.Status),
		Command:           req.Command,
		Input:             req.Input,
		Config:            req.Config,
		Context:           req.Context,
		Metadata:          req.Metadata,
		StreamMode:        req.StreamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
		IdempotencyKey:    req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty run")
	}

	return &CreateRunResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) GetRun(ctx context.Context, req *GetRunRequest) (*GetRunResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get run request is required")
	}

	run, err := s.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: req.RunID})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty run")
	}

	return &GetRunResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) ListRuns(ctx context.Context, req *ListRunsRequest) (*ListRunsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list runs request is required")
	}

	var status *domainentity.RunStatus
	if req.Status != nil {
		mapped := domainentity.RunStatus(*req.Status)
		status = &mapped
	}

	runs, total, err := s.ThreadSVC.ListRuns(ctx, &domainservice.ListRunsRequest{
		ThreadID: req.ThreadID,
		Status:   status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListRunsResponse{
		Runs:  make([]*RunSummary, 0, len(runs)),
		Total: total,
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}

	return resp, nil
}

func (s *ApplicationService) AppendRunEvent(ctx context.Context, req *AppendRunEventRequest) (*AppendRunEventResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("append run event request is required")
	}

	event, err := s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		EventType: req.EventType,
		Payload:   req.Payload,
	})
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, fmt.Errorf("agent thread service returned empty run event")
	}

	return &AppendRunEventResponse{Event: DomainRunEventToSummary(event)}, nil
}

func (s *ApplicationService) ListRunEvents(ctx context.Context, req *ListRunEventsRequest) (*ListRunEventsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list run events request is required")
	}

	events, total, err := s.ThreadSVC.ListRunEvents(ctx, &domainservice.ListRunEventsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListRunEventsResponse{
		Events: make([]*RunEventSummary, 0, len(events)),
		Total:  total,
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainRunEventToSummary(event))
	}

	return resp, nil
}

func (s *ApplicationService) CreateCheckpoint(ctx context.Context, req *CreateCheckpointRequest) (*CreateCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create checkpoint request is required")
	}

	checkpoint, err := s.ThreadSVC.CreateCheckpoint(ctx, &domainservice.CreateCheckpointRequest{
		ThreadID:           req.ThreadID,
		RunID:              req.RunID,
		ParentCheckpointID: req.ParentCheckpointID,
		CheckpointNS:       req.CheckpointNS,
		ChannelValues:      req.ChannelValues,
		ChannelVersions:    req.ChannelVersions,
		PendingSends:       req.PendingSends,
		Metadata:           req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("agent thread service returned empty checkpoint")
	}

	return &CreateCheckpointResponse{Checkpoint: DomainCheckpointToSummary(checkpoint)}, nil
}

func (s *ApplicationService) ListCheckpoints(ctx context.Context, req *ListCheckpointsRequest) (*ListCheckpointsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list checkpoints request is required")
	}

	checkpoints, total, err := s.ThreadSVC.ListCheckpoints(ctx, &domainservice.ListCheckpointsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListCheckpointsResponse{
		Checkpoints: make([]*CheckpointSummary, 0, len(checkpoints)),
		Total:       total,
	}
	for _, checkpoint := range checkpoints {
		resp.Checkpoints = append(resp.Checkpoints, DomainCheckpointToSummary(checkpoint))
	}

	return resp, nil
}

func (s *ApplicationService) GetCheckpoint(ctx context.Context, req *GetCheckpointRequest) (*GetCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get checkpoint request is required")
	}

	checkpoint, err := s.ThreadSVC.GetCheckpoint(ctx, &domainservice.GetCheckpointRequest{
		CheckpointID: req.CheckpointID,
	})
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("agent thread service returned empty checkpoint")
	}

	return &GetCheckpointResponse{Checkpoint: DomainCheckpointToSummary(checkpoint)}, nil
}

func (s *ApplicationService) GetLatestCheckpoint(ctx context.Context, req *GetLatestCheckpointRequest) (*GetLatestCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get latest checkpoint request is required")
	}

	checkpoint, err := s.ThreadSVC.GetLatestCheckpoint(ctx, &domainservice.GetLatestCheckpointRequest{
		ThreadID: req.ThreadID,
	})
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("agent thread service returned empty checkpoint")
	}

	return &GetLatestCheckpointResponse{Checkpoint: DomainCheckpointToSummary(checkpoint)}, nil
}

func (s *ApplicationService) RememberMemory(ctx context.Context, req *RememberMemoryRequest) (*RememberMemoryResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("remember memory request is required")
	}

	memory, err := s.ThreadSVC.RememberMemory(ctx, &domainservice.RememberMemoryRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		Scope:     domainentity.MemoryScope(req.Scope),
		Content:   req.Content,
		Metadata:  req.Metadata,
		Score:     req.Score,
		ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	if memory == nil {
		return nil, fmt.Errorf("agent thread service returned empty memory")
	}

	return &RememberMemoryResponse{Memory: DomainMemoryToSummary(memory)}, nil
}

func (s *ApplicationService) RecallMemories(ctx context.Context, req *RecallMemoriesRequest) (*RecallMemoriesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("recall memories request is required")
	}

	scopes := make([]domainentity.MemoryScope, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		if scope == "" {
			continue
		}
		scopes = append(scopes, domainentity.MemoryScope(scope))
	}

	memories, total, err := s.ThreadSVC.RecallMemories(ctx, &domainservice.RecallMemoriesRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Scopes:   scopes,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &RecallMemoriesResponse{
		Memories: make([]*MemorySummary, 0, len(memories)),
		Total:    total,
	}
	for _, memory := range memories {
		resp.Memories = append(resp.Memories, DomainMemoryToSummary(memory))
	}

	return resp, nil
}

func (s *ApplicationService) RecordTokenUsage(ctx context.Context, req *RecordTokenUsageRequest) (*RecordTokenUsageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("record token usage request is required")
	}

	usage, err := s.ThreadSVC.RecordTokenUsage(ctx, &domainservice.RecordTokenUsageRequest{
		RunID:        req.RunID,
		Source:       domainentity.TokenUsageSource(req.Source),
		StepID:       req.StepID,
		StepIndex:    req.StepIndex,
		StepName:     req.StepName,
		ModelName:    req.ModelName,
		Provider:     req.Provider,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		TotalTokens:  req.TotalTokens,
		CostMicros:   req.CostMicros,
		Currency:     req.Currency,
		Estimated:    req.Estimated,
		RawUsage:     req.RawUsage,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, fmt.Errorf("agent thread service returned empty token usage")
	}

	return &RecordTokenUsageResponse{Usage: DomainTokenUsageToSummary(usage)}, nil
}

func (s *ApplicationService) GetRunTokenUsage(ctx context.Context, req *GetTokenUsageRequest) (*GetTokenUsageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get run token usage request is required")
	}

	rows, total, aggregate, err := s.ThreadSVC.GetRunTokenUsage(ctx, &domainservice.GetRunTokenUsageRequest{
		RunID:    req.RunID,
		Source:   domainentity.TokenUsageSource(req.Source),
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	return tokenUsageResponse(rows, total, aggregate), nil
}

func (s *ApplicationService) GetThreadTokenUsage(ctx context.Context, req *GetTokenUsageRequest) (*GetTokenUsageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get thread token usage request is required")
	}

	rows, total, aggregate, err := s.ThreadSVC.GetThreadTokenUsage(ctx, &domainservice.GetThreadTokenUsageRequest{
		ThreadID: req.ThreadID,
		Source:   domainentity.TokenUsageSource(req.Source),
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	return tokenUsageResponse(rows, total, aggregate), nil
}

func tokenUsageResponse(rows []*domainentity.TokenUsage, total int64, aggregate *domainentity.TokenUsageAggregate) *GetTokenUsageResponse {
	resp := &GetTokenUsageResponse{
		Usage:     make([]*TokenUsageSummary, 0, len(rows)),
		Total:     total,
		Aggregate: DomainTokenUsageAggregateToSummary(aggregate),
	}
	for _, usage := range rows {
		resp.Usage = append(resp.Usage, DomainTokenUsageToSummary(usage))
	}

	return resp
}

func (s *ApplicationService) ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) (*ClaimPendingRunsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("claim pending runs request is required")
	}

	runs, err := s.ThreadSVC.ClaimPendingRuns(ctx, &domainservice.ClaimPendingRunsRequest{
		WorkerID: req.WorkerID,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &ClaimPendingRunsResponse{
		Runs: make([]*RunSummary, 0, len(runs)),
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}

	return resp, nil
}

func (s *ApplicationService) ClaimQueuedResumeRuns(ctx context.Context, req *ClaimQueuedResumeRunsRequest) (*ClaimQueuedResumeRunsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("claim queued resume runs request is required")
	}

	runs, err := s.ThreadSVC.ClaimQueuedResumeRuns(ctx, &domainservice.ClaimQueuedResumeRunsRequest{
		WorkerID: req.WorkerID,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &ClaimQueuedResumeRunsResponse{
		Runs: make([]*RunSummary, 0, len(runs)),
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}

	return resp, nil
}

func (s *ApplicationService) CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}

	return s.updateRunStatus(ctx, req, s.ThreadSVC.CompleteRun)
}

func (s *ApplicationService) FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}

	return s.updateRunStatus(ctx, req, s.ThreadSVC.FailRun)
}

func (s *ApplicationService) CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}

	return s.updateRunStatus(ctx, req, s.ThreadSVC.CancelRun)
}

func (s *ApplicationService) updateRunStatus(
	ctx context.Context,
	req *UpdateRunStatusRequest,
	update func(context.Context, *domainservice.UpdateRunStatusRequest) (*domainentity.Run, error),
) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("update run status request is required")
	}
	if update == nil {
		return nil, fmt.Errorf("update run status handler is required")
	}

	run, err := update(ctx, &domainservice.UpdateRunStatusRequest{
		RunID:        req.RunID,
		From:         domainentity.RunStatus(req.From),
		To:           domainentity.RunStatus(req.To),
		WorkerID:     req.WorkerID,
		ErrorCode:    req.ErrorCode,
		ErrorMessage: req.ErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty run")
	}

	return &UpdateRunStatusResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) requireThreadSVC() error {
	if s == nil || s.ThreadSVC == nil {
		return fmt.Errorf("agent thread service is not initialized")
	}

	return nil
}

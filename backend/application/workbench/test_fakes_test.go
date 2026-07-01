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

package workbench

import (
	"context"

	"github.com/cloudwego/eino/schema"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	agentthreadentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	agentthreadsvc "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
)

type recordedWorkbenchEvent struct {
	taskID    int64
	eventType string
	payload   string
}

type recordingWorkbenchTaskApp struct {
	created *taskapi.ChatTask
	got     *taskapi.ChatTask

	createCalls int

	events          []recordedWorkbenchEvent
	completedID     int64
	completedResult string
	failedID        int64
	failError       string
}

func (r *recordingWorkbenchTaskApp) CreateRunningTask(_ context.Context, req *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error) {
	r.createCalls++
	if r.created == nil {
		r.created = &taskapi.ChatTask{
			ID:      1,
			SpaceID: req.SpaceID,
			Title:   req.Title,
			Status:  taskapi.TaskStatus_Running,
		}
	}
	return &taskapi.CreateTaskResponse{Code: 0, Msg: "success", Data: r.created}, nil
}

func (r *recordingWorkbenchTaskApp) GetTask(_ context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error) {
	if r.got == nil {
		r.got = &taskapi.ChatTask{
			ID:     req.TaskID,
			Status: taskapi.TaskStatus_Running,
		}
	}
	return &taskapi.GetTaskResponse{Code: 0, Msg: "success", Data: r.got}, nil
}

func (r *recordingWorkbenchTaskApp) AppendTaskEvent(_ context.Context, taskID int64, eventType, payload string) error {
	r.events = append(r.events, recordedWorkbenchEvent{taskID: taskID, eventType: eventType, payload: payload})
	return nil
}

func (r *recordingWorkbenchTaskApp) CompleteTask(_ context.Context, taskID int64, result string) error {
	r.completedID = taskID
	r.completedResult = result
	return nil
}

func (r *recordingWorkbenchTaskApp) FailTask(_ context.Context, taskID int64, errMsg string) error {
	r.failedID = taskID
	r.failError = errMsg
	return nil
}

type recordingAgentThreadService struct {
	createReq   *agentthreadsvc.CreateThreadRequest
	createCalls int
	createErr   error
}

func (r *recordingAgentThreadService) CreateThread(_ context.Context, req *agentthreadsvc.CreateThreadRequest) (*agentthreadentity.Thread, error) {
	r.createCalls++
	r.createReq = req
	if r.createErr != nil {
		return nil, r.createErr
	}

	return &agentthreadentity.Thread{
		ID:           200,
		SpaceID:      req.SpaceID,
		CreatorID:    req.UserID,
		Title:        req.Title,
		Status:       agentthreadentity.ThreadStatusIdle,
		Source:       req.Source,
		LegacyTaskID: req.LegacyTaskID,
		Metadata:     req.Metadata,
	}, nil
}

func (r *recordingAgentThreadService) GetThread(context.Context, int64) (*agentthreadentity.Thread, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) UpdateThreadTitle(
	context.Context,
	*agentthreadsvc.UpdateThreadTitleRequest,
) (*agentthreadentity.Thread, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) ListThreads(context.Context, *agentthreadsvc.ListThreadsRequest) ([]*agentthreadentity.Thread, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) AppendMessage(context.Context, *agentthreadsvc.AppendMessageRequest) (*agentthreadentity.Message, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) ListMessages(context.Context, *agentthreadsvc.ListMessagesRequest) ([]*agentthreadentity.Message, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) CreateRun(context.Context, *agentthreadsvc.CreateRunRequest) (*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) GetRun(context.Context, *agentthreadsvc.GetRunRequest) (*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) GetRunByIdempotencyKey(context.Context, int64, string) (*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) ListRuns(context.Context, *agentthreadsvc.ListRunsRequest) ([]*agentthreadentity.Run, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) AppendRunEvent(context.Context, *agentthreadsvc.AppendRunEventRequest) (*agentthreadentity.RunEvent, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) ListRunEvents(context.Context, *agentthreadsvc.ListRunEventsRequest) ([]*agentthreadentity.RunEvent, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) CreateCheckpoint(context.Context, *agentthreadsvc.CreateCheckpointRequest) (*agentthreadentity.Checkpoint, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) GetCheckpoint(context.Context, *agentthreadsvc.GetCheckpointRequest) (*agentthreadentity.Checkpoint, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) ListCheckpoints(context.Context, *agentthreadsvc.ListCheckpointsRequest) ([]*agentthreadentity.Checkpoint, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) GetLatestCheckpoint(context.Context, *agentthreadsvc.GetLatestCheckpointRequest) (*agentthreadentity.Checkpoint, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) GetLatestRuntimeCheckpoint(
	context.Context,
	*agentthreadsvc.GetLatestRuntimeCheckpointRequest,
) (*agentthreadentity.Checkpoint, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) DeleteRuntimeCheckpoint(
	context.Context,
	*agentthreadsvc.DeleteRuntimeCheckpointRequest,
) error {
	return nil
}

func (r *recordingAgentThreadService) RememberMemory(context.Context, *agentthreadsvc.RememberMemoryRequest) (*agentthreadentity.Memory, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) ImportMemories(
	context.Context,
	*agentthreadsvc.ImportMemoriesRequest,
) (*agentthreadsvc.ImportMemoriesResult, error) {
	return &agentthreadsvc.ImportMemoriesResult{}, nil
}

func (r *recordingAgentThreadService) RecallMemories(context.Context, *agentthreadsvc.RecallMemoriesRequest) ([]*agentthreadentity.Memory, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) ListMemories(context.Context, *agentthreadsvc.ListMemoriesRequest) ([]*agentthreadentity.Memory, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) UpdateMemory(context.Context, *agentthreadsvc.UpdateMemoryRequest) (*agentthreadentity.Memory, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) DeleteMemory(context.Context, *agentthreadsvc.DeleteMemoryRequest) (bool, error) {
	return false, nil
}

func (r *recordingAgentThreadService) ClearMemories(context.Context, *agentthreadsvc.ClearMemoriesRequest) (int64, error) {
	return 0, nil
}

func (r *recordingAgentThreadService) RestoreMemory(context.Context, *agentthreadsvc.RestoreMemoryRequest) (*agentthreadentity.Memory, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) ListMemoryAuditEvents(context.Context, *agentthreadsvc.ListMemoryAuditEventsRequest) ([]*agentthreadentity.MemoryAuditEvent, int64, error) {
	return nil, 0, nil
}

func (r *recordingAgentThreadService) PersistTranscriptSnapshot(
	context.Context,
	*agentthreadsvc.PersistTranscriptSnapshotRequest,
) (*agentthreadentity.TranscriptSnapshot, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) GetTranscriptSnapshot(
	context.Context,
	*agentthreadsvc.GetTranscriptSnapshotRequest,
) (*agentthreadentity.TranscriptSnapshot, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) EnqueueMemoryFlushJob(
	context.Context,
	*agentthreadsvc.EnqueueMemoryFlushJobRequest,
) (*agentthreadentity.MemoryFlushJob, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) ClaimMemoryFlushJobs(
	context.Context,
	*agentthreadsvc.ClaimMemoryFlushJobsRequest,
) ([]*agentthreadentity.MemoryFlushJob, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) CompleteMemoryFlushJob(
	context.Context,
	*agentthreadsvc.CompleteMemoryFlushJobRequest,
) (*agentthreadentity.MemoryFlushJob, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) RetryMemoryFlushJob(
	context.Context,
	*agentthreadsvc.RetryMemoryFlushJobRequest,
) (*agentthreadentity.MemoryFlushJob, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) FailMemoryFlushJob(
	context.Context,
	*agentthreadsvc.FailMemoryFlushJobRequest,
) (*agentthreadentity.MemoryFlushJob, bool, error) {
	return nil, false, nil
}

func (r *recordingAgentThreadService) RecordTokenUsage(context.Context, *agentthreadsvc.RecordTokenUsageRequest) (*agentthreadentity.TokenUsage, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) GetRunTokenUsage(context.Context, *agentthreadsvc.GetRunTokenUsageRequest) ([]*agentthreadentity.TokenUsage, int64, *agentthreadentity.TokenUsageAggregate, []*agentthreadentity.RunTokenUsageAggregate, error) {
	return nil, 0, nil, nil, nil
}

func (r *recordingAgentThreadService) GetThreadTokenUsage(context.Context, *agentthreadsvc.GetThreadTokenUsageRequest) ([]*agentthreadentity.TokenUsage, int64, *agentthreadentity.TokenUsageAggregate, error) {
	return nil, 0, nil, nil
}

func (r *recordingAgentThreadService) ClaimPendingRuns(context.Context, *agentthreadsvc.ClaimPendingRunsRequest) ([]*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) ClaimQueuedResumeRuns(context.Context, *agentthreadsvc.ClaimQueuedResumeRunsRequest) ([]*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) CompleteRun(context.Context, *agentthreadsvc.UpdateRunStatusRequest) (*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) InterruptRun(context.Context, *agentthreadsvc.UpdateRunStatusRequest) (*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) FailRun(context.Context, *agentthreadsvc.UpdateRunStatusRequest) (*agentthreadentity.Run, error) {
	return nil, nil
}

func (r *recordingAgentThreadService) CancelRun(context.Context, *agentthreadsvc.UpdateRunStatusRequest) (*agentthreadentity.Run, error) {
	return nil, nil
}

type fakeAgentRun struct {
	stream *schema.StreamReader[*agentrunentity.AgentRunResponse]
	req    *agentrunentity.AgentRunMeta
	err    error
}

func (f *fakeAgentRun) AgentRun(_ context.Context, req *agentrunentity.AgentRunMeta) (*schema.StreamReader[*agentrunentity.AgentRunResponse], error) {
	f.req = req
	return f.stream, f.err
}

func (f *fakeAgentRun) Delete(context.Context, []int64) error {
	return nil
}

func (f *fakeAgentRun) Create(context.Context, *agentrunentity.AgentRunMeta) (*agentrunentity.RunRecordMeta, error) {
	return &agentrunentity.RunRecordMeta{}, nil
}

func (f *fakeAgentRun) List(context.Context, *agentrunentity.ListRunRecordMeta) ([]*agentrunentity.RunRecordMeta, error) {
	return nil, nil
}

func (f *fakeAgentRun) GetByID(context.Context, int64) (*agentrunentity.RunRecordMeta, error) {
	return &agentrunentity.RunRecordMeta{}, nil
}

func (f *fakeAgentRun) Cancel(context.Context, *agentrunentity.CancelRunMeta) (*agentrunentity.RunRecordMeta, error) {
	return &agentrunentity.RunRecordMeta{}, nil
}

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

func (r *recordingAgentThreadService) ListThreads(context.Context, *agentthreadsvc.ListThreadsRequest) ([]*agentthreadentity.Thread, int64, error) {
	return nil, 0, nil
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

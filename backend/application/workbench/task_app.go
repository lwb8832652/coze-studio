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
	"fmt"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
)

type workbenchTaskApplication interface {
	CreateRunningTask(ctx context.Context, req *taskapi.CreateTaskRequest) (*taskapi.CreateTaskResponse, error)
	GetTask(ctx context.Context, req *taskapi.GetTaskRequest) (*taskapi.GetTaskResponse, error)
	AppendTaskEvent(ctx context.Context, taskID int64, eventType, payload string) error
	CompleteTask(ctx context.Context, taskID int64, result string) error
	FailTask(ctx context.Context, taskID int64, errMsg string) error
}

func (s *ApplicationService) appendTaskEvent(ctx context.Context, taskID int64, eventType, payload string) error {
	if s != nil && s.taskApp != nil {
		return s.taskApp.AppendTaskEvent(ctx, taskID, eventType, payload)
	}
	if s == nil || s.taskSVC == nil {
		return fmt.Errorf("workbench task service is not initialized")
	}
	return s.taskSVC.AppendTaskEvent(ctx, taskID, eventType, payload)
}

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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
	appskill "github.com/coze-dev/coze-studio/backend/application/skill"
	apptask "github.com/coze-dev/coze-studio/backend/application/task"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	agentrun "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/service"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type ApplicationService struct {
	skillSVC          *appskill.ApplicationService
	taskSVC           *apptask.ApplicationService
	agentThreadSVC    *appagentthread.ApplicationService
	mcpToolSVC        *appmcptool.ApplicationService
	taskApp           workbenchTaskApplication
	knowledgeSVC      crossknowledge.Knowledge
	agentRunSVC       agentrun.Run
	chatModelProvider chatModelProvider
	runAsync          func(func())
}

func (s *ApplicationService) HandleMessage(ctx context.Context, req *chatapi.WorkbenchChatRequest) (*chatapi.WorkbenchChatResponse, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("workbench chat request is required")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, InvalidArgumentErrorf("message is required")
	}

	mode, err := chatModeFromAPI(req.Mode)
	if err != nil {
		return nil, err
	}
	if _, _, err := workbenchRuntimeSettingsPayload(req); err != nil {
		return nil, err
	}

	task, err := s.prepareTask(ctx, req, message)
	if err != nil {
		return nil, err
	}

	turnReq := cloneWorkbenchChatRequest(req)
	runCtx := context.WithoutCancel(ctx)
	s.scheduleTurn(func() {
		s.executeAndCompleteTurn(runCtx, task.ID, turnReq, mode, message)
	})
	resultType := resultTypeFromMode(req.Mode)
	executionType := executionTypeFromMode(req.Mode)

	return &chatapi.WorkbenchChatResponse{
		Code: 0,
		Msg:  "success",
		Data: &chatapi.WorkbenchChatData{
			RouteTarget:    chatapi.RouteTarget_TaskEngine,
			Task:           task,
			ConversationID: task.ConversationID,
			ResultType:     stringPtr(resultType),
			ExecutionType:  optionalStringPtr(executionType),
		},
	}, nil
}

func (s *ApplicationService) scheduleTurn(fn func()) {
	if s != nil && s.runAsync != nil {
		s.runAsync(fn)
		return
	}
	go fn()
}

func (s *ApplicationService) executeAndCompleteTurn(ctx context.Context, taskID int64, req *chatapi.WorkbenchChatRequest, mode ChatMode, message string) {
	result, err := s.executeTurn(ctx, taskID, req, mode, message)
	if err != nil {
		if failErr := s.failTurn(ctx, taskID, err.Error()); failErr != nil {
			logs.CtxErrorf(ctx, "workbench fail task %d failed after turn error %v: %v", taskID, err, failErr)
		}
		return
	}
	resultJSON, err := marshalResultPayload(result)
	if err != nil {
		if failErr := s.failTurn(ctx, taskID, err.Error()); failErr != nil {
			logs.CtxErrorf(ctx, "workbench fail task %d failed after marshal error %v: %v", taskID, err, failErr)
		}
		return
	}
	if err := s.completeTask(ctx, taskID, resultJSON); err != nil {
		logs.CtxErrorf(ctx, "workbench complete task %d failed: %v", taskID, err)
	}
}

func (s *ApplicationService) prepareTask(ctx context.Context, req *chatapi.WorkbenchChatRequest, message string) (*taskapi.ChatTask, error) {
	if req.IsSetTaskID() {
		task, err := s.getTask(ctx, req.GetTaskID())
		if err != nil {
			return nil, err
		}
		if task == nil {
			return nil, fmt.Errorf("task service returned empty task")
		}
		if task.SpaceID != 0 && task.SpaceID != req.SpaceID {
			return nil, InvalidArgumentErrorf("task %d does not belong to space %d", task.ID, req.SpaceID)
		}
		if err := s.appendTaskEvent(ctx, task.ID, "user.message", mustJSON(map[string]string{
			"message": message,
			"status":  "completed",
			"title":   "用户追问",
		})); err != nil {
			return nil, err
		}
		return task, nil
	}

	input, err := taskInputJSON(message, executionTypeFromMode(req.Mode), resultTypeFromMode(req.Mode))
	if err != nil {
		return nil, err
	}
	task, err := s.createRunningTask(ctx, &taskapi.CreateTaskRequest{
		SpaceID:        req.SpaceID,
		Title:          taskTitle(message),
		ConversationID: req.ConversationID,
		SkillID:        req.SelectedSkillID,
		Input:          &input,
	})
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task service returned empty task")
	}
	if err := s.createAgentThreadForTask(ctx, req, task, message); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ApplicationService) executeTurn(ctx context.Context, taskID int64, req *chatapi.WorkbenchChatRequest, mode ChatMode, message string) (resultPayload, error) {
	if mode == ChatModeAgent {
		return s.runAgent(ctx, s.agentRequestFromWorkbench(ctx, taskID, req, message))
	}

	result, err := s.runAnswer(ctx, answerRequest{
		taskID:    taskID,
		mode:      mode,
		spaceID:   req.SpaceID,
		message:   message,
		modelType: req.GetModelType(),
		modelName: strings.TrimSpace(req.GetModelName()),
		enableKbs: req.GetEnableKbs(),
	})
	if err != nil {
		return resultPayload{}, err
	}
	if err := s.appendTaskEvent(ctx, taskID, "answer.completed", mustJSON(map[string]string{
		"title":   "生成回答",
		"message": result.Message,
		"status":  "completed",
		"runtime": result.ExecutionType,
	})); err != nil {
		return resultPayload{}, err
	}
	return result, nil
}

func (s *ApplicationService) agentRequestFromWorkbench(ctx context.Context, taskID int64, req *chatapi.WorkbenchChatRequest, message string) agentRequest {
	userID := ""
	cozeUID := int64(0)
	if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
		cozeUID = *uid
		userID = strconv.FormatInt(*uid, 10)
	}

	return agentRequest{
		taskID:          taskID,
		spaceID:         req.SpaceID,
		conversationID:  req.GetConversationID(),
		modelID:         req.GetModelType(),
		userID:          userID,
		cozeUID:         cozeUID,
		message:         message,
		enableSkills:    req.GetEnableSkills(),
		enableMcp:       req.GetEnableMcp(),
		enableKbs:       req.GetEnableKbs(),
		enableDatabases: req.GetEnableDatabases(),
		runtimeSettings: req.GetRuntimeSettings(),
	}
}

func cloneWorkbenchChatRequest(req *chatapi.WorkbenchChatRequest) *chatapi.WorkbenchChatRequest {
	if req == nil {
		return nil
	}
	clone := *req
	if req.ConversationID != nil {
		v := *req.ConversationID
		clone.ConversationID = &v
	}
	if req.SelectedSkillID != nil {
		v := *req.SelectedSkillID
		clone.SelectedSkillID = &v
	}
	if req.TaskID != nil {
		v := *req.TaskID
		clone.TaskID = &v
	}
	if req.ModelType != nil {
		v := *req.ModelType
		clone.ModelType = &v
	}
	if req.ModelName != nil {
		v := *req.ModelName
		clone.ModelName = &v
	}
	if req.RuntimeSettings != nil {
		v := *req.RuntimeSettings
		clone.RuntimeSettings = &v
	}
	clone.EnableSkills = append([]string(nil), req.EnableSkills...)
	clone.EnableMcp = append([]string(nil), req.EnableMcp...)
	clone.EnableKbs = append([]string(nil), req.EnableKbs...)
	clone.EnableDatabases = append([]string(nil), req.EnableDatabases...)
	return &clone
}

func (s *ApplicationService) failTurn(ctx context.Context, taskID int64, errMsg string) error {
	appendErr := s.appendTaskEvent(ctx, taskID, "turn.failed", mustJSON(map[string]string{
		"title":  "执行失败",
		"status": "failed",
		"error":  errMsg,
	}))
	failErr := s.failTask(ctx, taskID, errMsg)
	if appendErr != nil {
		return appendErr
	}
	return failErr
}

func IsClientError(err error) bool {
	return isInvalidArgument(err) || appskill.IsClientError(err) || apptask.IsClientError(err)
}

type invalidArgumentError struct {
	msg string
}

func (e invalidArgumentError) Error() string {
	return e.msg
}

func InvalidArgumentErrorf(format string, args ...any) error {
	return invalidArgumentError{msg: fmt.Sprintf(format, args...)}
}

func isInvalidArgument(err error) bool {
	var target invalidArgumentError
	return errors.As(err, &target)
}

func chatModeFromAPI(mode chatapi.ChatMode) (ChatMode, error) {
	switch mode {
	case chatapi.ChatMode_Ask:
		return ChatModeAsk, nil
	case chatapi.ChatMode_Agent:
		return ChatModeAgent, nil
	case chatapi.ChatMode_Auto:
		return ChatModeAuto, nil
	default:
		return 0, InvalidArgumentErrorf("unsupported chat mode: %d", mode)
	}
}

func taskInputJSON(message, executionType, resultType string) (string, error) {
	payload := map[string]string{
		"message":        message,
		"execution_type": executionType,
		"result_type":    resultType,
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func executionTypeFromMode(mode chatapi.ChatMode) string {
	if mode == chatapi.ChatMode_Agent {
		return "Agent"
	}
	return "Ark"
}

func resultTypeFromMode(mode chatapi.ChatMode) string {
	if mode == chatapi.ChatMode_Agent {
		return resultTypeAgentTrace
	}
	return resultTypeAnswer
}

func taskTitle(message string) string {
	title := strings.TrimSpace(message)
	if title == "" {
		return "未命名任务"
	}
	const maxRunes = 40
	if utf8.RuneCountInString(title) <= maxRunes {
		return title
	}
	runes := []rune(title)
	return string(runes[:maxRunes])
}

func stringPtr(v string) *string {
	return &v
}

func optionalStringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

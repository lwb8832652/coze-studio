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
	"strings"
	"unicode/utf8"

	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	appskill "github.com/coze-dev/coze-studio/backend/application/skill"
	apptask "github.com/coze-dev/coze-studio/backend/application/task"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	agentrun "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/service"
)

const (
	answerAskDirect     = "已进入 Ask 快速问答模式。"
	answerAgentDirect   = "已进入 Agent 智能体模式。"
	answerChatDirect    = "已进入 Chat 直接问答模式。"
	answerSkillFallback = "未选择技能，已回退到 Chat 直接问答模式。"
)

type ApplicationService struct {
	skillSVC          *appskill.ApplicationService
	taskSVC           *apptask.ApplicationService
	knowledgeSVC      crossknowledge.Knowledge
	agentRunSVC       agentrun.Run
	chatModelProvider chatModelProvider
}

func (s *ApplicationService) HandleMessage(ctx context.Context, req *chatapi.WorkbenchChatRequest) (*chatapi.WorkbenchChatResponse, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("workbench chat request is required")
	}
	if strings.TrimSpace(req.Message) == "" {
		return nil, InvalidArgumentErrorf("message is required")
	}

	mode, err := chatModeFromAPI(req.Mode)
	if err != nil {
		return nil, err
	}
	intent := ResolveIntent(req)
	decision := DecideRoute(mode, intent)

	data := &chatapi.WorkbenchChatData{
		RouteTarget: routeTargetToAPI(decision.Target),
		Reason:      stringPtr(decision.Reason),
	}
	if req.IsSetConversationID() {
		conversationID := req.GetConversationID()
		data.ConversationID = &conversationID
	}

	switch decision.Target {
	case RouteChatDirect:
		data.Answer = stringPtr(chatDirectAnswer(mode))
	case RouteAgentEngine:
		data.Answer = stringPtr(answerAgentDirect)
	case RouteSkillEngine:
		answer, err := s.runSkill(ctx, req)
		if err != nil {
			return nil, err
		}
		data.Answer = stringPtr(answer)
		if !req.IsSetSelectedSkillID() {
			data.RouteTarget = chatapi.RouteTarget_ChatDirect
		}
	case RouteTaskEngine:
		created, err := s.createTask(ctx, req)
		if err != nil {
			return nil, err
		}
		data.Task = created
		data.Answer = stringPtr(fmt.Sprintf("已创建异步任务：%s", created.Title))
	}

	return &chatapi.WorkbenchChatResponse{
		Code: 0,
		Msg:  "success",
		Data: data,
	}, nil
}

func (s *ApplicationService) runSkill(ctx context.Context, req *chatapi.WorkbenchChatRequest) (string, error) {
	if !req.IsSetSelectedSkillID() {
		return answerSkillFallback, nil
	}
	if s == nil || s.skillSVC == nil {
		return "", fmt.Errorf("workbench skill service is not initialized")
	}
	input, err := messageInputJSON(req.Message)
	if err != nil {
		return "", err
	}
	resp, err := s.skillSVC.TestRunSkill(ctx, &skillapi.TestRunSkillRequest{
		SkillID: req.GetSelectedSkillID(),
		Input:   input,
	})
	if err != nil {
		return "", err
	}
	if resp != nil && resp.Data != nil && resp.Data.Output != nil && *resp.Data.Output != "" {
		return *resp.Data.Output, nil
	}
	return "技能执行完成，暂无输出。", nil
}

func (s *ApplicationService) createTask(ctx context.Context, req *chatapi.WorkbenchChatRequest) (*taskapi.ChatTask, error) {
	if s == nil || s.taskSVC == nil {
		return nil, fmt.Errorf("workbench task service is not initialized")
	}
	input, err := messageInputJSON(req.Message)
	if err != nil {
		return nil, err
	}
	createReq := &taskapi.CreateTaskRequest{
		SpaceID:        req.SpaceID,
		Title:          taskTitle(req.Message),
		ConversationID: req.ConversationID,
		SkillID:        req.SelectedSkillID,
		Input:          &input,
	}
	resp, err := s.taskSVC.CreateTask(ctx, createReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Data == nil {
		return nil, fmt.Errorf("task service returned empty response")
	}
	if s.taskSVC.DomainSVC == nil {
		return nil, fmt.Errorf("task domain service is not initialized")
	}
	queued, err := s.taskSVC.DomainSVC.Enqueue(ctx, resp.Data.ID)
	if err != nil {
		return nil, err
	}
	if queued != nil {
		resp.Data.Status = taskapi.TaskStatus_Queued
		resp.Data.Progress = queued.Progress
		resp.Data.UpdatedAt = queued.UpdatedAt
	}
	return resp.Data, nil
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

func routeTargetToAPI(target RouteTarget) chatapi.RouteTarget {
	switch target {
	case RouteAgentEngine:
		return chatapi.RouteTarget_AgentEngine
	case RouteSkillEngine:
		return chatapi.RouteTarget_SkillEngine
	case RouteTaskEngine:
		return chatapi.RouteTarget_TaskEngine
	default:
		return chatapi.RouteTarget_ChatDirect
	}
}

func chatDirectAnswer(mode ChatMode) string {
	if mode == ChatModeAsk {
		return answerAskDirect
	}
	return answerChatDirect
}

func messageInputJSON(message string) (string, error) {
	payload := map[string]string{"message": message}
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
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

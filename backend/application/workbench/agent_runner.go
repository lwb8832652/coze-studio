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
	"io"
	"strings"

	"github.com/coze-dev/coze-studio/backend/api/model/conversation/common"
	agentrunmodel "github.com/coze-dev/coze-studio/backend/crossdomain/agentrun/model"
	crossmessage "github.com/coze-dev/coze-studio/backend/crossdomain/message/model"
	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type agentRequest struct {
	taskID           int64
	spaceID          int64
	conversationID   int64
	sectionID        int64
	agentID          int64
	modelID          int64
	userID           string
	cozeUID          int64
	isDraft          bool
	version          string
	message          string
	preRetrieveTools []*agentrunmodel.Tool
	customVariables  map[string]string
	enableSkills     []string
	enableMcp        []string
	enableKbs        []string
	enableDatabases  []string
	runtimeSettings  string
}

func (s *ApplicationService) runAgent(ctx context.Context, req agentRequest) (resultPayload, error) {
	if s == nil || s.agentRunSVC == nil {
		return resultPayload{}, fmt.Errorf("workbench agent run service is not initialized")
	}
	if req.agentID == 0 {
		return resultPayload{}, fmt.Errorf("workbench agent_id is required")
	}
	if strings.TrimSpace(req.userID) == "" {
		return resultPayload{}, fmt.Errorf("workbench agent user_id is required")
	}
	if req.conversationID == 0 {
		return resultPayload{}, fmt.Errorf("workbench agent conversation_id is required")
	}
	if req.sectionID == 0 {
		return resultPayload{}, fmt.Errorf("workbench agent section_id is required")
	}

	ext := map[string]string{
		"workbench_task_id": fmt.Sprintf("%d", req.taskID),
		"enable_skills":     strings.Join(req.enableSkills, ","),
		"enable_mcp":        strings.Join(req.enableMcp, ","),
		"enable_kbs":        strings.Join(req.enableKbs, ","),
		"enable_databases":  strings.Join(req.enableDatabases, ","),
	}
	if runtimeSettings := strings.TrimSpace(req.runtimeSettings); runtimeSettings != "" {
		ext["runtime_settings"] = runtimeSettings
	}

	stream, err := s.agentRunSVC.AgentRun(ctx, &agentrunentity.AgentRunMeta{
		ConversationID: req.conversationID,
		ConnectorID:    consts.CozeConnectorID,
		SpaceID:        req.spaceID,
		Scene:          common.Scene_AgentAPP,
		SectionID:      req.sectionID,
		AgentID:        req.agentID,
		UserID:         req.userID,
		CozeUID:        req.cozeUID,
		IsDraft:        req.isDraft,
		Version:        req.version,
		CustomerConfig: agentCustomerConfig(req.modelID),
		ContentType:    crossmessage.ContentTypeText,
		Content: []*crossmessage.InputMetaData{
			{Type: crossmessage.InputTypeText, Text: req.message},
		},
		DisplayContent:   req.message,
		PreRetrieveTools: req.preRetrieveTools,
		CustomVariables:  req.customVariables,
		Ext:              ext,
	})
	if err != nil {
		return resultPayload{}, err
	}
	if stream == nil {
		return resultPayload{}, fmt.Errorf("workbench agent run returned empty stream")
	}

	finalAnswer := ""
	for {
		chunk, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			return resultPayload{}, recvErr
		}
		if chunk != nil && chunk.Event == agentrunentity.RunEventStreamDone {
			break
		}

		eventType, payload := agentChunkToTaskEvent(chunk)
		if eventType != "" {
			if err := s.appendTaskEvent(ctx, req.taskID, eventType, payload); err != nil {
				return resultPayload{}, err
			}
		}
		if chunk != nil && chunk.Event == agentrunentity.RunEventError {
			if chunk.Error != nil && strings.TrimSpace(chunk.Error.Msg) != "" {
				return resultPayload{}, fmt.Errorf("workbench agent run failed: %s", chunk.Error.Msg)
			}
			return resultPayload{}, fmt.Errorf("workbench agent run failed")
		}

		if isFinalAnswerChunk(chunk) {
			finalAnswer = strings.TrimSpace(chunk.ChunkMessageItem.Content)
		}
	}

	return resultPayload{
		Message:       finalAnswer,
		ResultType:    resultTypeAgentTrace,
		ExecutionType: executionTypeAgent,
	}, nil
}

func agentCustomerConfig(modelID int64) *agentrunentity.CustomerConfig {
	if modelID <= 0 {
		return nil
	}

	return &agentrunentity.CustomerConfig{
		ModelConfig: &agentrunentity.ModelConfig{
			ModelId: &modelID,
		},
	}
}

func agentChunkToTaskEvent(chunk *agentrunentity.AgentRunResponse) (string, string) {
	if chunk == nil {
		return "", ""
	}

	payload := map[string]string{
		"status": "running",
		"title":  string(chunk.Event),
	}
	switch chunk.Event {
	case agentrunentity.RunEventCreated:
		payload["title"] = "AgentRun 已启动"
		payload["status"] = "completed"
		return "agent.run_started", mustJSON(payload)
	case agentrunentity.RunEventMessageDelta:
		if chunk.ChunkMessageItem != nil {
			payload["title"] = "生成回答"
			payload["message"] = chunk.ChunkMessageItem.Content
		}
		return "agent.answer_delta", mustJSON(payload)
	case agentrunentity.RunEventMessageCompleted:
		if !isFinalAnswerChunk(chunk) {
			return "", ""
		}
		payload["title"] = "Agent 最终结果"
		payload["message"] = chunk.ChunkMessageItem.Content
		payload["message_type"] = string(chunk.ChunkMessageItem.MessageType)
		payload["status"] = "completed"
		return "agent.run_completed", mustJSON(payload)
	case agentrunentity.RunEventError:
		payload["title"] = "Agent 执行失败"
		payload["status"] = "failed"
		if chunk.Error != nil {
			payload["detail"] = chunk.Error.Msg
		}
		return "agent.run_failed", mustJSON(payload)
	default:
		return "", ""
	}
}

func isFinalAnswerChunk(chunk *agentrunentity.AgentRunResponse) bool {
	if chunk == nil || chunk.ChunkMessageItem == nil {
		return false
	}
	if chunk.Event != agentrunentity.RunEventMessageCompleted && !chunk.ChunkMessageItem.IsFinish {
		return false
	}
	switch chunk.ChunkMessageItem.MessageType {
	case crossmessage.MessageTypeAnswer, crossmessage.MessageTypeToolAsAnswer:
		return true
	default:
		return false
	}
}

func mustJSON(payload map[string]string) string {
	bytes, _ := json.Marshal(payload)
	return string(bytes)
}

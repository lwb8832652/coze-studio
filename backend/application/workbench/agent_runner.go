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
	crossmessage "github.com/coze-dev/coze-studio/backend/crossdomain/message/model"
	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type agentRequest struct {
	taskID          int64
	spaceID         int64
	conversationID  int64
	message         string
	enableSkills    []string
	enableMcp       []string
	enableKbs       []string
	enableDatabases []string
}

func (s *ApplicationService) runAgent(ctx context.Context, req agentRequest) (resultPayload, error) {
	if s == nil || s.agentRunSVC == nil {
		return resultPayload{}, fmt.Errorf("workbench agent run service is not initialized")
	}

	stream, err := s.agentRunSVC.AgentRun(ctx, &agentrunentity.AgentRunMeta{
		ConversationID: req.conversationID,
		ConnectorID:    consts.CozeConnectorID,
		SpaceID:        req.spaceID,
		Scene:          common.Scene_AgentAPP,
		ContentType:    crossmessage.ContentTypeText,
		Content: []*crossmessage.InputMetaData{
			{Type: crossmessage.InputTypeText, Text: req.message},
		},
		DisplayContent: req.message,
		Ext: map[string]string{
			"workbench_task_id": fmt.Sprintf("%d", req.taskID),
			"enable_skills":     strings.Join(req.enableSkills, ","),
			"enable_mcp":        strings.Join(req.enableMcp, ","),
			"enable_kbs":        strings.Join(req.enableKbs, ","),
			"enable_databases":  strings.Join(req.enableDatabases, ","),
		},
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

		if chunk != nil && chunk.ChunkMessageItem != nil &&
			(chunk.Event == agentrunentity.RunEventMessageCompleted || chunk.ChunkMessageItem.IsFinish) {
			finalAnswer = strings.TrimSpace(chunk.ChunkMessageItem.Content)
		}
	}

	return resultPayload{
		Message:       finalAnswer,
		ResultType:    resultTypeAgentTrace,
		ExecutionType: executionTypeAgent,
	}, nil
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
		if chunk.ChunkMessageItem != nil {
			payload["title"] = "Agent 最终结果"
			payload["message"] = chunk.ChunkMessageItem.Content
			payload["status"] = "completed"
		}
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

func mustJSON(payload map[string]string) string {
	bytes, _ := json.Marshal(payload)
	return string(bytes)
}

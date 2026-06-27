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
	"strconv"

	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
)

func (s *ApplicationService) createAgentThreadForTask(ctx context.Context, req *chatapi.WorkbenchChatRequest, task *taskapi.ChatTask, message string) error {
	if s == nil || s.agentThreadSVC == nil {
		return nil
	}

	userID := int64(0)
	if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
		userID = *uid
	}

	metadata, err := agentThreadMetadataJSON(req, message)
	if err != nil {
		return err
	}

	_, err = s.agentThreadSVC.CreateThread(ctx, &appagentthread.CreateThreadRequest{
		SpaceID:      req.SpaceID,
		UserID:       userID,
		Title:        task.Title,
		Source:       appagentthread.ThreadSourceWeb,
		LegacyTaskID: task.ID,
		Metadata:     metadata,
	})

	return err
}

func agentThreadMetadataJSON(req *chatapi.WorkbenchChatRequest, message string) (string, error) {
	payload := map[string]any{
		"message": message,
		"mode":    req.Mode.String(),
	}
	if req.ConversationID != nil {
		payload["conversation_id"] = strconv.FormatInt(*req.ConversationID, 10)
	}
	if req.SelectedSkillID != nil {
		payload["skill_id"] = strconv.FormatInt(*req.SelectedSkillID, 10)
	}
	if settings, ok, err := workbenchRuntimeSettingsPayload(req); err != nil {
		return "", err
	} else if ok {
		payload["runtime_settings"] = settings
	}

	return mustJSONAny(payload), nil
}

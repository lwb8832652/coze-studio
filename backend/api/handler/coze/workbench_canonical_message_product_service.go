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

package coze

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	threadproduct "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread_product_contract"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	appworkbench "github.com/coze-dev/coze-studio/backend/application/workbench"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	canonicalMessageAppendModeInternalCompat = "internal_compat"
	canonicalSuggestionMessageLimit          = 40
)

// AppendCanonicalThreadMessage serves POST /api/workbench/threads/:thread_id/messages.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and referenced Run. It calls
// ApplicationService.AppendMessage, and returns canonical message JSON.
func AppendCanonicalThreadMessage(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.message.append", "/api/workbench/threads/:thread_id/messages")
	requestLog.ResponseBodyKind = "values"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	var req threadproduct.AppendCanonicalThreadMessageRequest
	if public := rejectCanonicalBodyRouteIdentifiers(c); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	runID := req.GetRunID()
	if strings.TrimSpace(req.GetAppendMode()) != canonicalMessageAppendModeInternalCompat {
		public := canonicalMessageBadRequest(
			"invalid_append_mode",
			"append_mode must be internal_compat",
			"invalid_append_mode",
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if runID <= 0 {
		public := canonicalMessageBadRequest(
			"invalid_run_id",
			"run_id must be positive",
			"invalid_run_id",
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.RunID = runID

	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, runID)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, runID); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}

	role := strings.TrimSpace(strings.ToLower(req.GetRole()))
	switch role {
	case string(appagentthread.MessageRoleUser), "human":
		public := newCanonicalError(
			consts.StatusConflict,
			"atomic_run_submission_required",
			"User messages must be submitted atomically through POST /runs",
			"atomic_run_submission_required",
			false,
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	case string(appagentthread.MessageRoleAssistant), string(appagentthread.MessageRoleTool):
	default:
		public := canonicalMessageBadRequest(
			"invalid_message_role",
			"role must be assistant or tool",
			"invalid_message_role",
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	content := canonicalCleanString(req.GetContent(), canonicalMaxRunMessageBytes)
	if content == "" {
		public := canonicalMessageBadRequest(
			"invalid_message_content",
			"content is required",
			"invalid_message_content",
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	resp, err := appagentthread.SVC.AppendMessage(ctx, &appagentthread.AppendMessageRequest{
		ThreadID: threadID,
		RunID:    runID,
		Role:     appagentthread.MessageRole(role),
		Content:  content,
		Metadata: req.GetMetadata(),
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	message, err := projectCanonicalAppendMessage(resp)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if message == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("canonical appended message projection is empty"))
		return
	}
	requestLog.ResourceID = message.MessageID
	requestLog.LifecycleStage = "created"
	c.JSON(consts.StatusOK, message)
}

// GenerateCanonicalThreadSuggestions serves POST /api/workbench/threads/:thread_id/suggestions.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// Workbench ApplicationService.GenerateSuggestions, and returns suggestion-list JSON.
func GenerateCanonicalThreadSuggestions(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.suggestions.generate", "/api/workbench/threads/:thread_id/suggestions")
	requestLog.ResponseBodyKind = "values"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	var req threadproduct.GenerateCanonicalThreadSuggestionsRequest
	if public := rejectCanonicalBodyRouteIdentifiers(c); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}

	messages, err := loadCanonicalSuggestionMessages(ctx, threadID, canonicalSuggestionMessageLimit)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	resp, err := appworkbench.SVC.GenerateSuggestions(ctx, &appworkbench.GenerateSuggestionsRequest{
		Messages:  messages,
		N:         int(req.GetN()),
		ModelName: req.GetModelName(),
		ModelType: req.GetModelType(),
	})
	if err != nil {
		logs.CtxWarnf(ctx, "event_name=workbench.suggestions.failed suggestion_failure=%s", canonicalSuggestionFailureCategory(err))
		c.JSON(consts.StatusOK, &threadproduct.CanonicalSuggestionResponse{Suggestions: []string{}})
		return
	}
	if resp == nil {
		c.JSON(consts.StatusOK, &threadproduct.CanonicalSuggestionResponse{Suggestions: []string{}})
		return
	}
	requestLog.LifecycleStage = "completed"
	c.JSON(consts.StatusOK, &threadproduct.CanonicalSuggestionResponse{Suggestions: resp.Suggestions})
}

func canonicalProductThreadAccessContext(
	ctx context.Context,
	spaceID, threadID, runID int64,
) context.Context {
	return appagentthread.WithThreadAccessRequest(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		SpaceID:  spaceID,
		ThreadID: threadID,
		RunID:    runID,
	})
}

func authorizeCanonicalProductThreadAccess(
	ctx context.Context,
	spaceID, threadID, runID int64,
) error {
	return appagentthread.SVC.AuthorizeThreadAccess(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		SpaceID:  spaceID,
		ThreadID: threadID,
		RunID:    runID,
	})
}

func rejectCanonicalBodyRouteIdentifiers(c *app.RequestContext) *canonicalError {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		return nil
	}
	for _, field := range []string{"thread_id", "space_id"} {
		if _, ok := body[field]; ok {
			return newCanonicalError(
				consts.StatusUnprocessableEntity,
				"unsupported_sdk_field",
				"Unsupported field: "+field,
				"unsupported_field",
				false,
			)
		}
	}
	return nil
}

func projectCanonicalAppendMessage(resp *appagentthread.AppendMessageResponse) (*canonicalMessage, error) {
	if resp == nil || resp.Message == nil {
		return nil, fmt.Errorf("agent thread application returned empty message")
	}
	if resp.Message.Role == appagentthread.MessageRoleTool {
		if resp.Message.MessageID <= 0 || resp.Message.ThreadID <= 0 || resp.Message.RunID <= 0 {
			return nil, fmt.Errorf("canonical tool message projection requires positive message, thread, and run ids")
		}
		return &canonicalMessage{
			MessageID: fmt.Sprint(resp.Message.MessageID),
			ThreadID:  fmt.Sprint(resp.Message.ThreadID),
			RunID:     fmt.Sprint(resp.Message.RunID),
			Role:      string(resp.Message.Role),
			Content:   "",
			Metadata:  canonicalMetadataFromJSON(appagentthread.ProjectPublicMessageMetadataJSON(resp.Message.Metadata), ""),
			CreatedAt: canonicalTime(resp.Message.CreatedAt),
		}, nil
	}
	return projectCanonicalMessage(resp.Message)
}

func loadCanonicalSuggestionMessages(
	ctx context.Context,
	threadID int64,
	limit int32,
) ([]appworkbench.SuggestionMessage, error) {
	if limit <= 0 {
		return []appworkbench.SuggestionMessage{}, nil
	}
	resp, err := appagentthread.SVC.ListRecentPublicMessages(ctx, &appagentthread.ListRecentPublicMessagesRequest{
		ThreadID: threadID,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("agent thread application returned empty recent public messages")
	}
	result := make([]appworkbench.SuggestionMessage, 0, len(resp.Messages))
	for _, message := range resp.Messages {
		if message == nil {
			continue
		}
		content := canonicalCleanString(message.Content, canonicalMaxPublicValueRunes)
		if content == "" {
			continue
		}
		result = append(result, appworkbench.SuggestionMessage{
			Role:    string(message.Role),
			Content: content,
		})
	}
	return result, nil
}

func canonicalMessageBadRequest(code, detail, errorClass string) *canonicalError {
	return newCanonicalError(
		consts.StatusBadRequest,
		code,
		detail,
		errorClass,
		false,
	)
}

func canonicalSuggestionFailureCategory(error) string {
	return "provider_error"
}

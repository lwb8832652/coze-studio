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
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	langgraphapi "github.com/coze-dev/coze-studio/backend/api/model/agent/langgraph"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

// CreateLangGraphRun .
// @router /api/threads/:thread_id/runs [POST]
func CreateLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.CreateRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	if len(req.Input) == 0 {
		invalidParamRequestResponse(c, "input is required")
		return
	}

	input, err := langGraphMarshalJSON(req.Input, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	command, err := langGraphMarshalJSON(req.Command, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	metadata, err := langGraphMarshalJSON(req.Metadata, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	config, err := langGraphMarshalJSON(req.Config, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	runContext, err := langGraphMarshalJSON(req.Context, "{}")
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}
	streamMode, err := langGraphMarshalJSON(req.StreamMode, `["messages","updates"]`)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.CreateRun(ctx, &appagentthread.CreateRunRequest{
		ThreadID:          req.ThreadID,
		AssistantID:       req.AssistantID,
		Input:             input,
		Command:           command,
		Metadata:          metadata,
		Config:            config,
		Context:           runContext,
		StreamMode:        streamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

// ListLangGraphRuns .
// @router /api/threads/:thread_id/runs [GET]
func ListLangGraphRuns(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.ListRunsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var status *appagentthread.RunStatus
	if strings.TrimSpace(req.Status) != "" {
		mapped := appagentthread.RunStatus(req.Status)
		status = &mapped
	}
	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 20
	}
	page := int32(1)
	if req.Offset > 0 {
		page = req.Offset/pageSize + 1
	}

	resp, err := appagentthread.SVC.ListRuns(ctx, &appagentthread.ListRunsRequest{
		ThreadID: req.ThreadID,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphRunsToAPI(resp.Runs))
}

// GetLangGraphRun .
// @router /api/threads/:thread_id/runs/:run_id [GET]
func GetLangGraphRun(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.GetRunRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: req.RunID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if resp == nil || resp.Run == nil || resp.Run.ThreadID != req.ThreadID {
		invalidParamRequestResponse(c, "run_id does not belong to thread_id")
		return
	}

	c.JSON(consts.StatusOK, langGraphRunToAPI(resp.Run))
}

func langGraphRunsToAPI(runs []*appagentthread.RunSummary) []*langgraphapi.Run {
	result := make([]*langgraphapi.Run, 0, len(runs))
	for _, run := range runs {
		result = append(result, langGraphRunToAPI(run))
	}

	return result
}

func langGraphRunToAPI(run *appagentthread.RunSummary) *langgraphapi.Run {
	if run == nil {
		return nil
	}

	return &langgraphapi.Run{
		RunID:             strconv.FormatInt(run.RunID, 10),
		ThreadID:          strconv.FormatInt(run.ThreadID, 10),
		AssistantID:       run.AssistantID,
		Status:            string(run.Status),
		CreatedAt:         langGraphTime(run.CreatedAt),
		UpdatedAt:         langGraphTime(run.UpdatedAt),
		Metadata:          langGraphJSONMap(run.Metadata),
		Input:             langGraphJSONMap(run.Input),
		Command:           langGraphJSONMap(run.Command),
		Config:            langGraphJSONMap(run.Config),
		Context:           langGraphJSONMap(run.Context),
		StreamMode:        langGraphJSONStringSlice(run.StreamMode),
		MultitaskStrategy: run.MultitaskStrategy,
		OnDisconnect:      run.OnDisconnect,
		Durability:        run.Durability,
		Error:             run.ErrorMessage,
	}
}

func langGraphMarshalJSON(value any, defaultValue string) (string, error) {
	if value == nil {
		return defaultValue, nil
	}

	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return defaultValue, nil
		}
	case []string:
		if len(typed) == 0 {
			return defaultValue, nil
		}
	}

	return sonic.MarshalString(value)
}

func langGraphJSONMap(raw string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := sonic.UnmarshalString(raw, &result); err != nil {
		return map[string]any{}
	}

	return result
}

func langGraphJSONStringSlice(raw string) []string {
	result := []string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := sonic.UnmarshalString(raw, &result); err != nil {
		return []string{}
	}

	return result
}

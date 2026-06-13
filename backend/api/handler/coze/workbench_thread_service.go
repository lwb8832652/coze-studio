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

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	threadapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/thread"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

// ListTaskThreads .
// @router /api/workbench/task_threads [GET]
func ListTaskThreads(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	var status *appagentthread.ThreadStatus
	if req.Status != "" {
		mapped := appagentthread.ThreadStatus(req.Status)
		status = &mapped
	}
	resp, err := appagentthread.SVC.ListThreads(ctx, &appagentthread.ListThreadsRequest{
		SpaceID:  req.SpaceID,
		Status:   status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadsResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadsData{
			Threads: taskThreadsToAPI(resp.Threads),
			Total:   resp.Total,
		},
	})
}

// GetTaskThread .
// @router /api/workbench/task_threads/:thread_id [GET]
func GetTaskThread(ctx context.Context, c *app.RequestContext) {
	var req threadapi.GetTaskThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.GetTaskThreadResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadToAPI(resp.Thread),
	})
}

// ListTaskThreadMessages .
// @router /api/workbench/task_threads/:thread_id/messages [GET]
func ListTaskThreadMessages(ctx context.Context, c *app.RequestContext) {
	var req threadapi.ListTaskThreadMessagesRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.ListMessages(ctx, &appagentthread.ListMessagesRequest{
		ThreadID: req.ThreadID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.ListTaskThreadMessagesResponse{
		Code: 0,
		Msg:  "success",
		Data: &threadapi.ListTaskThreadMessagesData{
			Messages: taskThreadMessagesToAPI(resp.Messages),
			Total:    resp.Total,
		},
	})
}

// AppendTaskThreadMessage .
// @router /api/workbench/task_threads/:thread_id/messages [POST]
func AppendTaskThreadMessage(ctx context.Context, c *app.RequestContext) {
	var req threadapi.AppendTaskThreadMessageRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.AppendMessage(ctx, &appagentthread.AppendMessageRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Role:     appagentthread.MessageRole(req.Role),
		Content:  req.Content,
		Metadata: req.Metadata,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, &threadapi.AppendTaskThreadMessageResponse{
		Code: 0,
		Msg:  "success",
		Data: taskThreadMessageToAPI(resp.Message),
	})
}

func taskThreadsToAPI(threads []*appagentthread.ThreadSummary) []*threadapi.TaskThread {
	result := make([]*threadapi.TaskThread, 0, len(threads))
	for _, item := range threads {
		result = append(result, taskThreadToAPI(item))
	}

	return result
}

func taskThreadMessagesToAPI(messages []*appagentthread.MessageSummary) []*threadapi.TaskThreadMessage {
	result := make([]*threadapi.TaskThreadMessage, 0, len(messages))
	for _, item := range messages {
		result = append(result, taskThreadMessageToAPI(item))
	}

	return result
}

func taskThreadToAPI(thread *appagentthread.ThreadSummary) *threadapi.TaskThread {
	if thread == nil {
		return nil
	}

	return &threadapi.TaskThread{
		ThreadID:         thread.ThreadID,
		LegacyTaskID:     thread.LegacyTaskID,
		SpaceID:          thread.SpaceID,
		CreatorID:        thread.CreatorID,
		Title:            thread.Title,
		Status:           string(thread.Status),
		Source:           string(thread.Source),
		Progress:         thread.Progress,
		LastUserMessage:  thread.LastUserMessage,
		LastAgentMessage: thread.LastAgentMessage,
		CreatedAt:        thread.CreatedAt,
		UpdatedAt:        thread.UpdatedAt,
	}
}

func taskThreadMessageToAPI(message *appagentthread.MessageSummary) *threadapi.TaskThreadMessage {
	if message == nil {
		return nil
	}

	return &threadapi.TaskThreadMessage{
		MessageID: message.MessageID,
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Role:      string(message.Role),
		Content:   message.Content,
		Metadata:  message.Metadata,
		CreatedAt: message.CreatedAt,
	}
}

func workbenchThreadErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	internalServerErrorResponse(ctx, c, err)
}

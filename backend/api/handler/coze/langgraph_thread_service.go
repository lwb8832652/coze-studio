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
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"gorm.io/gorm"

	langgraphapi "github.com/coze-dev/coze-studio/backend/api/model/agent/langgraph"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

const defaultLangGraphThreadTitle = "新建任务"

// CreateLangGraphThread .
// @router /api/threads [POST]
func CreateLangGraphThread(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.CreateThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	metadata := normalizeLangGraphMetadata(req.Metadata)
	title := langGraphStringMetadata(metadata, "title")
	if title == "" {
		title = defaultLangGraphThreadTitle
		metadata["title"] = title
	}
	source := appagentthread.ThreadSource(langGraphStringMetadata(metadata, "source"))
	if source == "" {
		source = appagentthread.ThreadSourceAPI
		metadata["source"] = string(source)
	}

	metadataJSON, err := sonic.MarshalString(metadata)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.CreateThread(ctx, &appagentthread.CreateThreadRequest{
		SpaceID:  langGraphInt64Metadata(metadata, "space_id"),
		UserID:   langGraphInt64Metadata(metadata, "user_id", "creator_id"),
		Title:    title,
		Source:   source,
		Metadata: metadataJSON,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphThreadToAPI(resp.Thread))
}

// GetLangGraphThread .
// @router /api/threads/:thread_id [GET]
func GetLangGraphThread(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.GetThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	resp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphThreadToAPI(resp.Thread))
}

// SearchLangGraphThreads .
// @router /api/threads/search [POST]
func SearchLangGraphThreads(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.SearchThreadsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	metadata := normalizeLangGraphMetadata(req.Metadata)
	spaceID := langGraphInt64Metadata(metadata, "space_id")
	if spaceID <= 0 {
		invalidParamRequestResponse(c, "metadata.space_id is required")
		return
	}

	var status *appagentthread.ThreadStatus
	if strings.TrimSpace(req.Status) != "" {
		mapped := appagentthread.ThreadStatus(req.Status)
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

	resp, err := appagentthread.SVC.ListThreads(ctx, &appagentthread.ListThreadsRequest{
		SpaceID:  spaceID,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphThreadsToAPI(resp.Threads))
}

// GetLangGraphThreadState .
// @router /api/threads/:thread_id/state [GET]
func GetLangGraphThreadState(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.GetThreadStateRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	threadResp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if threadResp == nil || threadResp.Thread == nil {
		invalidParamRequestResponse(c, "thread_id is invalid")
		return
	}

	state, err := buildLangGraphThreadState(ctx, threadResp.Thread)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, state)
}

// GetLangGraphThreadHistory .
// @router /api/threads/:thread_id/history [GET]
func GetLangGraphThreadHistory(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.GetThreadHistoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}

	threadResp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if threadResp == nil || threadResp.Thread == nil {
		invalidParamRequestResponse(c, "thread_id is invalid")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	checkpointLimit := limit + req.Offset
	if checkpointLimit <= 0 || checkpointLimit > 100 {
		checkpointLimit = 100
	}
	checkpointsResp, err := appagentthread.SVC.ListCheckpoints(ctx, &appagentthread.ListCheckpointsRequest{
		ThreadID: req.ThreadID,
		Limit:    checkpointLimit,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if checkpointsResp != nil && checkpointsResp.Total > 0 {
		checkpoints := checkpointsResp.Checkpoints
		if req.Offset > 0 {
			if req.Offset >= int32(len(checkpoints)) {
				c.JSON(consts.StatusOK, []*langgraphapi.ThreadState{})
				return
			}
			checkpoints = checkpoints[req.Offset:]
		}
		if int32(len(checkpoints)) > limit {
			checkpoints = checkpoints[:limit]
		}
		c.JSON(consts.StatusOK, langGraphThreadHistoryFromCheckpoints(threadResp.Thread, checkpoints))
		return
	}

	page := int32(1)
	if req.Offset > 0 {
		page = req.Offset/limit + 1
	}

	eventsResp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
		ThreadID: req.ThreadID,
		Page:     page,
		PageSize: limit,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphThreadHistoryFromEvents(eventsResp.Events))
}

func langGraphThreadsToAPI(threads []*appagentthread.ThreadSummary) []*langgraphapi.Thread {
	result := make([]*langgraphapi.Thread, 0, len(threads))
	for _, thread := range threads {
		result = append(result, langGraphThreadToAPI(thread))
	}

	return result
}

func buildLangGraphThreadState(ctx context.Context, thread *appagentthread.ThreadSummary) (*langgraphapi.ThreadState, error) {
	if thread == nil {
		return nil, nil
	}

	checkpointResp, err := appagentthread.SVC.GetLatestCheckpoint(ctx, &appagentthread.GetLatestCheckpointRequest{
		ThreadID: thread.ThreadID,
	})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if checkpointResp != nil && checkpointResp.Checkpoint != nil {
		return langGraphThreadStateFromCheckpoint(thread, checkpointResp.Checkpoint), nil
	}

	messagesResp, err := appagentthread.SVC.ListMessages(ctx, &appagentthread.ListMessagesRequest{
		ThreadID: thread.ThreadID,
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		return nil, err
	}

	return &langgraphapi.ThreadState{
		Values: langGraphThreadStateValues(messagesResp.Messages),
		Next:   []string{},
		Config: langGraphThreadStateConfig(thread.ThreadID, "thread-"+strconv.FormatInt(thread.ThreadID, 10)+"-latest"),
		Metadata: langGraphThreadStateMetadata(
			thread,
			map[string]any{
				"checkpoint_source": "thread_snapshot",
			},
		),
		CreatedAt: langGraphTime(thread.CreatedAt),
		UpdatedAt: langGraphTime(thread.UpdatedAt),
	}, nil
}

func langGraphThreadHistoryFromCheckpoints(
	thread *appagentthread.ThreadSummary,
	checkpoints []*appagentthread.CheckpointSummary,
) []*langgraphapi.ThreadState {
	result := make([]*langgraphapi.ThreadState, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if checkpoint == nil {
			continue
		}
		result = append(result, langGraphThreadStateFromCheckpoint(thread, checkpoint))
	}

	return result
}

func langGraphThreadStateFromCheckpoint(
	thread *appagentthread.ThreadSummary,
	checkpoint *appagentthread.CheckpointSummary,
) *langgraphapi.ThreadState {
	values := langGraphCheckpointValues(checkpoint.ChannelValues)
	metadata := langGraphThreadStateMetadata(
		thread,
		langGraphCheckpointMetadata(checkpoint),
	)

	return &langgraphapi.ThreadState{
		Values: values,
		Next:   langGraphCheckpointNext(checkpoint.PendingSends, values),
		Config: langGraphThreadStateConfig(
			checkpoint.ThreadID,
			strconv.FormatInt(checkpoint.CheckpointID, 10),
			checkpoint.CheckpointNS,
		),
		Metadata:  metadata,
		CreatedAt: langGraphTime(checkpoint.CreatedAt),
		UpdatedAt: langGraphTime(checkpoint.CreatedAt),
	}
}

func langGraphThreadHistoryFromEvents(events []*appagentthread.RunEventSummary) []*langgraphapi.ThreadState {
	result := make([]*langgraphapi.ThreadState, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		checkpointID := "event-" + strconv.FormatInt(event.EventID, 10)
		result = append(result, &langgraphapi.ThreadState{
			Values: map[string]any{
				"messages":     []map[string]any{},
				"artifacts":    map[string]any{},
				"todos":        []any{},
				"memory":       map[string]any{},
				"tool_results": map[string]any{},
				"event":        langGraphRunEventPayload(event.Payload),
			},
			Next:   []string{},
			Config: langGraphThreadStateConfig(event.ThreadID, checkpointID),
			Metadata: map[string]any{
				"thread_id":  strconv.FormatInt(event.ThreadID, 10),
				"run_id":     strconv.FormatInt(event.RunID, 10),
				"event_id":   strconv.FormatInt(event.EventID, 10),
				"event_type": event.EventType,
				"source":     "event_log",
			},
			CreatedAt: langGraphTime(event.CreatedAt),
		})
	}

	return result
}

func langGraphCheckpointValues(raw string) map[string]any {
	values := langGraphRunEventPayloadMap(raw)
	defaults := langGraphThreadStateValues(nil)
	for key, value := range defaults {
		if _, ok := values[key]; !ok {
			values[key] = value
		}
	}

	return values
}

func langGraphCheckpointMetadata(checkpoint *appagentthread.CheckpointSummary) map[string]any {
	metadata := langGraphJSONMap(checkpoint.Metadata)
	metadata["thread_id"] = strconv.FormatInt(checkpoint.ThreadID, 10)
	metadata["run_id"] = strconv.FormatInt(checkpoint.RunID, 10)
	metadata["checkpoint_id"] = strconv.FormatInt(checkpoint.CheckpointID, 10)
	metadata["checkpoint_ns"] = checkpoint.CheckpointNS
	metadata["checkpoint_source"] = "checkpoint"
	if checkpoint.ParentCheckpointID > 0 {
		metadata["parent_checkpoint_id"] = strconv.FormatInt(checkpoint.ParentCheckpointID, 10)
	}
	if strings.TrimSpace(checkpoint.ChannelVersions) != "" {
		metadata["channel_versions"] = langGraphRunEventPayload(checkpoint.ChannelVersions)
	}

	return metadata
}

func langGraphCheckpointNext(raw string, values map[string]any) []string {
	next := langGraphStringSliceValue(langGraphRunEventPayload(raw))
	if len(next) > 0 {
		return next
	}

	return langGraphStringSliceValue(values["next"])
}

func langGraphStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return compactLangGraphStreamModes(typed)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			switch candidate := item.(type) {
			case string:
				if candidate = strings.TrimSpace(candidate); candidate != "" {
					result = append(result, candidate)
				}
			case map[string]any:
				for _, key := range []string{"node", "target", "name"} {
					if candidate := langGraphStringValue(candidate[key]); candidate != "" {
						result = append(result, candidate)
						break
					}
				}
			}
		}
		return result
	default:
		return []string{}
	}
}

func langGraphThreadStateValues(messages []*appagentthread.MessageSummary) map[string]any {
	result := map[string]any{
		"messages":     langGraphThreadStateMessages(messages),
		"artifacts":    map[string]any{},
		"todos":        []any{},
		"memory":       map[string]any{},
		"tool_results": map[string]any{},
	}

	return result
}

func langGraphThreadStateMessages(messages []*appagentthread.MessageSummary) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, map[string]any{
			"id":         strconv.FormatInt(message.MessageID, 10),
			"thread_id":  strconv.FormatInt(message.ThreadID, 10),
			"run_id":     strconv.FormatInt(message.RunID, 10),
			"role":       string(message.Role),
			"content":    message.Content,
			"metadata":   langGraphJSONMap(message.Metadata),
			"created_at": langGraphTime(message.CreatedAt),
		})
	}

	return result
}

func langGraphThreadStateConfig(threadID int64, checkpointID string, checkpointNS ...string) map[string]any {
	ns := ""
	if len(checkpointNS) > 0 {
		ns = checkpointNS[0]
	}

	return map[string]any{
		"configurable": map[string]any{
			"thread_id":     strconv.FormatInt(threadID, 10),
			"checkpoint_id": checkpointID,
			"checkpoint_ns": ns,
		},
	}
}

func langGraphThreadStateMetadata(thread *appagentthread.ThreadSummary, extra map[string]any) map[string]any {
	metadata := langGraphThreadMetadata(thread)
	for key, value := range extra {
		metadata[key] = value
	}

	return metadata
}

func langGraphThreadToAPI(thread *appagentthread.ThreadSummary) *langgraphapi.Thread {
	if thread == nil {
		return nil
	}

	return &langgraphapi.Thread{
		ThreadID:  strconv.FormatInt(thread.ThreadID, 10),
		CreatedAt: langGraphTime(thread.CreatedAt),
		UpdatedAt: langGraphTime(thread.UpdatedAt),
		Metadata:  langGraphThreadMetadata(thread),
		Status:    string(thread.Status),
		Values: &langgraphapi.ThreadValues{
			Messages: []map[string]any{},
		},
	}
}

func langGraphThreadMetadata(thread *appagentthread.ThreadSummary) map[string]any {
	metadata := map[string]any{}
	if thread.Metadata != "" {
		_ = sonic.UnmarshalString(thread.Metadata, &metadata)
	}
	if _, ok := metadata["space_id"]; !ok && thread.SpaceID > 0 {
		metadata["space_id"] = strconv.FormatInt(thread.SpaceID, 10)
	}
	if _, ok := metadata["user_id"]; !ok && thread.CreatorID > 0 {
		metadata["user_id"] = strconv.FormatInt(thread.CreatorID, 10)
	}
	if _, ok := metadata["title"]; !ok && thread.Title != "" {
		metadata["title"] = thread.Title
	}
	if _, ok := metadata["source"]; !ok && thread.Source != "" {
		metadata["source"] = string(thread.Source)
	}

	return metadata
}

func normalizeLangGraphMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}

	result := make(map[string]any, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}

	return result
}

func langGraphStringMetadata(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}

	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func langGraphInt64Metadata(metadata map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok {
			continue
		}
		if parsed := langGraphInt64Value(value); parsed > 0 {
			return parsed
		}
	}

	return 0
}

func langGraphInt64Value(value any) int64 {
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		parsed, _ := strconv.ParseInt(typed.String(), 10, 64)
		return parsed
	default:
		return 0
	}
}

func langGraphTime(ms int64) string {
	if ms <= 0 {
		return ""
	}

	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

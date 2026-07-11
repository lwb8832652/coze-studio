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
	"sort"
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
	ctx = workbenchThreadAccessContext(ctx, 0, 0)

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
	metadata["user_id"] = strconv.FormatInt(workbenchViewerIDFromCtx(ctx), 10)
	delete(metadata, "creator_id")

	metadataJSON, err := sonic.MarshalString(metadata)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.CreateThread(ctx, &appagentthread.CreateThreadRequest{
		SpaceID:  langGraphInt64Metadata(metadata, "space_id"),
		UserID:   workbenchViewerIDFromCtx(ctx),
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	resp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, langGraphThreadToAPI(resp.Thread))
}

// PatchLangGraphThread .
// @router /api/threads/:thread_id [PATCH]
func PatchLangGraphThread(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.PatchThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	threadResp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if threadResp == nil || threadResp.Thread == nil {
		c.JSON(consts.StatusNotFound, map[string]any{
			"detail": "Thread " + strconv.FormatInt(req.ThreadID, 10) + " not found",
		})
		return
	}

	metadata := langGraphStoredThreadMetadata(threadResp.Thread)
	for key, value := range stripLangGraphPatchMetadata(req.Metadata) {
		metadata[key] = value
	}
	metadataJSON, err := sonic.MarshalString(metadata)
	if err != nil {
		internalServerErrorResponse(ctx, c, err)
		return
	}

	updateResp, err := appagentthread.SVC.UpdateThreadMetadata(ctx, &appagentthread.UpdateThreadMetadataRequest{
		ThreadID: req.ThreadID,
		Metadata: metadataJSON,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if updateResp == nil || !updateResp.Updated || updateResp.Thread == nil {
		c.JSON(consts.StatusNotFound, map[string]any{
			"detail": "Thread " + strconv.FormatInt(req.ThreadID, 10) + " not found",
		})
		return
	}

	c.JSON(consts.StatusOK, langGraphThreadToAPI(updateResp.Thread))
}

// DeleteLangGraphThread .
// @router /api/threads/:thread_id [DELETE]
func DeleteLangGraphThread(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.DeleteThreadRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	deleteResp, err := appagentthread.SVC.DeleteThread(ctx, &appagentthread.DeleteThreadRequest{
		ThreadID: req.ThreadID,
	})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if deleteResp == nil || !deleteResp.Deleted {
		c.JSON(consts.StatusNotFound, map[string]any{
			"detail": "Thread " + strconv.FormatInt(req.ThreadID, 10) + " not found",
		})
		return
	}

	threadID := strconv.FormatInt(req.ThreadID, 10)
	c.JSON(consts.StatusOK, &langgraphapi.ThreadDeleteResponse{
		Success: true,
		Message: "Deleted local thread data for " + threadID,
	})
}

// SearchLangGraphThreads .
// @router /api/threads/search [POST]
func SearchLangGraphThreads(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.SearchThreadsRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, 0)

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
		UserID:   workbenchViewerIDFromCtx(ctx),
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

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

// PostLangGraphThreadState .
// @router /api/threads/:thread_id/state [POST]
func PostLangGraphThreadState(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.PostThreadStateRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	threadResp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: req.ThreadID})
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if threadResp == nil || threadResp.Thread == nil {
		invalidParamRequestResponse(c, "thread_id is invalid")
		return
	}

	state, err := buildLangGraphThreadStateUpdate(ctx, threadResp.Thread, &req)
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
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	thread, err := getLangGraphHistoryThread(ctx, req.ThreadID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if thread == nil {
		invalidParamRequestResponse(c, "thread_id is invalid")
		return
	}

	history, err := buildLangGraphThreadHistory(ctx, thread, req.Limit, req.Offset)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, history)
}

// PostLangGraphThreadHistory .
// @router /api/threads/:thread_id/history [POST]
func PostLangGraphThreadHistory(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.PostThreadHistoryRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	thread, err := getLangGraphHistoryThread(ctx, req.ThreadID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if thread == nil {
		invalidParamRequestResponse(c, "thread_id is invalid")
		return
	}

	history, err := buildLangGraphThreadHistoryEntries(ctx, thread, req.Limit, req.Before)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}

	c.JSON(consts.StatusOK, history)
}

// GetLangGraphCheckpointResumeReadiness .
// @router /api/threads/:thread_id/checkpoints/:checkpoint_id/resume [GET]
func GetLangGraphCheckpointResumeReadiness(ctx context.Context, c *app.RequestContext) {
	var req langgraphapi.GetCheckpointResumeRequest
	if err := c.BindAndValidate(&req); err != nil {
		invalidParamRequestResponse(c, err.Error())
		return
	}
	ctx = workbenchThreadAccessContext(ctx, req.ThreadID, 0)

	readiness, err := buildLangGraphCheckpointResumeReadiness(ctx, req.ThreadID, req.CheckpointID)
	if err != nil {
		workbenchThreadErrorResponse(ctx, c, err)
		return
	}
	if readiness == nil {
		invalidParamRequestResponse(c, "checkpoint_id does not belong to thread_id")
		return
	}

	c.JSON(consts.StatusOK, readiness)
}

func langGraphThreadsToAPI(threads []*appagentthread.ThreadSummary) []*langgraphapi.Thread {
	result := make([]*langgraphapi.Thread, 0, len(threads))
	for _, thread := range threads {
		result = append(result, langGraphThreadToAPI(thread))
	}

	return result
}

func buildLangGraphCheckpointResumeReadiness(
	ctx context.Context,
	threadID int64,
	checkpointID int64,
) (*langgraphapi.CheckpointResumeReadiness, error) {
	checkpointResp, err := appagentthread.SVC.GetCheckpoint(ctx, &appagentthread.GetCheckpointRequest{
		CheckpointID: checkpointID,
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}

		return nil, err
	}
	if checkpointResp == nil || checkpointResp.Checkpoint == nil || checkpointResp.Checkpoint.ThreadID != threadID {
		return nil, nil
	}

	return langGraphCheckpointResumeReadiness(checkpointResp.Checkpoint), nil
}

func getLangGraphHistoryThread(ctx context.Context, threadID int64) (*appagentthread.ThreadSummary, error) {
	threadResp, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: threadID})
	if err != nil {
		return nil, err
	}
	if threadResp == nil || threadResp.Thread == nil {
		return nil, nil
	}

	return threadResp.Thread, nil
}

func buildLangGraphThreadHistory(
	ctx context.Context,
	thread *appagentthread.ThreadSummary,
	limit int32,
	offset int32,
) ([]*langgraphapi.ThreadState, error) {
	limit = normalizeLangGraphHistoryLimit(limit)
	checkpointLimit := limit + offset
	if checkpointLimit <= 0 || checkpointLimit > 100 {
		checkpointLimit = 100
	}
	checkpointsResp, err := appagentthread.SVC.ListCheckpoints(ctx, &appagentthread.ListCheckpointsRequest{
		ThreadID: thread.ThreadID,
		Limit:    checkpointLimit,
	})
	if err != nil {
		return nil, err
	}
	if checkpointsResp != nil && checkpointsResp.Total > 0 {
		checkpoints := checkpointsResp.Checkpoints
		if offset > 0 {
			if offset >= int32(len(checkpoints)) {
				return []*langgraphapi.ThreadState{}, nil
			}
			checkpoints = checkpoints[offset:]
		}
		if int32(len(checkpoints)) > limit {
			checkpoints = checkpoints[:limit]
		}

		return langGraphThreadHistoryFromCheckpoints(thread, checkpoints), nil
	}

	page := int32(1)
	if offset > 0 {
		page = offset/limit + 1
	}

	eventsResp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
		ThreadID: thread.ThreadID,
		Page:     page,
		PageSize: limit,
	})
	if err != nil {
		return nil, err
	}

	return langGraphThreadHistoryFromEvents(eventsResp.Events), nil
}

func buildLangGraphThreadHistoryEntries(
	ctx context.Context,
	thread *appagentthread.ThreadSummary,
	limit int32,
	before string,
) ([]*langgraphapi.ThreadHistoryEntry, error) {
	limit = normalizeLangGraphHistoryLimit(limit)
	checkpointLimit := limit
	if strings.TrimSpace(before) != "" {
		checkpointLimit = 100
	}
	checkpointsResp, err := appagentthread.SVC.ListCheckpoints(ctx, &appagentthread.ListCheckpointsRequest{
		ThreadID: thread.ThreadID,
		Limit:    checkpointLimit,
	})
	if err != nil {
		return nil, err
	}
	if checkpointsResp != nil && checkpointsResp.Total > 0 {
		checkpoints := langGraphCheckpointsAfterCursor(checkpointsResp.Checkpoints, before)
		if int32(len(checkpoints)) > limit {
			checkpoints = checkpoints[:limit]
		}

		return langGraphThreadHistoryEntriesFromCheckpoints(thread, checkpoints), nil
	}

	eventsResp, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
		ThreadID: thread.ThreadID,
		Page:     1,
		PageSize: limit,
	})
	if err != nil {
		return nil, err
	}

	return langGraphThreadHistoryEntriesFromEvents(eventsResp.Events), nil
}

func normalizeLangGraphHistoryLimit(limit int32) int32 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}

	return limit
}

func langGraphCheckpointsAfterCursor(
	checkpoints []*appagentthread.CheckpointSummary,
	before string,
) []*appagentthread.CheckpointSummary {
	before = strings.TrimSpace(before)
	if before == "" {
		return checkpoints
	}
	for idx, checkpoint := range checkpoints {
		if checkpoint == nil {
			continue
		}
		if strconv.FormatInt(checkpoint.CheckpointID, 10) == before {
			if idx+1 >= len(checkpoints) {
				return []*appagentthread.CheckpointSummary{}
			}

			return checkpoints[idx+1:]
		}
	}

	return []*appagentthread.CheckpointSummary{}
}

func langGraphCheckpointResumeReadiness(checkpoint *appagentthread.CheckpointSummary) *langgraphapi.CheckpointResumeReadiness {
	if checkpoint == nil {
		return nil
	}

	metadata := langGraphCheckpointMetadata(checkpoint)
	if strings.TrimSpace(checkpoint.RuntimeType) == string(appagentthread.RuntimeModeEinoADK) {
		return langGraphADKCheckpointResumeReadiness(checkpoint, metadata)
	}

	projected := appagentthread.ProjectPublicCheckpoint(checkpoint)
	values := langGraphPublicCheckpointValues(projected)
	next := langGraphPublicCheckpointNext(projected, values)
	status := langGraphStringValue(metadata["status"])
	errorType := langGraphStringValue(metadata["error_type"])
	reason := langGraphCheckpointResumeReason(metadata, status, next)
	resumable := reason == "pending_sends_available"
	resumeFrom := ""
	if resumable {
		resumeFrom = "pending_sends"
	}

	return &langgraphapi.CheckpointResumeReadiness{
		ThreadID:     strconv.FormatInt(checkpoint.ThreadID, 10),
		RunID:        strconv.FormatInt(checkpoint.RunID, 10),
		CheckpointID: strconv.FormatInt(checkpoint.CheckpointID, 10),
		CheckpointNS: checkpoint.CheckpointNS,
		Resumable:    resumable,
		ResumeFrom:   resumeFrom,
		Reason:       reason,
		Status:       status,
		ErrorType:    errorType,
		PendingSends: next,
		Config: langGraphThreadStateConfig(
			checkpoint.ThreadID,
			strconv.FormatInt(checkpoint.CheckpointID, 10),
			checkpoint.CheckpointNS,
		),
		Metadata: metadata,
	}
}

func langGraphADKCheckpointResumeReadiness(
	checkpoint *appagentthread.CheckpointSummary,
	metadata map[string]any,
) *langgraphapi.CheckpointResumeReadiness {
	reason := "interrupts_available"
	resumable := checkpoint.RuntimeDeletedAt == 0
	interruptIDs := []string{}
	if projected := appagentthread.ProjectPublicCheckpoint(checkpoint); projected != nil {
		interruptIDs = append(interruptIDs, projected.InterruptIDs...)
	}
	_, err := appagentthread.UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
	if err != nil {
		reason = "invalid_adk_checkpoint"
		resumable = false
	} else {
		if len(interruptIDs) == 0 {
			reason = "no_interrupts"
			resumable = false
		}
	}
	if checkpoint.RuntimeDeletedAt > 0 {
		reason = "checkpoint_deleted"
		resumable = false
	}

	resumeFrom := ""
	if resumable {
		resumeFrom = "interrupt"
	}

	return &langgraphapi.CheckpointResumeReadiness{
		ThreadID:     strconv.FormatInt(checkpoint.ThreadID, 10),
		RunID:        strconv.FormatInt(checkpoint.RunID, 10),
		CheckpointID: strconv.FormatInt(checkpoint.CheckpointID, 10),
		CheckpointNS: checkpoint.CheckpointNS,
		Resumable:    resumable,
		ResumeFrom:   resumeFrom,
		Reason:       reason,
		PendingSends: interruptIDs,
		Config: langGraphThreadStateConfig(
			checkpoint.ThreadID,
			strconv.FormatInt(checkpoint.CheckpointID, 10),
			checkpoint.CheckpointNS,
		),
		Metadata: metadata,
	}
}

func langGraphADKInterruptIDs(interrupts map[string]appagentthread.ADKInterruptItem) []string {
	ids := make([]string, 0, len(interrupts))
	for key, item := range interrupts {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(key)
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	return ids
}

func langGraphCheckpointResumeReason(metadata map[string]any, status string, pendingSends []string) string {
	if source := langGraphStringValue(metadata["source"]); source != "agent_harness" {
		return "unsupported_checkpoint_source"
	}
	if phase := langGraphStringValue(metadata["checkpoint_phase"]); phase != "terminal" {
		return "checkpoint_not_terminal"
	}
	if status == "succeeded" {
		return "checkpoint_already_succeeded"
	}
	if len(pendingSends) == 0 {
		return "no_pending_sends"
	}
	if status == "failed" || status == "canceled" {
		return "pending_sends_available"
	}

	return "unsupported_checkpoint_status"
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

func buildLangGraphThreadStateUpdate(
	ctx context.Context,
	thread *appagentthread.ThreadSummary,
	req *langgraphapi.PostThreadStateRequest,
) (*langgraphapi.ThreadState, error) {
	if thread == nil || req == nil {
		return nil, errors.New("thread state update request is required")
	}

	base, err := langGraphThreadStateUpdateBaseCheckpoint(ctx, thread.ThreadID, req.CheckpointID)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return nil, errors.New("thread checkpoint is required")
	}
	if base.ThreadID != thread.ThreadID {
		return nil, errors.New("checkpoint does not belong to thread")
	}
	if strings.TrimSpace(base.RuntimeType) == string(appagentthread.RuntimeModeEinoADK) {
		return nil, errors.New("eino adk checkpoint state update is not supported")
	}

	values := langGraphMergeChannelValues(langGraphInternalCheckpointValues(base), req.Values)
	channelValues, err := sonic.MarshalString(values)
	if err != nil {
		return nil, err
	}
	channelVersions, err := sonic.MarshalString(langGraphUpdatedChannelVersions(base.ChannelVersions, req.Values))
	if err != nil {
		return nil, err
	}
	metadata, err := sonic.MarshalString(langGraphThreadStateUpdateMetadata(base.Metadata, req.Values, req.AsNode))
	if err != nil {
		return nil, err
	}

	created, err := appagentthread.SVC.CreateCheckpoint(ctx, &appagentthread.CreateCheckpointRequest{
		ThreadID:           thread.ThreadID,
		RunID:              base.RunID,
		ParentCheckpointID: base.CheckpointID,
		CheckpointNS:       base.CheckpointNS,
		RuntimeType:        base.RuntimeType,
		RuntimeKey:         base.RuntimeKey,
		EnvelopeVersion:    base.EnvelopeVersion,
		ChannelValues:      channelValues,
		ChannelVersions:    channelVersions,
		PendingSends:       base.PendingSends,
		Metadata:           metadata,
	})
	if err != nil {
		return nil, err
	}
	updatedThread := thread
	if title := strings.TrimSpace(langGraphStringValue(req.Values["title"])); title != "" {
		updatedResp, updateErr := appagentthread.SVC.UpdateThreadTitle(ctx, &appagentthread.UpdateThreadTitleRequest{
			ThreadID: thread.ThreadID,
			Title:    title,
		})
		if updateErr != nil {
			return nil, updateErr
		}
		if updatedResp != nil && updatedResp.Thread != nil {
			updatedThread = updatedResp.Thread
		}
	}

	return langGraphThreadStateFromCheckpoint(updatedThread, created.Checkpoint), nil
}

func langGraphThreadStateUpdateBaseCheckpoint(
	ctx context.Context,
	threadID int64,
	checkpointID string,
) (*appagentthread.CheckpointSummary, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID != "" {
		id, err := strconv.ParseInt(checkpointID, 10, 64)
		if err != nil {
			return nil, err
		}
		resp, err := appagentthread.SVC.GetCheckpoint(ctx, &appagentthread.GetCheckpointRequest{CheckpointID: id})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, nil
		}

		return resp.Checkpoint, nil
	}

	resp, err := appagentthread.SVC.GetLatestCheckpoint(ctx, &appagentthread.GetLatestCheckpointRequest{ThreadID: threadID})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	return resp.Checkpoint, nil
}

func langGraphMergeChannelValues(base map[string]any, updates map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range updates {
		merged[key] = value
	}

	return merged
}

func langGraphUpdatedChannelVersions(raw string, updates map[string]any) map[string]any {
	versions := langGraphRunEventPayloadMap(raw)
	for key := range updates {
		current := int64(0)
		switch typed := versions[key].(type) {
		case float64:
			current = int64(typed)
		case int64:
			current = typed
		case int:
			current = int64(typed)
		}
		versions[key] = current + 1
	}

	return versions
}

func langGraphThreadStateUpdateMetadata(raw string, values map[string]any, asNode string) map[string]any {
	metadata := langGraphRunEventPayloadMap(raw)
	metadata["updated_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if asNode = strings.TrimSpace(asNode); asNode != "" {
		metadata["source"] = "update"
		metadata["step"] = langGraphMetadataStep(metadata["step"]) + 1
		metadata["writes"] = map[string]any{asNode: values}
	}

	return metadata
}

func langGraphMetadataStep(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	default:
		return 0
	}
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

func langGraphThreadHistoryEntriesFromCheckpoints(
	thread *appagentthread.ThreadSummary,
	checkpoints []*appagentthread.CheckpointSummary,
) []*langgraphapi.ThreadHistoryEntry {
	result := make([]*langgraphapi.ThreadHistoryEntry, 0, len(checkpoints))
	for index, checkpoint := range checkpoints {
		projected := appagentthread.ProjectPublicCheckpoint(checkpoint)
		if projected == nil {
			continue
		}
		values := langGraphPublicCheckpointValues(projected)
		if index > 0 {
			delete(values, "messages")
		}
		parentID := (*string)(nil)
		if projected.ParentCheckpointID > 0 {
			value := strconv.FormatInt(projected.ParentCheckpointID, 10)
			parentID = &value
		}
		result = append(result, &langgraphapi.ThreadHistoryEntry{
			CheckpointID:       strconv.FormatInt(projected.CheckpointID, 10),
			ParentCheckpointID: parentID,
			Metadata: langGraphThreadStateMetadata(
				thread,
				langGraphPublicCheckpointMetadata(projected),
			),
			Values:    values,
			CreatedAt: langGraphTime(projected.CreatedAt),
			Next:      langGraphPublicCheckpointNext(projected, values),
		})
	}

	return result
}

func langGraphThreadStateFromCheckpoint(
	thread *appagentthread.ThreadSummary,
	checkpoint *appagentthread.CheckpointSummary,
) *langgraphapi.ThreadState {
	projected := appagentthread.ProjectPublicCheckpoint(checkpoint)
	if projected == nil {
		return nil
	}
	values := langGraphPublicCheckpointValues(projected)
	metadata := langGraphThreadStateMetadata(
		thread,
		langGraphPublicCheckpointMetadata(projected),
	)

	return &langgraphapi.ThreadState{
		Values: values,
		Next:   langGraphPublicCheckpointNext(projected, values),
		Config: langGraphThreadStateConfig(
			projected.ThreadID,
			strconv.FormatInt(projected.CheckpointID, 10),
			projected.CheckpointNS,
		),
		Metadata:  metadata,
		CreatedAt: langGraphTime(projected.CreatedAt),
		UpdatedAt: langGraphTime(projected.CreatedAt),
	}
}

func langGraphThreadHistoryEntriesFromEvents(events []*appagentthread.RunEventSummary) []*langgraphapi.ThreadHistoryEntry {
	result := make([]*langgraphapi.ThreadHistoryEntry, 0, len(events))
	for _, event := range events {
		projected := appagentthread.ProjectPublicRunEvent(event)
		if projected == nil {
			continue
		}
		checkpointID := "event-" + strconv.FormatInt(projected.EventID, 10)
		result = append(result, &langgraphapi.ThreadHistoryEntry{
			CheckpointID: checkpointID,
			Metadata: map[string]any{
				"thread_id":  strconv.FormatInt(projected.ThreadID, 10),
				"run_id":     strconv.FormatInt(projected.RunID, 10),
				"event_id":   strconv.FormatInt(projected.EventID, 10),
				"event_type": projected.EventType,
				"source":     "event_log",
			},
			Values: map[string]any{
				"event": langGraphRunEventPayload(projected.Payload),
			},
			CreatedAt: langGraphTime(projected.CreatedAt),
			Next:      []string{},
		})
	}

	return result
}

func langGraphThreadHistoryFromEvents(events []*appagentthread.RunEventSummary) []*langgraphapi.ThreadState {
	result := make([]*langgraphapi.ThreadState, 0, len(events))
	for _, event := range events {
		projected := appagentthread.ProjectPublicRunEvent(event)
		if projected == nil {
			continue
		}
		checkpointID := "event-" + strconv.FormatInt(projected.EventID, 10)
		result = append(result, &langgraphapi.ThreadState{
			Values: map[string]any{
				"messages":     []map[string]any{},
				"artifacts":    map[string]any{},
				"todos":        []any{},
				"memory":       map[string]any{},
				"tool_results": map[string]any{},
				"event":        langGraphRunEventPayload(projected.Payload),
			},
			Next:   []string{},
			Config: langGraphThreadStateConfig(projected.ThreadID, checkpointID),
			Metadata: map[string]any{
				"thread_id":  strconv.FormatInt(projected.ThreadID, 10),
				"run_id":     strconv.FormatInt(projected.RunID, 10),
				"event_id":   strconv.FormatInt(projected.EventID, 10),
				"event_type": projected.EventType,
				"source":     "event_log",
			},
			CreatedAt: langGraphTime(projected.CreatedAt),
		})
	}

	return result
}

func langGraphInternalCheckpointValues(checkpoint *appagentthread.CheckpointSummary) map[string]any {
	defaults := langGraphThreadStateValues(nil)
	if checkpoint == nil {
		return defaults
	}
	if strings.TrimSpace(checkpoint.RuntimeType) == string(appagentthread.RuntimeModeEinoADK) {
		values := defaults
		values["runtime"] = string(appagentthread.RuntimeModeEinoADK)
		if envelope, err := appagentthread.UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues)); err == nil {
			values["interrupts"] = langGraphADKInterruptIDs(envelope.Interrupts)
		} else {
			values["interrupts"] = []string{}
		}
		return values
	}

	values := langGraphRunEventPayloadMap(checkpoint.ChannelValues)
	for key, value := range defaults {
		if _, ok := values[key]; !ok {
			values[key] = value
		}
	}
	return values
}

func langGraphPublicCheckpointValues(checkpoint *appagentthread.PublicCheckpoint) map[string]any {
	defaults := langGraphThreadStateValues(nil)
	if checkpoint == nil {
		return defaults
	}
	values := make(map[string]any, len(checkpoint.Values)+len(defaults))
	for key, value := range checkpoint.Values {
		values[key] = value
	}
	for key, value := range defaults {
		if _, ok := values[key]; !ok {
			values[key] = value
		}
	}

	return values
}

func langGraphCheckpointMetadata(checkpoint *appagentthread.CheckpointSummary) map[string]any {
	return langGraphPublicCheckpointMetadata(appagentthread.ProjectPublicCheckpoint(checkpoint))
}

func langGraphPublicCheckpointMetadata(checkpoint *appagentthread.PublicCheckpoint) map[string]any {
	if checkpoint == nil {
		return map[string]any{}
	}
	metadata := make(map[string]any, len(checkpoint.Metadata)+6)
	for key, value := range checkpoint.Metadata {
		metadata[key] = value
	}
	metadata["thread_id"] = strconv.FormatInt(checkpoint.ThreadID, 10)
	metadata["run_id"] = strconv.FormatInt(checkpoint.RunID, 10)
	metadata["checkpoint_id"] = strconv.FormatInt(checkpoint.CheckpointID, 10)
	metadata["checkpoint_ns"] = checkpoint.CheckpointNS
	metadata["checkpoint_source"] = "checkpoint"
	if checkpoint.ParentCheckpointID > 0 {
		metadata["parent_checkpoint_id"] = strconv.FormatInt(checkpoint.ParentCheckpointID, 10)
	}
	return metadata
}

func langGraphPublicCheckpointNext(checkpoint *appagentthread.PublicCheckpoint, values map[string]any) []string {
	if checkpoint != nil && len(checkpoint.InterruptIDs) > 0 {
		return append([]string(nil), checkpoint.InterruptIDs...)
	}
	if interrupts := langGraphStringSliceValue(values["interrupts"]); len(interrupts) > 0 {
		return interrupts
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
		projected := appagentthread.ProjectPublicMessage(message)
		if projected == nil {
			continue
		}
		result = append(result, map[string]any{
			"id":         strconv.FormatInt(projected.MessageID, 10),
			"thread_id":  strconv.FormatInt(projected.ThreadID, 10),
			"run_id":     strconv.FormatInt(projected.RunID, 10),
			"role":       string(projected.Role),
			"content":    projected.Content,
			"metadata":   langGraphJSONMap(projected.Metadata),
			"created_at": langGraphTime(projected.CreatedAt),
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
	metadata := langGraphStoredThreadMetadata(thread)
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

func langGraphStoredThreadMetadata(thread *appagentthread.ThreadSummary) map[string]any {
	metadata := map[string]any{}
	if thread == nil || thread.Metadata == "" {
		return metadata
	}
	_ = sonic.UnmarshalString(thread.Metadata, &metadata)
	if metadata == nil {
		return map[string]any{}
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

func stripLangGraphPatchMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}
	reserved := map[string]struct{}{
		"owner_id":       {},
		"user_id":        {},
		"creator_id":     {},
		"space_id":       {},
		"thread_id":      {},
		"id":             {},
		"status":         {},
		"source":         {},
		"legacy_task_id": {},
		"created_at":     {},
		"updated_at":     {},
		"values":         {},
		"interrupts":     {},
	}
	result := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if _, ok := reserved[key]; ok {
			continue
		}
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

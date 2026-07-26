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
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

const canonicalMaxThreadMetadataKeys = 16

type canonicalCreateThreadRequest struct {
	ThreadID   *string                    `json:"thread_id,omitempty"`
	Metadata   map[string]any             `json:"metadata,omitempty"`
	IfExists   *string                    `json:"if_exists,omitempty"`
	TTL        any                        `json:"ttl,omitempty"`
	Supersteps []json.RawMessage          `json:"supersteps,omitempty"`
	Coze       *canonicalCreateThreadCoze `json:"coze,omitempty"`
}

type canonicalCreateThreadCoze struct {
	InitialRun         *canonicalInitialThreadRun `json:"initial_run,omitempty"`
	DeferredInitialRun *canonicalInitialThreadRun `json:"deferred_initial_run,omitempty"`
}

type canonicalInitialThreadRun struct {
	AssistantID string                      `json:"assistant_id,omitempty"`
	Input       canonicalInitialThreadInput `json:"input"`
	Config      map[string]any              `json:"config,omitempty"`
	Context     map[string]any              `json:"context,omitempty"`
	Metadata    map[string]any              `json:"metadata,omitempty"`
}

type canonicalInitialThreadInput struct {
	Messages []canonicalInitialThreadMessage `json:"messages"`
}

type canonicalInitialThreadMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type canonicalInitialSubmission struct {
	Message *canonicalMessage `json:"message"`
	Run     *canonicalRun     `json:"run"`
}

type canonicalSearchThreadsRequest struct {
	Metadata  map[string]any  `json:"metadata,omitempty"`
	Status    string          `json:"status,omitempty"`
	IDs       []string        `json:"ids,omitempty"`
	Limit     int32           `json:"limit,omitempty"`
	Offset    int32           `json:"offset,omitempty"`
	SortBy    string          `json:"sort_by,omitempty"`
	SortOrder string          `json:"sort_order,omitempty"`
	Values    json.RawMessage `json:"values,omitempty"`
	Select    json.RawMessage `json:"select,omitempty"`
	Extract   json.RawMessage `json:"extract,omitempty"`
}

type canonicalPatchThreadRequest struct {
	Metadata map[string]any  `json:"metadata,omitempty"`
	TTL      json.RawMessage `json:"ttl,omitempty"`
}

type canonicalUpdateThreadStateRequest struct {
	Values       map[string]any       `json:"values,omitempty"`
	AsNode       string               `json:"as_node,omitempty"`
	Checkpoint   *canonicalCheckpoint `json:"checkpoint,omitempty"`
	CheckpointID string               `json:"checkpoint_id,omitempty"`
}

type canonicalThreadHistoryRequest struct {
	Limit        int32                `json:"limit,omitempty"`
	Before       string               `json:"before,omitempty"`
	Checkpoint   *canonicalCheckpoint `json:"checkpoint,omitempty"`
	CheckpointID string               `json:"checkpoint_id,omitempty"`
}

type canonicalMessagePage struct {
	Data          []*canonicalMessage `json:"data"`
	HasMore       bool                `json:"has_more"`
	NextBeforeSeq *string             `json:"next_before_seq,omitempty"`
	NextAfterSeq  *string             `json:"next_after_seq,omitempty"`
}

// CreateCanonicalThread creates a canonical Thread for the authenticated principal in an
// authorized X-Coze-Space-ID workspace. Empty requests call CreateThread; initial and
// deferred submissions call CreateTaskThread so transaction, runtime validation, and title
// derivation remain owned by the existing application layer. It returns one canonical Thread.
func CreateCanonicalThread(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.create", "/api/workbench/threads")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}

	var req canonicalCreateThreadRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if public := validateCanonicalCreateThreadRequest(&req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, 0)

	metadata, title, threadSource, public := canonicalCreateThreadMetadata(req.Metadata)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("marshal canonical thread metadata: %w", err))
		return
	}

	var responseThread *appagentthread.ThreadSummary
	var initialSubmission *canonicalInitialSubmission
	initialRun, deferred := canonicalCreateThreadSubmission(&req)
	if initialRun == nil {
		requestLog.SubmissionKind = "empty_thread"
		if title == "" {
			title = defaultLangGraphThreadTitle
		}
		response, err := appagentthread.SVC.CreateThread(ctx, &appagentthread.CreateThreadRequest{
			SpaceID:  spaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			Title:    title,
			Source:   threadSource,
			Metadata: string(metadataJSON),
		})
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		if response == nil || response.Thread == nil {
			writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty thread"))
			return
		}
		responseThread = response.Thread
	} else {
		if deferred {
			requestLog.SubmissionKind = "deferred_initial_run"
		} else {
			requestLog.SubmissionKind = "initial_run"
		}
		message, public := canonicalInitialThreadMessageContent(initialRun)
		if public != nil {
			writeCanonicalError(ctx, c, public.status, *public)
			return
		}
		configJSON, err := canonicalMarshalOptionalObject(initialRun.Config)
		if err != nil {
			writeCanonicalApplicationError(ctx, c, fmt.Errorf("marshal canonical initial config: %w", err))
			return
		}
		contextJSON, err := canonicalMarshalOptionalObject(initialRun.Context)
		if err != nil {
			writeCanonicalApplicationError(ctx, c, fmt.Errorf("marshal canonical initial context: %w", err))
			return
		}
		runMetadata := canonicalSanitizeMap(initialRun.Metadata)
		runMetadataJSON, err := canonicalMarshalOptionalObject(runMetadata)
		if err != nil {
			writeCanonicalApplicationError(ctx, c, fmt.Errorf("marshal canonical initial metadata: %w", err))
			return
		}

		response, err := appagentthread.SVC.CreateTaskThread(ctx, &appagentthread.CreateTaskThreadRequest{
			SpaceID:        spaceID,
			UserID:         workbenchViewerIDFromCtx(ctx),
			Message:        message,
			Title:          title,
			ThreadMetadata: string(metadataJSON),
			ThreadSource:   threadSource,
			DeferStart:     deferred,
			AssistantID:    strings.TrimSpace(initialRun.AssistantID),
			Config:         configJSON,
			Context:        contextJSON,
			Metadata:       runMetadataJSON,
			IdempotencyKey: strings.TrimSpace(string(c.GetHeader("Idempotency-Key"))),
		})
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		if response == nil || response.Thread == nil {
			writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty task thread"))
			return
		}
		responseThread = response.Thread
		if !deferred {
			messageProjection, projectionErr := projectCanonicalMessage(response.Message)
			if projectionErr != nil {
				writeCanonicalApplicationError(ctx, c, projectionErr)
				return
			}
			runProjection, projectionErr := projectCanonicalRun(response.Run)
			if projectionErr != nil {
				writeCanonicalApplicationError(ctx, c, projectionErr)
				return
			}
			if messageProjection == nil || runProjection == nil {
				writeCanonicalApplicationError(ctx, c, fmt.Errorf("canonical initial submission projection is incomplete"))
				return
			}
			initialSubmission = &canonicalInitialSubmission{
				Message: messageProjection,
				Run:     runProjection,
			}
		}
	}
	projected, err := projectCanonicalThread(ctx, responseThread)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical thread projection returned empty thread")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	projected.Coze.InitialSubmission = initialSubmission
	requestLog.ThreadID = responseThread.ThreadID
	c.JSON(consts.StatusOK, projected)
}

// SearchCanonicalThreads searches the authenticated principal's authorized
// X-Coze-Space-ID workspace through ApplicationService.SearchThreads. It returns
// a raw canonical Thread array plus X-Pagination-Total and X-Pagination-Next.
// The handler is read-only and computes cursors only after authorization and
// canonical status projection.
func SearchCanonicalThreads(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.search", "/api/workbench/threads/search")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	var req canonicalSearchThreadsRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if public := validateCanonicalSearchThreadsRequest(&req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, 0, 0)

	ids, public := canonicalThreadIDs(req.IDs)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	metadata, public := canonicalThreadMetadataFilter(req.Metadata)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	limit := canonicalThreadLimit(req.Limit)
	threads, total, err := searchCanonicalThreadProjections(ctx, canonicalThreadSearch{
		SpaceID: spaceID, IDs: ids, Metadata: metadata,
		Status: strings.TrimSpace(req.Status), SortBy: req.SortBy, SortOrder: req.SortOrder,
		Offset: req.Offset, Limit: limit,
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	setCanonicalPaginationHeaders(c, total, int(req.Offset), int(limit))
	c.JSON(consts.StatusOK, threads)
}

// GetCanonicalThread reads one authorized Thread through ApplicationService.GetThread
// and returns its canonical public projection. It has no side effects and rejects the
// SDK include expansion until field-level projection authorization is available.
func GetCanonicalThread(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.get", "/api/workbench/threads/:thread_id")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	if include, exists := c.GetQuery("include"); exists && strings.TrimSpace(include) != "" {
		public := canonicalUnsupportedField("include")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	response, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{
		ThreadID: threadID,
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil || response.Thread == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	projected, err := projectCanonicalThread(ctx, response.Thread)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical thread projection returned empty thread")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, projected)
}

// PatchCanonicalThread atomically patches an authorized Thread's title and reviewed
// scalar metadata through ApplicationService.PatchThread. It never reads and rewrites
// metadata in the handler. A precise Prefer: return=minimal request returns 204;
// otherwise the response is the updated canonical Thread.
func PatchCanonicalThread(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.patch", "/api/workbench/threads/:thread_id")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	prefer := string(c.GetHeader("Prefer"))
	if prefer != "" && prefer != "return=minimal" {
		public := canonicalUnsupportedField("Prefer")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	var req canonicalPatchThreadRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if len(req.TTL) > 0 {
		public := canonicalUnsupportedField("ttl")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	title, metadata, public := canonicalThreadMetadataPatch(req.Metadata)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	response, err := appagentthread.SVC.PatchThread(ctx, &appagentthread.PatchThreadRequest{
		ThreadID: threadID, Title: title, MetadataPatch: metadata, UpdatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil || response.Thread == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	if prefer == "return=minimal" {
		c.Status(consts.StatusNoContent)
		return
	}
	projected, err := projectCanonicalThread(ctx, response.Thread)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical patched thread projection returned empty thread")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, projected)
}

// DeleteCanonicalThread deletes an authorized non-busy Thread through the atomic
// ApplicationService.DeleteThreadIfIdle use case. Active top-level Runs produce
// 409 thread_busy and are neither canceled nor deleted. Success returns 204.
func DeleteCanonicalThread(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.delete", "/api/workbench/threads/:thread_id")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	response, err := appagentthread.SVC.DeleteThreadIfIdle(ctx, &appagentthread.DeleteThreadIfIdleRequest{
		ThreadID: threadID,
	})
	if errors.Is(err, appagentthread.ErrActiveRunExists) {
		public := newCanonicalError(
			consts.StatusConflict,
			"thread_busy",
			"Thread has an active run",
			"active_run_exists",
			false,
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil || !response.Deleted {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	c.Status(consts.StatusNoContent)
}

// GetCanonicalThreadState reads an authorized Thread's latest or selected
// checkpoint through the application layer and returns only the canonical public
// state projection. Raw runtime checkpoint bytes and internal task results are never
// exposed. subgraphs=false is accepted; true is rejected until safe subgraph DTOs exist.
func GetCanonicalThreadState(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.state.get", "/api/workbench/threads/:thread_id/state")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	if public := validateCanonicalSubgraphsQuery(c); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	checkpointID, public := canonicalCheckpointIDFromStrings(
		canonicalQueryString(c, "checkpoint"),
		canonicalQueryString(c, "checkpoint_id"),
	)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	checkpoint, err := getCanonicalThreadCheckpoint(ctx, threadID, checkpointID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if checkpoint == nil || checkpoint.ThreadID != threadID {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	state, err := projectCanonicalCheckpointState(checkpoint)
	if err != nil || state == nil {
		if err == nil {
			err = fmt.Errorf("canonical checkpoint projection returned empty state")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, state)
}

// UpdateCanonicalThreadState atomically merges the reviewed custom channel through
// ApplicationService.UpdatePublicThreadState. The new canonical_public_state row is
// isolated from Eino runtime checkpoints. The response contains checkpoint and
// configurable references derived from the same committed row.
func UpdateCanonicalThreadState(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.state.update", "/api/workbench/threads/:thread_id/state")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	var req canonicalUpdateThreadStateRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if public := validateCanonicalPublicStateValues(req.Values); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	checkpointID, public := canonicalCheckpointIDFromReference(threadID, req.Checkpoint, req.CheckpointID)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	response, err := appagentthread.SVC.UpdatePublicThreadState(
		ctx,
		&appagentthread.UpdatePublicThreadStateRequest{
			ThreadID: threadID, BaseCheckpointID: checkpointID,
			Values: req.Values, AsNode: strings.TrimSpace(req.AsNode),
		},
	)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil || response.Checkpoint == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty public state"))
		return
	}
	checkpoint, err := projectCanonicalCheckpointReference(response.Checkpoint)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, projectCanonicalThreadUpdateStateResult(checkpoint))
}

// GetCanonicalThreadHistory reads authorized checkpoint history using the stable
// numeric before cursor and returns newest-first canonical ThreadState entries.
// It shares the same application query and projection as the POST form.
func GetCanonicalThreadHistory(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.history.get", "/api/workbench/threads/:thread_id/history")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	limit, public := canonicalQueryLimit(c, "limit", 20)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	before, public := canonicalCheckpointIDFromStrings(
		canonicalQueryString(c, "before"),
		firstCanonicalQueryString(c, "checkpoint_id", "checkpoint"),
	)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	serveCanonicalThreadHistory(ctx, c, threadID, before, limit)
}

// PostCanonicalThreadHistory is the SDK-compatible body form of Thread history.
// It delegates to the same authorized ListCheckpointsBefore query and returns the
// same newest-first canonical ThreadState array as GET.
func PostCanonicalThreadHistory(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.history.post", "/api/workbench/threads/:thread_id/history")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	var req canonicalThreadHistoryRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if req.Limit < 0 {
		public := canonicalInvalidRequest("History limit cannot be negative", "invalid_pagination")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	limit := canonicalThreadLimit(req.Limit)
	checkpointID, public := canonicalCheckpointIDFromReference(threadID, req.Checkpoint, req.CheckpointID)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	before, public := canonicalCheckpointIDFromStrings(req.Before, canonicalOptionalIDString(checkpointID))
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	serveCanonicalThreadHistory(ctx, c, threadID, before, limit)
}

// ListCanonicalThreadMessages loads the complete authorized Run journal through
// application queries, narrows it to public human/assistant messages, assigns stable
// append-only seq values, and only then applies before_seq/after_seq pagination.
// Tool arguments, tool results, runtime errors, and checkpoint data are excluded.
func ListCanonicalThreadMessages(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("thread.messages.list", "/api/workbench/threads/:thread_id/messages")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAPI(ctx, c) {
		return
	}
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	before, public := canonicalOptionalPositiveQueryID(c, "before_seq")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	after, public := canonicalOptionalPositiveQueryID(c, "after_seq")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if before > 0 && after > 0 {
		public := canonicalInvalidRequest(
			"before_seq and after_seq are mutually exclusive",
			"mutually_exclusive_message_cursors",
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	limit, public := canonicalQueryLimit(c, "limit", 20)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	messages, err := loadCanonicalThreadMessages(ctx, threadID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, pageCanonicalMessages(messages, before, after, limit))
}

func serveCanonicalThreadHistory(
	ctx context.Context,
	c *app.RequestContext,
	threadID, before int64,
	limit int32,
) {
	ctx = workbenchThreadAccessContext(ctx, threadID, 0)
	response, err := appagentthread.SVC.ListCheckpointsBefore(
		ctx,
		&appagentthread.ListCheckpointsBeforeRequest{
			ThreadID: threadID, BeforeCheckpointID: before, Limit: limit,
		},
	)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty history"))
		return
	}
	history := make([]*canonicalThreadState, 0, len(response.Checkpoints))
	for _, checkpoint := range response.Checkpoints {
		if checkpoint == nil || checkpoint.ThreadID != threadID {
			continue
		}
		state, err := projectCanonicalCheckpointState(checkpoint)
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		if state != nil {
			history = append(history, state)
		}
	}
	c.JSON(consts.StatusOK, history)
}

func getCanonicalThreadCheckpoint(
	ctx context.Context,
	threadID, checkpointID int64,
) (*appagentthread.CheckpointSummary, error) {
	if checkpointID > 0 {
		response, err := appagentthread.SVC.GetCheckpoint(ctx, &appagentthread.GetCheckpointRequest{
			CheckpointID: checkpointID,
		})
		if err != nil {
			return nil, err
		}
		if response == nil || response.Checkpoint == nil || response.Checkpoint.ThreadID != threadID {
			return nil, nil
		}
		return response.Checkpoint, nil
	}
	response, err := appagentthread.SVC.GetLatestCheckpoint(ctx, &appagentthread.GetLatestCheckpointRequest{
		ThreadID: threadID,
	})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, nil
	}
	return response.Checkpoint, nil
}

func projectCanonicalCheckpointState(
	checkpoint *appagentthread.CheckpointSummary,
) (*canonicalThreadState, error) {
	public := appagentthread.ProjectPublicCheckpoint(checkpoint)
	if public == nil {
		return nil, nil
	}
	next := append([]string(nil), public.InterruptIDs...)
	if len(next) == 0 {
		next = canonicalStringSlice(public.Values["next"])
	}
	interrupts := make([]map[string]any, 0, len(public.InterruptIDs))
	for _, id := range public.InterruptIDs {
		interrupts = append(interrupts, map[string]any{
			"id": id, "when": "during", "resumable": true,
		})
	}
	return projectCanonicalThreadState(canonicalStateSource{
		ThreadID: checkpoint.ThreadID, CheckpointID: checkpoint.CheckpointID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       checkpoint.CheckpointNS, ParentCheckpointNS: checkpoint.CheckpointNS,
		Values: public.Values, Next: next, Metadata: public.Metadata,
		CreatedAt: checkpoint.CreatedAt, Interrupts: interrupts,
	})
}

func projectCanonicalCheckpointReference(
	checkpoint *appagentthread.CheckpointSummary,
) (canonicalCheckpoint, error) {
	if checkpoint == nil || checkpoint.ThreadID <= 0 || checkpoint.CheckpointID <= 0 {
		return canonicalCheckpoint{}, fmt.Errorf("canonical checkpoint reference requires positive ids")
	}
	namespace, err := canonicalCheckpointNamespace(checkpoint.CheckpointNS)
	if err != nil {
		return canonicalCheckpoint{}, err
	}
	return canonicalCheckpoint{
		ThreadID: strconv.FormatInt(checkpoint.ThreadID, 10), CheckpointNS: namespace,
		CheckpointID: strconv.FormatInt(checkpoint.CheckpointID, 10), CheckpointMap: map[string]any{},
	}, nil
}

func validateCanonicalSubgraphsQuery(c *app.RequestContext) *canonicalError {
	raw, exists := c.GetQuery("subgraphs")
	if !exists || raw == "" || raw == "false" {
		return nil
	}
	if raw == "true" {
		return canonicalUnsupportedField("subgraphs")
	}
	return canonicalInvalidRequest("subgraphs must be false", "invalid_subgraphs")
}

func validateCanonicalPublicStateValues(values map[string]any) *canonicalError {
	if len(values) != 1 {
		return canonicalInvalidRequest(
			"State update must contain only the custom channel",
			"unsupported_state_channel",
		)
	}
	custom, ok := values["custom"].(map[string]any)
	if !ok || custom == nil {
		return canonicalInvalidRequest(
			"State custom channel must be a JSON object",
			"invalid_custom_state",
		)
	}
	if !canonicalPublicCustomValueIsSafe(custom, 0) {
		return canonicalInvalidRequest(
			"State custom channel contains a protected or unsafe value",
			"unsafe_custom_state",
		)
	}
	return nil
}

func canonicalPublicCustomValueIsSafe(value any, depth int) bool {
	if depth > 32 {
		return false
	}
	switch typed := value.(type) {
	case nil, bool, json.Number,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	case string:
		return len([]rune(typed)) <= canonicalMaxPublicValueRunes &&
			!canonicalSensitiveValuePattern.MatchString(typed)
	case []any:
		for _, item := range typed {
			if !canonicalPublicCustomValueIsSafe(item, depth+1) {
				return false
			}
		}
		return true
	case map[string]any:
		for key, item := range typed {
			if strings.TrimSpace(key) == "" || len([]rune(key)) > 128 ||
				canonicalUnsafePublicField(key) ||
				!canonicalPublicCustomValueIsSafe(item, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func canonicalCheckpointIDFromReference(
	threadID int64,
	checkpoint *canonicalCheckpoint,
	checkpointID string,
) (int64, *canonicalError) {
	checkpointValue := ""
	if checkpoint != nil {
		if checkpoint.ThreadID != "" {
			value, public := parseCanonicalPositiveID(
				checkpoint.ThreadID,
				"invalid_request",
				"Checkpoint thread_id must be a positive decimal ID",
				"invalid_checkpoint_thread_id",
			)
			if public != nil || value != threadID {
				return 0, canonicalInvalidRequest(
					"Checkpoint does not belong to thread",
					"checkpoint_thread_mismatch",
				)
			}
		}
		checkpointValue = checkpoint.CheckpointID
	}
	return canonicalCheckpointIDFromStrings(checkpointValue, checkpointID)
}

func canonicalCheckpointIDFromStrings(left, right string) (int64, *canonicalError) {
	left, right = strings.TrimSpace(left), strings.TrimSpace(right)
	if left != "" && right != "" && left != right {
		return 0, canonicalInvalidRequest("Checkpoint cursors do not match", "checkpoint_cursor_mismatch")
	}
	value := left
	if value == "" {
		value = right
	}
	if value == "" {
		return 0, nil
	}
	id, public := parseCanonicalPositiveID(
		value,
		"invalid_request",
		"Checkpoint id must be a positive decimal ID",
		"invalid_checkpoint_id",
	)
	if public != nil {
		public.status = consts.StatusUnprocessableEntity
		return 0, public
	}
	return id, nil
}

func canonicalQueryString(c *app.RequestContext, name string) string {
	value, _ := c.GetQuery(name)
	return value
}

func firstCanonicalQueryString(c *app.RequestContext, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(canonicalQueryString(c, name)); value != "" {
			return value
		}
	}
	return ""
}

func canonicalOptionalIDString(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func canonicalQueryLimit(
	c *app.RequestContext,
	name string,
	defaultLimit int32,
) (int32, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists || strings.TrimSpace(raw) == "" {
		return defaultLimit, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 0 {
		return 0, canonicalInvalidRequest("Invalid query parameter: "+name, "invalid_pagination")
	}
	if value == 0 {
		return defaultLimit, nil
	}
	if value > 100 {
		return 100, nil
	}
	return int32(value), nil
}

func canonicalOptionalPositiveQueryID(c *app.RequestContext, name string) (int64, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, public := parseCanonicalPositiveID(
		raw,
		"invalid_query_parameter",
		"Invalid query parameter: "+name,
		"invalid_message_cursor",
	)
	if public != nil {
		public.status = consts.StatusUnprocessableEntity
		return 0, public
	}
	return value, nil
}

func canonicalStringSlice(value any) []string {
	result := []string{}
	switch values := value.(type) {
	case []string:
		result = append(result, values...)
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
	}
	return canonicalIdentifiers(result)
}

func loadCanonicalThreadMessages(
	ctx context.Context,
	threadID int64,
) ([]*canonicalMessage, error) {
	runs, err := loadAllCanonicalThreadRuns(ctx, threadID)
	if err != nil {
		return nil, err
	}
	messages, err := loadAllCanonicalPersistedMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	events, err := loadAllCanonicalThreadRunEvents(ctx, threadID)
	if err != nil {
		return nil, err
	}
	journal := appagentthread.ProjectThreadRunJournalMessages(runs, messages, events)
	projected := make([]*canonicalMessage, 0, len(journal))
	for _, message := range journal {
		if message == nil || message.ThreadID != threadID || message.RunID <= 0 {
			continue
		}
		if message.Role != appagentthread.MessageRoleUser &&
			message.Role != appagentthread.MessageRoleAssistant {
			continue
		}
		content := canonicalCleanString(message.Content, canonicalMaxPublicValueRunes)
		if content == "" {
			continue
		}
		projected = append(projected, &canonicalMessage{
			MessageID: canonicalCleanString(message.ID, 128),
			ThreadID:  strconv.FormatInt(message.ThreadID, 10),
			RunID:     strconv.FormatInt(message.RunID, 10),
			Role:      string(message.Role), Content: content, Metadata: map[string]any{},
			CreatedAt: canonicalTime(message.CreatedAt), sortCreatedAt: message.CreatedAt,
		})
	}
	sort.SliceStable(projected, func(left, right int) bool {
		leftTime, rightTime := projected[left].sortCreatedAt, projected[right].sortCreatedAt
		if leftTime != rightTime {
			return leftTime < rightTime
		}
		leftKind, leftID := canonicalMessageSourceOrder(projected[left].MessageID)
		rightKind, rightID := canonicalMessageSourceOrder(projected[right].MessageID)
		if leftKind != rightKind {
			return leftKind < rightKind
		}
		if leftID != rightID {
			return leftID < rightID
		}
		return projected[left].MessageID < projected[right].MessageID
	})
	for index := range projected {
		projected[index].Seq = strconv.Itoa(index + 1)
	}
	return projected, nil
}

func loadAllCanonicalThreadRuns(
	ctx context.Context,
	threadID int64,
) ([]*appagentthread.RunSummary, error) {
	const pageSize = int32(100)
	result := []*appagentthread.RunSummary{}
	for offset := int32(0); ; offset += pageSize {
		response, err := appagentthread.SVC.SearchRuns(ctx, &appagentthread.SearchRunsRequest{
			ThreadID: threadID, Page: appagentthread.CanonicalPage{Offset: offset, Limit: pageSize},
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, fmt.Errorf("agent thread application returned empty run page")
		}
		result = append(result, response.Runs...)
		if int64(offset)+int64(len(response.Runs)) >= response.Total || len(response.Runs) == 0 {
			return result, nil
		}
		if offset > math.MaxInt32-pageSize {
			return nil, fmt.Errorf("canonical run journal is too large")
		}
	}
}

func loadAllCanonicalPersistedMessages(
	ctx context.Context,
	threadID int64,
) ([]*appagentthread.MessageSummary, error) {
	const pageSize = int32(100)
	result := []*appagentthread.MessageSummary{}
	for page := int32(1); ; page++ {
		response, err := appagentthread.SVC.ListMessages(ctx, &appagentthread.ListMessagesRequest{
			ThreadID: threadID, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, fmt.Errorf("agent thread application returned empty message page")
		}
		result = append(result, response.Messages...)
		if int64(len(result)) >= response.Total || len(response.Messages) == 0 {
			return result, nil
		}
		if page == math.MaxInt32 {
			return nil, fmt.Errorf("canonical message journal is too large")
		}
	}
}

func loadAllCanonicalThreadRunEvents(
	ctx context.Context,
	threadID int64,
) ([]*appagentthread.RunEventSummary, error) {
	const pageSize = int32(100)
	result := []*appagentthread.RunEventSummary{}
	for page := int32(1); ; page++ {
		response, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
			ThreadID: threadID, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, fmt.Errorf("agent thread application returned empty event page")
		}
		result = append(result, response.Events...)
		if int64(len(result)) >= response.Total || len(response.Events) == 0 {
			return result, nil
		}
		if page == math.MaxInt32 {
			return nil, fmt.Errorf("canonical event journal is too large")
		}
	}
}

func canonicalMessageSourceOrder(id string) (int, int64) {
	if value, err := strconv.ParseInt(id, 10, 64); err == nil && value > 0 {
		return 0, value
	}
	if strings.HasPrefix(id, "run-") {
		return 1, canonicalNumericIDFragment(id)
	}
	if strings.HasPrefix(id, "event-") {
		return 2, canonicalNumericIDFragment(id)
	}
	return 3, canonicalNumericIDFragment(id)
}

func canonicalNumericIDFragment(value string) int64 {
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if id, err := strconv.ParseInt(part, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return math.MaxInt64
}

func pageCanonicalMessages(
	messages []*canonicalMessage,
	before, after int64,
	limit int32,
) canonicalMessagePage {
	page := canonicalMessagePage{Data: []*canonicalMessage{}}
	if before > 0 {
		end := int(before - 1)
		if end > len(messages) {
			end = len(messages)
		}
		start := end - int(limit)
		if start < 0 {
			start = 0
		}
		page.Data = append(page.Data, messages[start:end]...)
		page.HasMore = start > 0
		if page.HasMore && len(page.Data) > 0 {
			next := page.Data[0].Seq
			page.NextBeforeSeq = &next
		}
		if end < len(messages) && len(page.Data) > 0 {
			next := page.Data[len(page.Data)-1].Seq
			page.NextAfterSeq = &next
		}
		return page
	}

	start := int(after)
	if start > len(messages) {
		start = len(messages)
	}
	end := start + int(limit)
	if end > len(messages) {
		end = len(messages)
	}
	page.Data = append(page.Data, messages[start:end]...)
	page.HasMore = end < len(messages)
	if start > 0 && len(page.Data) > 0 {
		next := page.Data[0].Seq
		page.NextBeforeSeq = &next
	}
	if page.HasMore && len(page.Data) > 0 {
		next := page.Data[len(page.Data)-1].Seq
		page.NextAfterSeq = &next
	}
	return page
}

type canonicalThreadSearch struct {
	SpaceID   int64
	IDs       []int64
	Metadata  map[string]any
	Status    string
	SortBy    string
	SortOrder string
	Offset    int32
	Limit     int32
}

func searchCanonicalThreadProjections(
	ctx context.Context,
	query canonicalThreadSearch,
) ([]*canonicalThread, int64, error) {
	projectedStatusSort := strings.TrimSpace(query.SortBy) == "status"
	if query.Status == "" && !projectedStatusSort {
		response, err := appagentthread.SVC.SearchThreads(ctx, &appagentthread.SearchThreadsRequest{
			SpaceID: query.SpaceID, UserID: workbenchViewerIDFromCtx(ctx),
			IDs: query.IDs, Metadata: query.Metadata, SortBy: query.SortBy, SortOrder: query.SortOrder,
			Page: appagentthread.CanonicalPage{Offset: query.Offset, Limit: query.Limit},
		})
		if err != nil {
			return nil, 0, err
		}
		if response == nil {
			return nil, 0, fmt.Errorf("agent thread application returned empty search response")
		}
		projected, err := projectCanonicalThreads(ctx, response.Threads)
		return projected, response.Total, err
	}

	// SDK statuses are derived from the latest top-level Run and cannot be
	// represented by the persisted product-status column alone. Scan authorized
	// repository pages in stable order, project, then apply the public cursor.
	const applicationPageSize = int32(100)
	matching := make([]*canonicalThread, 0)
	for offset := int32(0); ; offset += applicationPageSize {
		response, err := appagentthread.SVC.SearchThreads(ctx, &appagentthread.SearchThreadsRequest{
			SpaceID: query.SpaceID, UserID: workbenchViewerIDFromCtx(ctx),
			IDs: query.IDs, Metadata: query.Metadata, SortBy: query.SortBy, SortOrder: query.SortOrder,
			Page: appagentthread.CanonicalPage{Offset: offset, Limit: applicationPageSize},
		})
		if err != nil {
			return nil, 0, err
		}
		if response == nil {
			return nil, 0, fmt.Errorf("agent thread application returned empty search response")
		}
		page, err := projectCanonicalThreads(ctx, response.Threads)
		if err != nil {
			return nil, 0, err
		}
		for _, thread := range page {
			if query.Status == "" || thread.Status == query.Status {
				matching = append(matching, thread)
			}
		}
		if int64(offset)+int64(len(response.Threads)) >= response.Total || len(response.Threads) == 0 {
			break
		}
		if offset > math.MaxInt32-applicationPageSize {
			return nil, 0, fmt.Errorf("canonical thread search result is too large")
		}
	}
	if projectedStatusSort {
		sortCanonicalThreadsByProjectedStatus(matching, strings.TrimSpace(query.SortOrder))
	}
	total := int64(len(matching))
	start := int(query.Offset)
	if start >= len(matching) {
		return []*canonicalThread{}, total, nil
	}
	end := start + int(query.Limit)
	if end > len(matching) {
		end = len(matching)
	}
	return matching[start:end], total, nil
}

func sortCanonicalThreadsByProjectedStatus(threads []*canonicalThread, order string) {
	descending := order == "desc"
	sort.SliceStable(threads, func(left, right int) bool {
		leftThread, rightThread := threads[left], threads[right]
		if leftThread.Status != rightThread.Status {
			if descending {
				return leftThread.Status > rightThread.Status
			}
			return leftThread.Status < rightThread.Status
		}
		leftID, _ := strconv.ParseInt(leftThread.ThreadID, 10, 64)
		rightID, _ := strconv.ParseInt(rightThread.ThreadID, 10, 64)
		if descending {
			return leftID > rightID
		}
		return leftID < rightID
	})
}

func projectCanonicalThreads(
	ctx context.Context,
	summaries []*appagentthread.ThreadSummary,
) ([]*canonicalThread, error) {
	result := make([]*canonicalThread, 0, len(summaries))
	for _, summary := range summaries {
		thread, err := projectCanonicalThread(ctx, summary)
		if err != nil {
			return nil, err
		}
		if thread != nil {
			result = append(result, thread)
		}
	}
	return result, nil
}

func validateCanonicalSearchThreadsRequest(req *canonicalSearchThreadsRequest) *canonicalError {
	if req == nil {
		return canonicalInvalidRequest("Search threads request is required", "missing_request")
	}
	for field, raw := range map[string]json.RawMessage{
		"values": req.Values, "select": req.Select, "extract": req.Extract,
	} {
		if len(raw) > 0 {
			return canonicalUnsupportedField(field)
		}
	}
	if req.Offset < 0 || req.Limit < 0 {
		return canonicalInvalidRequest("Pagination values cannot be negative", "invalid_pagination")
	}
	status := strings.TrimSpace(req.Status)
	if status != "" && status != "idle" && status != "busy" &&
		status != "interrupted" && status != "error" {
		return canonicalInvalidRequest("Invalid thread status", "invalid_thread_status")
	}
	sortBy := strings.TrimSpace(req.SortBy)
	if sortBy != "" && sortBy != "thread_id" && sortBy != "status" &&
		sortBy != "created_at" && sortBy != "updated_at" {
		return canonicalInvalidRequest("Invalid thread sort field", "invalid_sort")
	}
	sortOrder := strings.TrimSpace(req.SortOrder)
	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		return canonicalInvalidRequest("Invalid thread sort order", "invalid_sort")
	}
	return nil
}

func canonicalThreadIDs(values []string) ([]int64, *canonicalError) {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		id, public := parseCanonicalPositiveID(
			value,
			"invalid_request",
			"Thread ids must be positive decimal IDs",
			"invalid_thread_id",
		)
		if public != nil {
			public.status = consts.StatusUnprocessableEntity
			return nil, public
		}
		result = append(result, id)
	}
	return result, nil
}

func canonicalThreadMetadataFilter(metadata map[string]any) (map[string]any, *canonicalError) {
	if public := validateCanonicalThreadMetadataCount(metadata); public != nil {
		return nil, public
	}
	result := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) || canonicalProtectedMetadataKey(key) || key == "status" {
			return nil, canonicalUnsupportedField("metadata." + key)
		}
		projected, ok := canonicalMetadataValue(value)
		if !ok {
			return nil, canonicalInvalidRequest("Invalid metadata value: "+key, "invalid_metadata_value")
		}
		result[key] = projected
	}
	return result, nil
}

func canonicalThreadMetadataPatch(
	metadata map[string]any,
) (*string, map[string]any, *canonicalError) {
	if len(metadata) == 0 {
		return nil, nil, canonicalInvalidRequest("Thread patch has no fields", "empty_patch")
	}
	if public := validateCanonicalThreadMetadataCount(metadata); public != nil {
		return nil, nil, public
	}
	var title *string
	patch := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) || canonicalProtectedMetadataKey(key) || key == "status" {
			return nil, nil, canonicalUnsupportedField("metadata." + key)
		}
		if key == "title" {
			raw, ok := value.(string)
			projected := canonicalMetadataLabel(raw)
			if !ok || projected == "" {
				return nil, nil, canonicalInvalidRequest("Invalid metadata title", "invalid_title")
			}
			title = &projected
			continue
		}
		projected, ok := canonicalMetadataValue(value)
		if !ok {
			return nil, nil, canonicalInvalidRequest("Invalid metadata value: "+key, "invalid_metadata_value")
		}
		patch[key] = projected
	}
	return title, patch, nil
}

func canonicalThreadLimit(limit int32) int32 {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func writeCanonicalResourceNotFound(ctx context.Context, c *app.RequestContext) {
	public := newCanonicalError(
		consts.StatusNotFound,
		"resource_not_found",
		"Resource not found",
		"resource_not_found",
		false,
	)
	writeCanonicalError(ctx, c, public.status, *public)
}

func validateCanonicalCreateThreadRequest(req *canonicalCreateThreadRequest) *canonicalError {
	if req == nil {
		return canonicalInvalidRequest("Create thread request is required", "missing_request")
	}
	if req.ThreadID != nil {
		return canonicalUnsupportedField("thread_id")
	}
	if req.TTL != nil {
		return canonicalUnsupportedField("ttl")
	}
	if req.Supersteps != nil {
		return canonicalUnsupportedField("supersteps")
	}
	if req.IfExists != nil && *req.IfExists != "raise" {
		return canonicalUnsupportedField("if_exists")
	}
	if _, exists := req.Metadata["graph_id"]; exists {
		return canonicalUnsupportedField("metadata.graph_id")
	}
	if req.Coze != nil && req.Coze.InitialRun != nil && req.Coze.DeferredInitialRun != nil {
		return canonicalInvalidRequest(
			"coze.initial_run and coze.deferred_initial_run are mutually exclusive",
			"mutually_exclusive_initial_submission",
		)
	}
	return nil
}

func canonicalCreateThreadSubmission(
	req *canonicalCreateThreadRequest,
) (*canonicalInitialThreadRun, bool) {
	if req == nil || req.Coze == nil {
		return nil, false
	}
	if req.Coze.InitialRun != nil {
		return req.Coze.InitialRun, false
	}
	if req.Coze.DeferredInitialRun != nil {
		return req.Coze.DeferredInitialRun, true
	}
	return nil, false
}

func canonicalCreateThreadMetadata(
	metadata map[string]any,
) (map[string]any, string, appagentthread.ThreadSource, *canonicalError) {
	if public := validateCanonicalThreadMetadataCount(metadata); public != nil {
		return nil, "", "", public
	}
	result := make(map[string]any, len(metadata))
	threadSource := appagentthread.ThreadSourceAPI
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) ||
			canonicalProtectedMetadataKey(key) || key == "status" {
			return nil, "", "", canonicalUnsupportedField("metadata." + key)
		}
		projected, ok := canonicalMetadataValue(value)
		if !ok {
			return nil, "", "", canonicalInvalidRequest(
				"Invalid metadata value: "+key,
				"invalid_metadata_value",
			)
		}
		result[key] = projected
	}

	title := canonicalMetadataLabel(canonicalString(result["title"]))
	if raw, exists := result["title"]; exists {
		if _, ok := raw.(string); !ok || title == "" {
			return nil, "", "", canonicalInvalidRequest("Invalid metadata title", "invalid_title")
		}
		result["title"] = title
	}
	if source, ok := result["source"].(string); ok {
		switch strings.TrimSpace(source) {
		case string(appagentthread.ThreadSourceWeb):
			threadSource = appagentthread.ThreadSourceWeb
		case string(appagentthread.ThreadSourceAPI):
			threadSource = appagentthread.ThreadSourceAPI
		}
	}
	return result, title, threadSource, nil
}

func validateCanonicalThreadMetadataCount(metadata map[string]any) *canonicalError {
	if len(metadata) <= canonicalMaxThreadMetadataKeys {
		return nil
	}
	return canonicalInvalidRequest(
		"Thread metadata supports at most 16 keys",
		"invalid_metadata_count",
	)
}

func canonicalInitialThreadMessageContent(
	run *canonicalInitialThreadRun,
) (string, *canonicalError) {
	if run == nil || len(run.Input.Messages) != 1 {
		return "", canonicalInvalidRequest(
			"Initial submission must contain exactly one user message",
			"invalid_initial_messages",
		)
	}
	message := run.Input.Messages[0]
	if strings.TrimSpace(message.Role) != "user" || strings.TrimSpace(message.Content) == "" {
		return "", canonicalInvalidRequest(
			"Initial submission must contain one non-empty user message",
			"invalid_initial_message",
		)
	}
	return strings.TrimSpace(message.Content), nil
}

func canonicalMarshalOptionalObject(value map[string]any) (string, error) {
	if len(value) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func canonicalUnsupportedField(field string) *canonicalError {
	return newCanonicalError(
		consts.StatusUnprocessableEntity,
		"unsupported_sdk_field",
		"Unsupported field: "+field,
		"unsupported_field",
		false,
	)
}

func canonicalInvalidRequest(detail, errorClass string) *canonicalError {
	return newCanonicalError(
		consts.StatusUnprocessableEntity,
		"invalid_request",
		detail,
		errorClass,
		false,
	)
}

func writeCanonicalApplicationError(ctx context.Context, c *app.RequestContext, err error) {
	public := mapCanonicalApplicationError(err)
	writeCanonicalError(ctx, c, public.status, public)
}

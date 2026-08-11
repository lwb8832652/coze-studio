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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	canonicalPublicAssistantID              = "agent"
	canonicalRunIdempotencyOperationInitial = "workbench.thread.initial_run.v1"
	canonicalRunIdempotencyOperationTurn    = "workbench.run.turn.v1"
	canonicalRunIdempotencyOperationRetry   = "workbench.run.retry.v1"
	canonicalRunIdempotencyOperationResume  = "workbench.run.resume.v1"
	canonicalMaxRequestBytes                = 1 << 20
	canonicalMaxRunMessageBytes             = 256 << 10
	canonicalMaxRunPersistedStringBytes     = 32 << 10
	canonicalMaxRunUploadedFileReferences   = 10
)

var canonicalRunDefaults = canonicalRunOptions{
	StreamModes:       []string{"values"},
	MultitaskStrategy: "reject",
	OnDisconnect:      "cancel",
	Durability:        "async",
}

var canonicalRunStreamModeAllowlist = map[string]struct{}{
	"values": {}, "updates": {}, "messages": {}, "messages-tuple": {},
	"custom": {}, "events": {},
}

type canonicalRunOptions struct {
	StreamModes       []string
	MultitaskStrategy string
	OnDisconnect      string
	Durability        string
}

type canonicalCreateRunRequest struct {
	AssistantID       string          `json:"assistant_id"`
	Input             json.RawMessage `json:"input"`
	Command           json.RawMessage `json:"command,omitempty"`
	Metadata          map[string]any  `json:"metadata,omitempty"`
	Config            json.RawMessage `json:"config,omitempty"`
	Context           json.RawMessage `json:"context,omitempty"`
	StreamMode        json.RawMessage `json:"stream_mode,omitempty"`
	MultitaskStrategy string          `json:"multitask_strategy,omitempty"`
	OnDisconnect      string          `json:"on_disconnect,omitempty"`
	Durability        string          `json:"durability,omitempty"`
	StreamResumable   *bool           `json:"stream_resumable,omitempty"`
	StreamSubgraphs   *bool           `json:"stream_subgraphs,omitempty"`
	IfNotExists       string          `json:"if_not_exists,omitempty"`
	RaiseError        json.RawMessage `json:"raise_error,omitempty"`
	Webhook           json.RawMessage `json:"webhook,omitempty"`
	OnCompletion      json.RawMessage `json:"on_completion,omitempty"`
	AfterSeconds      json.RawMessage `json:"after_seconds,omitempty"`
	FeedbackKeys      json.RawMessage `json:"feedback_keys,omitempty"`
	InterruptBefore   json.RawMessage `json:"interrupt_before,omitempty"`
	InterruptAfter    json.RawMessage `json:"interrupt_after,omitempty"`
	Checkpoint        json.RawMessage `json:"checkpoint,omitempty"`
	CheckpointID      json.RawMessage `json:"checkpoint_id,omitempty"`
	LangsmithTracer   json.RawMessage `json:"langsmith_tracer,omitempty"`
	Coze              json.RawMessage `json:"coze,omitempty"`
}

type canonicalRunRequestCoze struct {
	MessageMetadata json.RawMessage `json:"message_metadata,omitempty"`
	AttemptKind     string          `json:"attempt_kind,omitempty"`
	SourceRunID     json.RawMessage `json:"source_run_id,omitempty"`
}

type canonicalRunInput struct {
	Messages      []canonicalRunInputMessage `json:"messages"`
	UploadedFiles json.RawMessage            `json:"uploaded_files,omitempty"`
}

type canonicalRunInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type canonicalRunUploadedFileReference struct {
	FileID json.RawMessage `json:"file_id"`
}

type canonicalRunSubmission struct {
	AssistantID            string
	Input                  string
	Command                string
	Metadata               string
	Config                 string
	Context                string
	MessageContent         string
	MessageMetadata        string
	UploadedFileIDs        []int64
	Options                canonicalRunOptions
	RaiseError             *bool
	IdempotencyKey         string
	IdempotencyOperation   string
	IdempotencyFingerprint string
	AcceptedHeaderKind     string
	Resume                 *canonicalResumeSubmission
	TopLevelRetry          *canonicalTopLevelRetrySubmission
}

type canonicalTopLevelRetrySubmission struct {
	SourceRunID int64
}

type canonicalResumeRunRequest struct {
	InterruptID string                  `json:"interrupt_id"`
	Response    canonicalResumeResponse `json:"response"`
}

type canonicalRunCommand struct {
	Resume *canonicalResumeCommand `json:"resume,omitempty"`
}

type canonicalResumeCommand struct {
	SourceRunID string                  `json:"source_run_id"`
	InterruptID string                  `json:"interrupt_id"`
	Response    canonicalResumeResponse `json:"response"`
}

type canonicalResumeResponse struct {
	Schema        string `json:"schema"`
	InteractionID string `json:"interaction_id"`
	Kind          string `json:"kind"`
	Decision      string `json:"decision"`
	Answer        string `json:"answer,omitempty"`
	ChoiceID      string `json:"choice_id,omitempty"`
	Comment       string `json:"comment,omitempty"`
}

type canonicalResumeSubmission struct {
	SourceRunID            int64
	InterruptID            string
	Response               canonicalResumeResponse
	IdempotencyOperation   string
	IdempotencyFingerprint string
}

type canonicalRunEventPage struct {
	Data             []*canonicalRunEvent `json:"data"`
	HasMore          bool                 `json:"has_more"`
	NextAfterEventID *string              `json:"next_after_event_id,omitempty"`
}

// CreateCanonicalRun serves POST /api/workbench/threads/:thread_id/runs.
// It authorizes the authenticated session principal against the path Thread and any referenced
// source Run; workspace identity comes from those resources, not X-Coze-Space-ID. It calls
// ApplicationService.CreateRun or ResumeHumanInteraction, and returns canonical Run JSON.
func CreateCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.create", "/api/workbench/threads/:thread_id/runs")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	submission, public := parseCanonicalRunSubmission(c, false)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ResponseBodyKind = "run"
	requestLog.StreamModes = strings.Join(submission.Options.StreamModes, ",")
	requestLog.IdempotencyKeyHash = canonicalLogHash(submission.IdempotencyKey)
	submission.IdempotencyKey, public = canonicalPrincipalScopedIdempotencyKey(ctx, submission.IdempotencyKey)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.SubmissionKind = "run_turn"
	if submission.TopLevelRetry != nil {
		requestLog.SubmissionKind = "run_retry"
		requestLog.SourceRunID = submission.TopLevelRetry.SourceRunID
	}
	if submission.Resume != nil {
		requestLog.SubmissionKind = "run_resume"
		requestLog.SourceRunID = submission.Resume.SourceRunID
		run, public, err := resumeCanonicalHumanInteraction(ctx, threadID, submission.Resume, submission.IdempotencyKey)
		if public != nil {
			writeCanonicalError(ctx, c, public.status, *public)
			return
		}
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		projected, err := projectCanonicalRun(run)
		if err != nil || projected == nil {
			if err == nil {
				err = fmt.Errorf("canonical resume projection returned empty run")
			}
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		requestLog.RunID = run.RunID
		c.Header("Content-Location", canonicalRunPath(threadID, run.RunID))
		c.JSON(consts.StatusOK, projected)
		return
	}
	ctx = canonicalThreadAccessContext(ctx, threadID, 0)
	response, public, err := createCanonicalRunBundle(ctx, threadID, submission)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if !canonicalRunCreationResponseValid(response, threadID, submission) {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned invalid run"))
		return
	}
	projected, err := projectCanonicalRun(response.Run)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical run projection returned empty run")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response.Message != nil {
		messageID := strconv.FormatInt(response.Message.MessageID, 10)
		projected.Coze.MessageID = &messageID
		submissionMessage, projectionErr := projectCanonicalMessage(response.Message)
		if projectionErr != nil || submissionMessage == nil {
			if projectionErr == nil {
				projectionErr = fmt.Errorf("canonical create projection returned empty submission message")
			}
			writeCanonicalApplicationError(ctx, c, projectionErr)
			return
		}
		projected.Coze.SubmissionMessage = submissionMessage
	}
	requestLog.RunID = response.Run.RunID
	c.Header("Content-Location", canonicalRunPath(threadID, response.Run.RunID))
	c.JSON(consts.StatusOK, projected)
}

// ListCanonicalRuns serves GET /api/workbench/threads/:thread_id/runs.
// It authorizes the authenticated session principal against the path Thread; workspace identity
// comes from that server-authorized Thread, not X-Coze-Space-ID. It calls
// ApplicationService.SearchRuns, and returns a Run array with pagination headers.
func ListCanonicalRuns(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.list", "/api/workbench/threads/:thread_id/runs")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "run_array"
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
	offset, public := canonicalRunOffset(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	parentRunID, public := canonicalOptionalPositiveQueryID(c, "parent_run_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	var parent *int64
	if parentRunID > 0 {
		parent = &parentRunID
	}
	if selectValue, exists := c.GetQuery("select"); exists && strings.TrimSpace(selectValue) != "" {
		public := canonicalUnsupportedField("select")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	status := strings.TrimSpace(canonicalQueryString(c, "status"))
	if !canonicalRunPublicStatusSupported(status) {
		public := canonicalInvalidRequest("Invalid run status", "invalid_run_status")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalThreadAccessContext(ctx, threadID, 0)
	runs, total, err := searchCanonicalRunProjections(ctx, threadID, parent, status, offset, limit)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	setCanonicalPaginationHeaders(c, total, int(offset), int(limit))
	c.JSON(consts.StatusOK, runs)
}

// GetCanonicalRun serves GET /api/workbench/threads/:thread_id/runs/:run_id.
// It authorizes the authenticated session principal against the path Thread and Run; workspace
// identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.GetRun, and returns canonical Run JSON.
func GetCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.get", "/api/workbench/threads/:thread_id/runs/:run_id")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "run"
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	projected, err := projectCanonicalRun(run)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical run projection returned empty run")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, projected)
}

// WaitCanonicalRun serves POST /api/workbench/threads/:thread_id/runs/wait.
// It authorizes the authenticated session principal against the path Thread and any referenced
// source Run; workspace identity comes from those resources, not X-Coze-Space-ID. It calls
// ApplicationService.CreateRun or ResumeHumanInteraction plus ListCheckpoints, and returns public values JSON.
func WaitCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.wait", "/api/workbench/threads/:thread_id/runs/wait")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	submission, public := parseCanonicalRunSubmission(c, true)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ResponseBodyKind = "values"
	requestLog.StreamModes = strings.Join(submission.Options.StreamModes, ",")
	requestLog.IdempotencyKeyHash = canonicalLogHash(submission.IdempotencyKey)
	submission.IdempotencyKey, public = canonicalPrincipalScopedIdempotencyKey(ctx, submission.IdempotencyKey)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.RaiseErrorMode = "omitted"
	if submission.RaiseError != nil {
		requestLog.RaiseErrorMode = canonicalRaiseErrorMode(submission.RaiseError)
	}
	requestLog.SubmissionKind = "run_turn"
	var (
		run *appagentthread.RunSummary
		err error
	)
	if submission.TopLevelRetry != nil {
		requestLog.SubmissionKind = "run_retry"
		requestLog.SourceRunID = submission.TopLevelRetry.SourceRunID
	}
	if submission.Resume != nil {
		requestLog.SubmissionKind = "run_resume"
		requestLog.SourceRunID = submission.Resume.SourceRunID
		run, public, err = resumeCanonicalHumanInteraction(ctx, threadID, submission.Resume, submission.IdempotencyKey)
		if public != nil {
			writeCanonicalError(ctx, c, public.status, *public)
			return
		}
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
	} else {
		ctx = canonicalThreadAccessContext(ctx, threadID, 0)
		response, public, err := createCanonicalRunBundle(ctx, threadID, submission)
		if public != nil {
			writeCanonicalError(ctx, c, public.status, *public)
			return
		}
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		run = response.Run
	}
	requestLog.RunID = run.RunID
	c.Header("Content-Location", canonicalRunPath(threadID, run.RunID))
	c.Header("Location", canonicalRunJoinPath(threadID, run.RunID))
	requestLog.LocationKind = "run_join"
	ctx = canonicalThreadAccessContext(ctx, threadID, run.RunID)

	run, err = waitCanonicalRunTerminal(
		ctx,
		threadID,
		run,
		submission.Options.OnDisconnect == "cancel",
	)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	values, publicError, err := canonicalRunPublicValues(ctx, run)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if submission.RaiseError != nil && *submission.RaiseError && publicError != nil {
		requestLog.FailureProjection = "http_error"
		public := newCanonicalError(
			consts.StatusInternalServerError,
			publicError.Code,
			publicError.Message,
			"run_failed",
			false,
		)
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if publicError != nil {
		requestLog.FailureProjection = "values_error"
	}
	c.JSON(consts.StatusOK, values)
}

// JoinCanonicalRun serves GET /api/workbench/threads/:thread_id/runs/:run_id/join.
// It authorizes the authenticated session principal against the path Thread and Run; workspace
// identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.GetRun and ListCheckpoints, and returns public values JSON rather than SSE.
func JoinCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.join", "/api/workbench/threads/:thread_id/runs/:run_id/join")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "values"
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	if public := validateCanonicalJoinCancelOnDisconnect(c); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	c.Header("Location", canonicalRunJoinPath(threadID, runID))
	requestLog.LocationKind = "run_join"
	run, err = waitCanonicalRunTerminal(ctx, threadID, run, false)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	values, publicError, err := canonicalRunPublicValues(ctx, run)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if publicError != nil {
		requestLog.FailureProjection = "values_error"
	}
	c.JSON(consts.StatusOK, values)
}

// CancelCanonicalRun serves POST /api/workbench/threads/:thread_id/runs/:run_id/cancel.
// It authorizes the authenticated session principal against the path Thread and Run; workspace
// identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.CancelRun, and returns 204 with an empty body.
func CancelCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.cancel", "/api/workbench/threads/:thread_id/runs/:run_id/cancel")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "empty"
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	wait, public := canonicalCancelRunOptions(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	if isCanonicalCancellationFinal(run.Status) {
		c.Status(consts.StatusNoContent)
		return
	}
	response, err := appagentthread.SVC.CancelRun(ctx, &appagentthread.UpdateRunStatusRequest{
		RunID: runID,
		From:  run.Status,
	})
	if err != nil {
		current, readErr := getCanonicalAuthorizedRun(ctx, threadID, runID)
		if readErr == nil && current != nil && isCanonicalCancellationFinal(current.Status) {
			c.Status(consts.StatusNoContent)
			return
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil || response.Run == nil || response.Run.ThreadID != threadID {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned invalid canceled run"))
		return
	}
	if wait && !isWorkbenchRunTerminal(response.Run.Status) {
		if _, err := waitCanonicalRunTerminal(ctx, threadID, response.Run, false); err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
	}
	c.Status(consts.StatusNoContent)
}

// ResumeCanonicalRun serves POST /api/workbench/threads/:thread_id/runs/:run_id/resume.
// It authorizes the authenticated session principal against the path Thread and source Run;
// workspace identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.ResumeHumanInteraction, and returns the new canonical Run JSON.
func ResumeCanonicalRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.resume", "/api/workbench/threads/:thread_id/runs/:run_id/resume")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "run"
	requestLog.SubmissionKind = "run_resume"
	threadID, sourceRunID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, sourceRunID
	requestLog.SourceRunID = sourceRunID
	if public := canonicalRequestBodyLimit(c, "Run"); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if public := validateCanonicalExecutionControlIngress(
		c.Request.Body(),
		canonicalExecutionControlRootOnly,
	); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	var req canonicalResumeRunRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	submission, public := canonicalResumeSubmissionFromRequest(sourceRunID, req.InterruptID, req.Response)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	idempotencyKey, public := canonicalRunIdempotencyKey(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.IdempotencyKeyHash = canonicalLogHash(idempotencyKey)
	idempotencyKey, public = canonicalPrincipalScopedIdempotencyKey(ctx, idempotencyKey)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	run, public, err := resumeCanonicalHumanInteraction(ctx, threadID, submission, idempotencyKey)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	projected, err := projectCanonicalRun(run)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical resume projection returned empty run")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.Header("Content-Location", canonicalRunPath(threadID, run.RunID))
	c.JSON(consts.StatusOK, projected)
}

// ListCanonicalRunEvents serves GET /api/workbench/threads/:thread_id/runs/:run_id/events.
// It authorizes the authenticated session principal against the path Thread and Run; workspace
// identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.ListRunEventsByCursor, and returns canonical event-page JSON.
func ListCanonicalRunEvents(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.events.list", "/api/workbench/threads/:thread_id/runs/:run_id/events")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	journalRequested := canonicalJournalEventListRequested(c)
	if journalRequested {
		if !requireCanonicalJournalAgentThreadService(ctx, c) {
			return
		}
	} else if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	var ok bool
	if journalRequested {
		ctx, ok = requireCanonicalJournalSpaceAccess(ctx, c)
	} else {
		ctx, ok = requireCanonicalSpaceAccess(ctx, c)
	}
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "event_page"
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		if journalRequested {
			writeCanonicalJournalError(ctx, c, public.status, *public)
		} else {
			writeCanonicalError(ctx, c, public.status, *public)
		}
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	if journalRequested {
		listCanonicalRunJournalEvents(ctx, c, requestLog, threadID, runID)
		return
	}
	afterEventID, public := canonicalNonNegativeQueryID(c, "after_event_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.AfterEventID = afterEventID
	eventTypes, public := canonicalRunEventTypes(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	limit, public := canonicalQueryLimit(c, "limit", 20)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	response, err := appagentthread.SVC.ListRunEventsByCursor(ctx, &appagentthread.ListRunEventsByCursorRequest{
		ThreadID: threadID, RunID: runID, AfterEventID: afterEventID,
		EventTypes: eventTypes, Limit: limit,
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if response == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty event page"))
		return
	}
	setCanonicalPaginationTotal(c, response.Total)
	page := canonicalRunEventPage{Data: []*canonicalRunEvent{}, HasMore: response.HasMore}
	for _, event := range response.Events {
		if event == nil || event.ThreadID != threadID || event.RunID != runID {
			continue
		}
		projected, err := projectCanonicalRunEvent(event)
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		if projected != nil {
			page.Data = append(page.Data, projected)
		}
	}
	if page.HasMore && len(page.Data) > 0 {
		next := page.Data[len(page.Data)-1].EventID
		page.NextAfterEventID = &next
	}
	c.JSON(consts.StatusOK, page)
}

// ListCanonicalRunMessages serves GET /api/workbench/threads/:thread_id/runs/:run_id/messages.
// It authorizes the authenticated session principal against the path Thread and Run; workspace
// identity comes from those server-authorized resources, not X-Coze-Space-ID. It calls
// ApplicationService.SearchRuns, ListMessages, and ListRunEvents, and returns canonical message-page JSON.
func ListCanonicalRunMessages(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("run.messages.list", "/api/workbench/threads/:thread_id/runs/:run_id/messages")
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	requestLog.ResponseBodyKind = "message_page"
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
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
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalResourceNotFound(ctx, c)
		return
	}
	messages, err := loadCanonicalThreadMessages(ctx, threadID)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	runIDString := strconv.FormatInt(runID, 10)
	filtered := make([]*canonicalMessage, 0)
	for _, message := range messages {
		if message != nil && message.RunID == runIDString {
			filtered = append(filtered, message)
		}
	}
	c.JSON(consts.StatusOK, pageCanonicalRunMessages(filtered, before, after, limit))
}

func createCanonicalRunBundle(
	ctx context.Context,
	threadID int64,
	submission *canonicalRunSubmission,
) (*appagentthread.CreateRunResponse, *canonicalError, error) {
	replayed, err := replayCanonicalRunBundle(ctx, threadID, submission)
	if err != nil || replayed != nil {
		return replayed, nil, err
	}
	if public, err := resolveCanonicalRunInput(ctx, threadID, submission); public != nil || err != nil {
		return nil, public, err
	}
	response, err := appagentthread.SVC.CreateRun(ctx, canonicalApplicationCreateRunRequest(threadID, submission))
	if err != nil {
		return nil, nil, err
	}
	if !canonicalRunCreationResponseValid(response, threadID, submission) {
		return nil, nil, fmt.Errorf("agent thread application returned invalid run")
	}
	return response, nil, nil
}

func canonicalRunCreationResponseValid(
	response *appagentthread.CreateRunResponse,
	threadID int64,
	submission *canonicalRunSubmission,
) bool {
	if response == nil || response.Run == nil || submission == nil || response.Run.ThreadID != threadID {
		return false
	}
	if submission.TopLevelRetry != nil {
		return response.Message == nil
	}
	return response.Message != nil && response.Message.ThreadID == threadID &&
		response.Message.RunID == response.Run.RunID
}

// replayCanonicalRunBundle resolves an existing idempotent Run before checking
// mutable upload state. This preserves the first committed result when a file is
// deleted after creation, while all ownership lookup remains in the application
// layer. Requests without an idempotency key take the normal creation path.
func replayCanonicalRunBundle(
	ctx context.Context,
	threadID int64,
	submission *canonicalRunSubmission,
) (*appagentthread.CreateRunResponse, error) {
	if submission == nil || strings.TrimSpace(submission.IdempotencyKey) == "" {
		return nil, nil
	}
	response, err := appagentthread.SVC.GetRunByIdempotencyKey(
		ctx,
		&appagentthread.GetRunByIdempotencyKeyRequest{
			ThreadID: threadID, IdempotencyKey: submission.IdempotencyKey,
			IdempotencyOperation:   submission.IdempotencyOperation,
			IdempotencyFingerprint: submission.IdempotencyFingerprint,
		},
	)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Run == nil {
		return nil, nil
	}
	if submission.TopLevelRetry != nil {
		return &appagentthread.CreateRunResponse{Run: response.Run}, nil
	}
	message, err := getCanonicalRunUserMessage(ctx, threadID, response.Run.RunID)
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, fmt.Errorf("idempotent canonical run is missing its user message")
	}
	return &appagentthread.CreateRunResponse{Run: response.Run, Message: message}, nil
}

func getCanonicalRunUserMessage(
	ctx context.Context,
	threadID int64,
	runID int64,
) (*appagentthread.MessageSummary, error) {
	const pageSize int32 = 100
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
		for _, message := range response.Messages {
			if message != nil && message.ThreadID == threadID && message.RunID == runID &&
				message.Role == appagentthread.MessageRoleUser {
				return message, nil
			}
		}
		if len(response.Messages) == 0 || int64(page)*int64(pageSize) >= response.Total {
			return nil, nil
		}
	}
}

// resolveCanonicalRunInput replaces client file references with the authoritative
// summaries registered for the authenticated principal and path Thread. This keeps
// file names, virtual paths, sizes, and ownership out of the caller's trust boundary.
func resolveCanonicalRunInput(
	ctx context.Context,
	threadID int64,
	submission *canonicalRunSubmission,
) (*canonicalError, error) {
	if submission == nil {
		return canonicalInvalidRequest("Run request is required", "missing_request"), nil
	}
	if len(submission.UploadedFileIDs) == 0 {
		return nil, setCanonicalResolvedRunInput(submission, []any{})
	}

	threadResponse, err := appagentthread.SVC.GetThread(ctx, &appagentthread.GetThreadRequest{ThreadID: threadID})
	if err != nil {
		return nil, err
	}
	if threadResponse == nil || threadResponse.Thread == nil || threadResponse.Thread.ThreadID != threadID {
		return newCanonicalError(
			consts.StatusNotFound,
			"resource_not_found",
			"Resource not found",
			"run_thread_not_found",
			false,
		), nil
	}
	filesResponse, err := appagentthread.SVC.ListTaskThreadUploadFiles(
		ctx,
		&appagentthread.ListTaskThreadUploadFilesRequest{
			SpaceID:  threadResponse.Thread.SpaceID,
			UserID:   workbenchViewerIDFromCtx(ctx),
			ThreadID: threadID,
		},
	)
	if err != nil {
		return nil, err
	}
	if filesResponse == nil {
		return nil, fmt.Errorf("agent thread application returned empty upload list")
	}
	available := make(map[int64]*appagentthread.TaskThreadUploadedFileSummary, len(filesResponse.Files))
	for _, file := range filesResponse.Files {
		if file != nil && file.FileID > 0 {
			available[file.FileID] = file
		}
	}
	selected := make([]*appagentthread.TaskThreadUploadedFileSummary, 0, len(submission.UploadedFileIDs))
	for _, fileID := range submission.UploadedFileIDs {
		file := available[fileID]
		if file == nil {
			return canonicalInvalidRequest(
				"Uploaded file is not available for this Thread",
				"invalid_uploaded_file",
			), nil
		}
		selected = append(selected, file)
	}
	return nil, setCanonicalResolvedRunInput(submission, selected)
}

func setCanonicalResolvedRunInput(submission *canonicalRunSubmission, uploadedFiles any) error {
	if submission == nil {
		return fmt.Errorf("canonical run submission is required")
	}
	payload := map[string]any{"uploaded_files": uploadedFiles}
	if submission.TopLevelRetry != nil {
		payload["messages"] = []canonicalRunInputMessage{{
			Role: "user", Content: submission.MessageContent,
		}}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal canonical run input: %w", err)
	}
	submission.Input = string(encoded)
	return nil
}

func resumeCanonicalHumanInteraction(
	ctx context.Context,
	threadID int64,
	submission *canonicalResumeSubmission,
	idempotencyKey string,
) (*appagentthread.RunSummary, *canonicalError, error) {
	if submission == nil {
		return nil, canonicalInvalidRequest("Resume request is invalid", "invalid_resume"), nil
	}
	accessCtx := canonicalThreadAccessContext(ctx, threadID, submission.SourceRunID)
	sourceRun, err := getCanonicalAuthorizedRun(accessCtx, threadID, submission.SourceRunID)
	if err != nil {
		return nil, nil, err
	}
	if sourceRun == nil {
		return nil, newCanonicalError(
			consts.StatusNotFound,
			"resource_not_found",
			"Resource not found",
			"resume_source_not_found",
			false,
		), nil
	}
	if sourceRun.Status != appagentthread.RunStatusInterrupted {
		return nil, newCanonicalError(
			consts.StatusConflict,
			"run_not_resumable",
			"Run is not resumable",
			"resume_source_not_interrupted",
			false,
		), nil
	}
	// ResumeHumanInteraction keeps its existing application semantics for current
	// callers. Canonical requests add this authorized lookup so a header key already
	// owned by another Thread is reported as the public 409 contract, rather than
	// being mistaken for an invalid resumed Run after the application call.
	if strings.TrimSpace(idempotencyKey) != "" {
		if _, err := appagentthread.SVC.GetRunByIdempotencyKey(
			accessCtx,
			&appagentthread.GetRunByIdempotencyKeyRequest{
				ThreadID: threadID, IdempotencyKey: idempotencyKey,
				IdempotencyOperation:   submission.IdempotencyOperation,
				IdempotencyFingerprint: submission.IdempotencyFingerprint,
			},
		); err != nil {
			return nil, nil, err
		}
	}
	response := submission.Response
	resumed, err := appagentthread.SVC.ResumeHumanInteraction(accessCtx, &appagentthread.ResumeHumanInteractionRequest{
		ThreadID:                threadID,
		SourceRunID:             submission.SourceRunID,
		InterruptID:             submission.InterruptID,
		IdempotencyKey:          idempotencyKey,
		IdempotencyOperation:    submission.IdempotencyOperation,
		IdempotencyFingerprint:  submission.IdempotencyFingerprint,
		PersistMessageReference: true,
		Response: appagentthread.HumanInteractionResponse{
			Schema:        response.Schema,
			InteractionID: response.InteractionID,
			Kind:          appagentthread.HumanInteractionKind(response.Kind),
			Decision:      appagentthread.HumanInteractionDecision(response.Decision),
			Answer:        response.Answer,
			ChoiceID:      response.ChoiceID,
			Comment:       response.Comment,
			SubmittedBy:   strconv.FormatInt(workbenchViewerIDFromCtx(ctx), 10),
			Source:        "canonical_api",
		},
	})
	if err != nil {
		return nil, nil, err
	}
	if resumed == nil || resumed.Run == nil || resumed.Run.ThreadID != threadID ||
		resumed.Run.RunID == submission.SourceRunID {
		return nil, nil, fmt.Errorf("agent thread application returned invalid resumed run")
	}
	return resumed.Run, nil, nil
}

func waitCanonicalRunTerminal(
	ctx context.Context,
	threadID int64,
	run *appagentthread.RunSummary,
	cancelOnDisconnect bool,
) (*appagentthread.RunSummary, error) {
	if run == nil || isWorkbenchRunTerminal(run.Status) {
		return run, nil
	}
	ticker := time.NewTicker(time.Duration(defaultRunEventStreamIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if cancelOnDisconnect {
				cancelCanonicalRunAfterWaitDisconnect(ctx, run.RunID)
			}
			return run, ctx.Err()
		case <-ticker.C:
			current, err := getCanonicalAuthorizedRun(ctx, threadID, run.RunID)
			if err != nil {
				return nil, err
			}
			if current == nil {
				return run, nil
			}
			run = current
			if isWorkbenchRunTerminal(run.Status) {
				return run, nil
			}
		}
	}
}

func cancelCanonicalRunAfterWaitDisconnect(ctx context.Context, runID int64) {
	if runID <= 0 {
		return
	}
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runStreamDisconnectCancelTimeout)
	defer cancel()

	if _, err := appagentthread.SVC.CancelRunOnDisconnect(cancelCtx, &appagentthread.CancelRunOnDisconnectRequest{
		RunID: runID,
	}); err != nil {
		errorCode, errorClass := canonicalErrorLogFields(err)
		logs.CtxWarnf(
			cancelCtx,
			"event_name=workbench.run.disconnect_cancel_failed client_contract=%s disconnect_source=canonical_wait run_id=%d error_code=%s error_class=%s",
			canonicalContractVersion,
			runID,
			errorCode,
			errorClass,
		)
	}
}

func isCanonicalCancellationFinal(status appagentthread.RunStatus) bool {
	return status == appagentthread.RunStatusSucceeded ||
		status == appagentthread.RunStatusFailed ||
		status == appagentthread.RunStatusCanceled
}

func canonicalRunPublicValues(
	ctx context.Context,
	run *appagentthread.RunSummary,
) (map[string]any, *appagentthread.PublicRuntimeError, error) {
	values := map[string]any{}
	if run == nil {
		return values, nil, nil
	}
	response, err := appagentthread.SVC.ListCheckpoints(ctx, &appagentthread.ListCheckpointsRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Limit:    1,
	})
	if err != nil {
		return nil, nil, err
	}
	if response != nil && len(response.Checkpoints) > 0 {
		if checkpoint := appagentthread.ProjectPublicCheckpoint(response.Checkpoints[0]); checkpoint != nil {
			for key, value := range checkpoint.Values {
				values[key] = value
			}
		}
	}
	if run.Status != appagentthread.RunStatusFailed {
		return values, nil, nil
	}
	publicError := appagentthread.ProjectPublicRuntimeError(run.ErrorCode, run.ErrorMessage)
	if publicError == nil {
		publicError = &appagentthread.PublicRuntimeError{Code: "runtime_failed", Message: "Agent run failed"}
	}
	values["__error__"] = map[string]any{
		"error":   publicError.Code,
		"message": publicError.Message,
	}
	return values, publicError, nil
}

func validateCanonicalJoinCancelOnDisconnect(c *app.RequestContext) *canonicalError {
	raw, exists := c.GetQuery("cancel_on_disconnect")
	if !exists {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "false", "0":
		return nil
	default:
		return canonicalUnsupportedField("cancel_on_disconnect")
	}
}

func canonicalCancelRunOptions(c *app.RequestContext) (bool, *canonicalError) {
	action := strings.ToLower(strings.TrimSpace(canonicalQueryString(c, "action")))
	if action != "" && action != "interrupt" {
		return false, canonicalUnsupportedField("action")
	}
	wait := strings.TrimSpace(canonicalQueryString(c, "wait"))
	switch wait {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, canonicalUnsupportedField("wait")
	}
}

func canonicalNonNegativeQueryID(c *app.RequestContext, name string) (int64, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value < 0 {
		return 0, canonicalInvalidRequest("Invalid query parameter: "+name, "invalid_event_cursor")
	}
	return value, nil
}

func canonicalRunEventTypes(c *app.RequestContext) ([]string, *canonicalError) {
	values := c.QueryArgs().PeekAll("event_types")
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, item := range strings.Split(string(value), ",") {
			eventType := strings.TrimSpace(item)
			if eventType == "" {
				continue
			}
			if len(eventType) > 128 || strings.IndexFunc(eventType, func(r rune) bool {
				return r < 0x21 || r > 0x7e
			}) >= 0 {
				return nil, canonicalInvalidRequest("event_types is invalid", "invalid_event_types")
			}
			if _, exists := seen[eventType]; exists {
				continue
			}
			seen[eventType] = struct{}{}
			result = append(result, eventType)
			if len(result) > 32 {
				return nil, canonicalInvalidRequest("event_types is too large", "invalid_event_types")
			}
		}
	}
	return result, nil
}

func pageCanonicalRunMessages(
	messages []*canonicalMessage,
	before, after int64,
	limit int32,
) canonicalMessagePage {
	candidates := make([]*canonicalMessage, 0, len(messages))
	for _, message := range messages {
		seq := canonicalMessageSequence(message)
		if seq <= 0 || (before > 0 && seq >= before) || (after > 0 && seq <= after) {
			continue
		}
		candidates = append(candidates, message)
	}
	page := canonicalMessagePage{Data: []*canonicalMessage{}}
	if before > 0 && len(candidates) > int(limit) {
		page.HasMore = true
		candidates = candidates[len(candidates)-int(limit):]
	} else if before == 0 && len(candidates) > int(limit) {
		page.HasMore = true
		candidates = candidates[:limit]
	}
	page.Data = append(page.Data, candidates...)
	if len(page.Data) == 0 {
		return page
	}
	first := canonicalMessageSequence(page.Data[0])
	last := canonicalMessageSequence(page.Data[len(page.Data)-1])
	for _, message := range messages {
		seq := canonicalMessageSequence(message)
		if seq > 0 && seq < first {
			next := strconv.FormatInt(first, 10)
			page.NextBeforeSeq = &next
		}
		if seq > last {
			next := strconv.FormatInt(last, 10)
			page.NextAfterSeq = &next
		}
	}
	return page
}

func canonicalMessageSequence(message *canonicalMessage) int64 {
	if message == nil {
		return 0
	}
	value, err := strconv.ParseInt(message.Seq, 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func canonicalRunPathIDs(c *app.RequestContext) (int64, int64, *canonicalError) {
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		return 0, 0, public
	}
	runID, public := canonicalPathID(c, "run_id")
	if public != nil {
		return 0, 0, public
	}
	return threadID, runID, nil
}

func getCanonicalAuthorizedRun(
	ctx context.Context,
	threadID, runID int64,
) (*appagentthread.RunSummary, error) {
	response, err := appagentthread.SVC.GetRun(ctx, &appagentthread.GetRunRequest{RunID: runID})
	if err != nil {
		return nil, err
	}
	if response == nil || response.Run == nil || response.Run.ThreadID != threadID {
		return nil, nil
	}
	return response.Run, nil
}

func searchCanonicalRunProjections(
	ctx context.Context,
	threadID int64,
	parentRunID *int64,
	status string,
	offset, limit int32,
) ([]*canonicalRun, int64, error) {
	if status == "" {
		response, err := appagentthread.SVC.SearchRuns(ctx, &appagentthread.SearchRunsRequest{
			ThreadID: threadID, ParentRunID: parentRunID,
			Page: appagentthread.CanonicalPage{Offset: offset, Limit: limit},
		})
		if err != nil {
			return nil, 0, err
		}
		if response == nil {
			return nil, 0, fmt.Errorf("agent thread application returned empty run search")
		}
		projected, err := projectCanonicalRuns(response.Runs)
		return projected, response.Total, err
	}

	const pageSize = int32(100)
	matching := make([]*canonicalRun, 0)
	for pageOffset := int32(0); ; pageOffset += pageSize {
		response, err := appagentthread.SVC.SearchRuns(ctx, &appagentthread.SearchRunsRequest{
			ThreadID: threadID, ParentRunID: parentRunID,
			Page: appagentthread.CanonicalPage{Offset: pageOffset, Limit: pageSize},
		})
		if err != nil {
			return nil, 0, err
		}
		if response == nil {
			return nil, 0, fmt.Errorf("agent thread application returned empty run search")
		}
		projected, err := projectCanonicalRuns(response.Runs)
		if err != nil {
			return nil, 0, err
		}
		for _, run := range projected {
			if run != nil && run.Status == status {
				matching = append(matching, run)
			}
		}
		if len(response.Runs) == 0 || int64(pageOffset)+int64(len(response.Runs)) >= response.Total {
			break
		}
		if pageOffset > int32(^uint32(0)>>1)-pageSize {
			return nil, 0, fmt.Errorf("canonical run search is too large")
		}
	}
	total := int64(len(matching))
	start := int(offset)
	if start > len(matching) {
		start = len(matching)
	}
	end := start + int(limit)
	if end > len(matching) {
		end = len(matching)
	}
	return matching[start:end], total, nil
}

func projectCanonicalRuns(summaries []*appagentthread.RunSummary) ([]*canonicalRun, error) {
	result := make([]*canonicalRun, 0, len(summaries))
	for _, summary := range summaries {
		projected, err := projectCanonicalRun(summary)
		if err != nil {
			return nil, err
		}
		if projected != nil {
			result = append(result, projected)
		}
	}
	return result, nil
}

func canonicalRunPublicStatusSupported(status string) bool {
	switch status {
	case "", "pending", "running", "interrupted", "success", "error":
		return true
	default:
		return false
	}
}

func canonicalRunOffset(c *app.RequestContext) (int32, *canonicalError) {
	raw, exists := c.GetQuery("offset")
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 0 {
		return 0, canonicalInvalidRequest("Invalid query parameter: offset", "invalid_pagination")
	}
	return int32(value), nil
}

func parseCanonicalRunSubmission(
	c *app.RequestContext,
	allowRaiseError bool,
) (*canonicalRunSubmission, *canonicalError) {
	if public := canonicalRequestBodyLimit(c, "Run"); public != nil {
		return nil, public
	}
	if public := validateCanonicalExecutionControlIngress(
		c.Request.Body(),
		canonicalExecutionControlRunSubmission,
	); public != nil {
		return nil, public
	}
	var req canonicalCreateRunRequest
	if public := decodeCanonicalJSON(c, &req); public != nil {
		return nil, public
	}
	raiseError, public := canonicalRunRaiseError(req.RaiseError, allowRaiseError)
	if public != nil {
		return nil, public
	}
	options, public := validateCanonicalRunOptions(&req)
	if public != nil {
		return nil, public
	}
	assistantID := strings.TrimSpace(req.AssistantID)
	if assistantID != canonicalPublicAssistantID {
		return nil, canonicalInvalidRequest("assistant_id is invalid", "invalid_assistant_id")
	}
	command, resume, public := canonicalRunCommandPayload(req.Command)
	if public != nil {
		return nil, public
	}
	if resume != nil && len(bytes.TrimSpace(req.Coze)) > 0 {
		return nil, canonicalUnsupportedField("coze")
	}
	messageMetadata, topLevelRetry, public := canonicalRunCozePayload(req.Coze)
	if public != nil {
		return nil, public
	}
	messageContent, input := "", ""
	var uploadedFileIDs []int64
	if resume == nil {
		messageContent, uploadedFileIDs, public = canonicalRunInputPayload(req.Input)
		if public != nil {
			return nil, public
		}
		input = `{"uploaded_files":[]}`
	} else if !canonicalRawJSONNullOrOmitted(req.Input) {
		return nil, canonicalUnsupportedField("input")
	}
	config, public := canonicalJSONObjectPayload(req.Config, "config")
	if public != nil {
		return nil, public
	}
	runContext, public := canonicalJSONObjectPayload(req.Context, "context")
	if public != nil {
		return nil, public
	}
	metadata, public := canonicalRunMetadataPayload(req.Metadata)
	if public != nil {
		return nil, public
	}
	if resume != nil {
		switch {
		case metadata != "":
			return nil, canonicalUnsupportedField("metadata")
		case config != "":
			return nil, canonicalUnsupportedField("config")
		case runContext != "":
			return nil, canonicalUnsupportedField("context")
		}
	}
	idempotencyKey, public := canonicalRunIdempotencyKey(c)
	if public != nil {
		return nil, public
	}
	headerKind := "omitted"
	if idempotencyKey != "" {
		headerKind = "present"
	}
	submission := &canonicalRunSubmission{
		AssistantID: assistantID, Input: input, Command: command,
		Metadata: metadata, Config: config, Context: runContext,
		MessageContent: messageContent, MessageMetadata: messageMetadata,
		UploadedFileIDs: uploadedFileIDs, Options: options,
		RaiseError: raiseError, IdempotencyKey: idempotencyKey,
		AcceptedHeaderKind: headerKind,
		Resume:             resume, TopLevelRetry: topLevelRetry,
	}
	if resume != nil {
		submission.IdempotencyOperation = resume.IdempotencyOperation
		submission.IdempotencyFingerprint = resume.IdempotencyFingerprint
	} else if topLevelRetry != nil && idempotencyKey != "" {
		submission.IdempotencyOperation = canonicalRunIdempotencyOperationRetry
		submission.IdempotencyFingerprint = canonicalRunRetryRequestFingerprint(submission)
	} else if idempotencyKey != "" {
		submission.IdempotencyOperation = canonicalRunIdempotencyOperationTurn
		submission.IdempotencyFingerprint = canonicalRunTurnRequestFingerprint(submission)
	}
	return submission, nil
}

func canonicalRunCozePayload(
	raw json.RawMessage,
) (string, *canonicalTopLevelRetrySubmission, *canonicalError) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil, canonicalInvalidRequest("coze must be a JSON object", "invalid_coze")
	}
	var extension canonicalRunRequestCoze
	if public := decodeCanonicalRawJSONObject(raw, &extension, "coze"); public != nil {
		return "", nil, public
	}

	messageMetadataProvided := len(bytes.TrimSpace(extension.MessageMetadata)) > 0
	messageMetadata := ""
	if messageMetadataProvided {
		if bytes.Equal(bytes.TrimSpace(extension.MessageMetadata), []byte("null")) {
			return "", nil, canonicalInvalidRequest(
				"coze.message_metadata must be a JSON object",
				"invalid_coze_message_metadata",
			)
		}
		var public *canonicalError
		messageMetadata, public = canonicalJSONObjectPayload(
			extension.MessageMetadata,
			"coze.message_metadata",
		)
		if public != nil {
			return "", nil, public
		}
	}

	attemptKind := strings.TrimSpace(extension.AttemptKind)
	sourceProvided := len(bytes.TrimSpace(extension.SourceRunID)) > 0
	if attemptKind == "" {
		if sourceProvided {
			return "", nil, canonicalUnsupportedField("coze.source_run_id")
		}
		return messageMetadata, nil, nil
	}
	if attemptKind != "retry" {
		return "", nil, canonicalInvalidRequest(
			"coze.attempt_kind is invalid",
			"invalid_coze_attempt_kind",
		)
	}
	if messageMetadataProvided {
		return "", nil, canonicalUnsupportedField("coze.message_metadata")
	}
	sourceRunID, public := canonicalTopLevelRetrySourceRunID(extension.SourceRunID)
	if public != nil {
		return "", nil, public
	}
	return "", &canonicalTopLevelRetrySubmission{SourceRunID: sourceRunID}, nil
}

func canonicalTopLevelRetrySourceRunID(raw json.RawMessage) (int64, *canonicalError) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || trimmed[0] != '"' {
		return 0, canonicalInvalidRequest(
			"coze.source_run_id must be a positive decimal string",
			"invalid_source_run_id",
		)
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return 0, canonicalInvalidRequest("coze.source_run_id is invalid", "invalid_source_run_id")
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, canonicalInvalidRequest("coze.source_run_id is invalid", "invalid_source_run_id")
		}
	}
	sourceRunID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || sourceRunID <= 0 {
		return 0, canonicalInvalidRequest("coze.source_run_id is invalid", "invalid_source_run_id")
	}
	return sourceRunID, nil
}

func canonicalRequestBodyLimit(c *app.RequestContext, resource string) *canonicalError {
	if c != nil && len(c.Request.Body()) > canonicalMaxRequestBytes {
		return newCanonicalError(
			consts.StatusRequestEntityTooLarge,
			"request_too_large",
			"Request body exceeds the canonical "+resource+" limit",
			"request_body_too_large",
			false,
		)
	}
	return nil
}

func canonicalRunRaiseError(raw json.RawMessage, allowed bool) (*bool, *canonicalError) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	if !allowed {
		return nil, canonicalUnsupportedField("raise_error")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, canonicalInvalidRequest("raise_error must be a boolean", "invalid_raise_error")
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, canonicalInvalidRequest("raise_error must be a boolean", "invalid_raise_error")
	}
	return &value, nil
}

func canonicalRunCommandPayload(
	raw json.RawMessage,
) (string, *canonicalResumeSubmission, *canonicalError) {
	if canonicalRawJSONNullOrOmitted(raw) {
		return "", nil, nil
	}
	var command canonicalRunCommand
	if public := decodeCanonicalRawJSONObject(raw, &command, "command"); public != nil {
		return "", nil, public
	}
	encoded, err := json.Marshal(command)
	if err != nil {
		return "", nil, canonicalInvalidRequest("command is invalid", "invalid_command")
	}
	if command.Resume == nil {
		return string(encoded), nil, nil
	}
	sourceRunID, err := strconv.ParseInt(strings.TrimSpace(command.Resume.SourceRunID), 10, 64)
	if err != nil || sourceRunID <= 0 {
		return "", nil, canonicalInvalidRequest("command.resume.source_run_id is invalid", "invalid_source_run_id")
	}
	resume, public := canonicalResumeSubmissionFromRequest(
		sourceRunID,
		command.Resume.InterruptID,
		command.Resume.Response,
	)
	if public != nil {
		return "", nil, public
	}
	return string(encoded), resume, nil
}

func canonicalResumeSubmissionFromRequest(
	sourceRunID int64,
	interruptID string,
	response canonicalResumeResponse,
) (*canonicalResumeSubmission, *canonicalError) {
	interruptID = strings.TrimSpace(interruptID)
	response.Schema = strings.TrimSpace(response.Schema)
	response.InteractionID = strings.TrimSpace(response.InteractionID)
	response.Kind = strings.TrimSpace(response.Kind)
	response.Decision = strings.TrimSpace(response.Decision)
	response.Answer = strings.TrimSpace(response.Answer)
	response.ChoiceID = strings.TrimSpace(response.ChoiceID)
	response.Comment = strings.TrimSpace(response.Comment)
	if sourceRunID <= 0 || interruptID == "" || response.Schema == "" ||
		response.InteractionID == "" || response.Kind == "" || response.Decision == "" {
		return nil, canonicalInvalidRequest("Resume request is invalid", "invalid_resume")
	}
	return &canonicalResumeSubmission{
		SourceRunID:          sourceRunID,
		InterruptID:          interruptID,
		Response:             response,
		IdempotencyOperation: canonicalRunIdempotencyOperationResume,
		IdempotencyFingerprint: canonicalResumeRequestFingerprint(
			sourceRunID,
			interruptID,
			response,
		),
	}, nil
}

func canonicalRunIdempotencyKey(c *app.RequestContext) (string, *canonicalError) {
	value := strings.TrimSpace(string(c.GetHeader("Idempotency-Key")))
	if len(value) > 128 {
		return "", canonicalInvalidRequest("Idempotency-Key is invalid", "invalid_idempotency_key")
	}
	return value, nil
}

// canonicalPrincipalScopedIdempotencyKey adapts the public client key to the
// existing (space_id, idempotency_key) unique index. The principal is included
// in a stable digest so two workspace members cannot reserve each other's key;
// operation remains in the persisted fingerprint contract so turn/resume reuse
// is still rejected instead of becoming an independent record.
func canonicalPrincipalScopedIdempotencyKey(
	ctx context.Context,
	clientKey string,
) (string, *canonicalError) {
	clientKey = strings.TrimSpace(clientKey)
	if clientKey == "" {
		return "", nil
	}
	principalID := workbenchViewerIDFromCtx(ctx)
	if principalID <= 0 {
		return "", newCanonicalError(
			consts.StatusUnauthorized,
			"unauthenticated",
			"Authentication required",
			"unauthenticated",
			false,
		)
	}
	return canonicalScopedIdempotencyKey(principalID, clientKey), nil
}

func canonicalScopedIdempotencyKey(principalID int64, clientKey string) string {
	payload := "session\x00" + strconv.FormatInt(principalID, 10) + "\x00" + strings.TrimSpace(clientKey)
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("canonical:v1:%x", sum[:])
}

func canonicalRunTurnRequestFingerprint(submission *canonicalRunSubmission) string {
	if submission == nil {
		return ""
	}
	payload := struct {
		Version           string   `json:"version"`
		Operation         string   `json:"operation"`
		AssistantID       string   `json:"assistant_id"`
		MessageContent    string   `json:"message_content"`
		MessageMetadata   string   `json:"message_metadata"`
		UploadedFileIDs   []int64  `json:"uploaded_file_ids"`
		Metadata          string   `json:"metadata"`
		Config            string   `json:"config"`
		Context           string   `json:"context"`
		StreamModes       []string `json:"stream_modes"`
		MultitaskStrategy string   `json:"multitask_strategy"`
		OnDisconnect      string   `json:"on_disconnect"`
		Durability        string   `json:"durability"`
	}{
		Version: "v1", Operation: canonicalRunIdempotencyOperationTurn,
		AssistantID: submission.AssistantID, MessageContent: submission.MessageContent,
		MessageMetadata:   canonicalRunFingerprintJSONObject(submission.MessageMetadata),
		UploadedFileIDs:   append([]int64{}, submission.UploadedFileIDs...),
		Metadata:          canonicalRunFingerprintJSONObject(submission.Metadata),
		Config:            canonicalRunFingerprintJSONObject(submission.Config),
		Context:           canonicalRunFingerprintJSONObject(submission.Context),
		StreamModes:       append([]string{}, submission.Options.StreamModes...),
		MultitaskStrategy: submission.Options.MultitaskStrategy,
		OnDisconnect:      submission.Options.OnDisconnect, Durability: submission.Options.Durability,
	}
	return canonicalRunRequestFingerprint(payload)
}

func canonicalRunRetryRequestFingerprint(submission *canonicalRunSubmission) string {
	if submission == nil || submission.TopLevelRetry == nil {
		return ""
	}
	payload := struct {
		Version           string   `json:"version"`
		Operation         string   `json:"operation"`
		SourceRunID       int64    `json:"source_run_id"`
		AssistantID       string   `json:"assistant_id"`
		MessageContent    string   `json:"message_content"`
		UploadedFileIDs   []int64  `json:"uploaded_file_ids"`
		Command           string   `json:"command"`
		Metadata          string   `json:"metadata"`
		Config            string   `json:"config"`
		Context           string   `json:"context"`
		StreamModes       []string `json:"stream_modes"`
		MultitaskStrategy string   `json:"multitask_strategy"`
		OnDisconnect      string   `json:"on_disconnect"`
		Durability        string   `json:"durability"`
	}{
		Version: "v1", Operation: canonicalRunIdempotencyOperationRetry,
		SourceRunID: submission.TopLevelRetry.SourceRunID,
		AssistantID: submission.AssistantID, MessageContent: submission.MessageContent,
		UploadedFileIDs:   append([]int64{}, submission.UploadedFileIDs...),
		Command:           canonicalRunFingerprintJSONObject(submission.Command),
		Metadata:          canonicalRunFingerprintJSONObject(submission.Metadata),
		Config:            canonicalRunFingerprintJSONObject(submission.Config),
		Context:           canonicalRunFingerprintJSONObject(submission.Context),
		StreamModes:       append([]string{}, submission.Options.StreamModes...),
		MultitaskStrategy: submission.Options.MultitaskStrategy,
		OnDisconnect:      submission.Options.OnDisconnect, Durability: submission.Options.Durability,
	}
	return canonicalRunRequestFingerprint(payload)
}

func canonicalResumeRequestFingerprint(
	sourceRunID int64,
	interruptID string,
	response canonicalResumeResponse,
) string {
	payload := struct {
		Version     string                  `json:"version"`
		Operation   string                  `json:"operation"`
		SourceRunID int64                   `json:"source_run_id"`
		InterruptID string                  `json:"interrupt_id"`
		Response    canonicalResumeResponse `json:"response"`
	}{
		Version: "v1", Operation: canonicalRunIdempotencyOperationResume,
		SourceRunID: sourceRunID, InterruptID: interruptID, Response: response,
	}
	return canonicalRunRequestFingerprint(payload)
}

func canonicalRunRequestFingerprint(payload any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func canonicalRunFingerprintJSONObject(value string) string {
	if strings.TrimSpace(value) == "" {
		return `{}`
	}
	return value
}

func validateCanonicalRunOptions(req *canonicalCreateRunRequest) (canonicalRunOptions, *canonicalError) {
	if req == nil {
		return canonicalRunOptions{}, canonicalInvalidRequest("Run request is required", "missing_request")
	}
	for field, raw := range map[string]json.RawMessage{
		"webhook": req.Webhook, "on_completion": req.OnCompletion,
		"after_seconds": req.AfterSeconds, "feedback_keys": req.FeedbackKeys,
		"interrupt_before": req.InterruptBefore, "interrupt_after": req.InterruptAfter,
		"checkpoint": req.Checkpoint, "checkpoint_id": req.CheckpointID,
		"langsmith_tracer": req.LangsmithTracer,
	} {
		if !canonicalRawJSONNullOrOmitted(raw) {
			return canonicalRunOptions{}, canonicalUnsupportedField(field)
		}
	}
	if req.StreamResumable != nil && *req.StreamResumable {
		return canonicalRunOptions{}, canonicalUnsupportedField("stream_resumable")
	}
	if req.StreamSubgraphs != nil && *req.StreamSubgraphs {
		return canonicalRunOptions{}, canonicalUnsupportedField("stream_subgraphs")
	}
	if req.IfNotExists != "" && req.IfNotExists != "reject" {
		return canonicalRunOptions{}, canonicalUnsupportedField("if_not_exists")
	}
	streamModes, public := canonicalRunStreamModesFromRequest(req.StreamMode)
	if public != nil {
		return canonicalRunOptions{}, public
	}
	options := canonicalRunOptions{
		StreamModes:       append([]string(nil), streamModes...),
		MultitaskStrategy: strings.TrimSpace(req.MultitaskStrategy),
		OnDisconnect:      strings.TrimSpace(req.OnDisconnect),
		Durability:        strings.TrimSpace(req.Durability),
	}
	if options.MultitaskStrategy == "" {
		options.MultitaskStrategy = canonicalRunDefaults.MultitaskStrategy
	}
	if options.MultitaskStrategy != "reject" {
		return canonicalRunOptions{}, canonicalUnsupportedField("multitask_strategy")
	}
	if options.OnDisconnect == "" {
		options.OnDisconnect = canonicalRunDefaults.OnDisconnect
	}
	if options.OnDisconnect != "cancel" && options.OnDisconnect != "continue" {
		return canonicalRunOptions{}, canonicalUnsupportedField("on_disconnect")
	}
	if options.Durability == "" {
		options.Durability = canonicalRunDefaults.Durability
	}
	if options.Durability != "async" {
		return canonicalRunOptions{}, canonicalUnsupportedField("durability")
	}
	return options, nil
}

func canonicalRunStreamModesFromRequest(raw json.RawMessage) ([]string, *canonicalError) {
	if canonicalRawJSONNullOrOmitted(raw) {
		return append([]string(nil), canonicalRunDefaults.StreamModes...), nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return validateCanonicalRunStreamModes([]string{single})
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil {
		return nil, canonicalInvalidRequest("stream_mode must be a string or string array", "invalid_stream_mode")
	}
	return validateCanonicalRunStreamModes(multiple)
}

func validateCanonicalRunStreamModes(modes []string) ([]string, *canonicalError) {
	if len(modes) == 0 {
		return nil, canonicalInvalidRequest("stream_mode cannot be empty", "invalid_stream_mode")
	}
	result := make([]string, 0, len(modes))
	seen := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		mode = strings.TrimSpace(mode)
		if _, ok := canonicalRunStreamModeAllowlist[mode]; !ok {
			return nil, newCanonicalError(
				consts.StatusUnprocessableEntity,
				"unsupported_stream_mode",
				"Unsupported stream mode",
				"unsupported_stream_mode",
				false,
			)
		}
		if _, exists := seen[mode]; exists {
			continue
		}
		seen[mode] = struct{}{}
		result = append(result, mode)
	}
	return result, nil
}

func canonicalRunInputPayload(raw json.RawMessage) (string, []int64, *canonicalError) {
	if canonicalRawJSONNullOrOmitted(raw) {
		return "", nil, canonicalInvalidRequest("input is required", "missing_input")
	}
	var input canonicalRunInput
	if public := decodeCanonicalRawJSONObject(raw, &input, "input"); public != nil {
		return "", nil, public
	}
	if len(input.Messages) != 1 {
		return "", nil, canonicalInvalidRequest(
			"input.messages must contain exactly one user message",
			"invalid_run_messages",
		)
	}
	message := input.Messages[0]
	message.Role = strings.TrimSpace(message.Role)
	message.Content = strings.TrimSpace(message.Content)
	if message.Role != "user" || message.Content == "" || len(message.Content) > canonicalMaxRunMessageBytes {
		return "", nil, canonicalInvalidRequest(
			"input.messages must contain one non-empty user message",
			"invalid_run_message",
		)
	}
	if len(input.UploadedFiles) > 0 && !canonicalRawJSONArrayOrNull(input.UploadedFiles) {
		return "", nil, canonicalInvalidRequest("input.uploaded_files must be an array", "invalid_uploaded_files")
	}
	if len(input.UploadedFiles) == 0 || bytes.Equal(bytes.TrimSpace(input.UploadedFiles), []byte("null")) {
		return message.Content, nil, nil
	}
	var references []json.RawMessage
	if err := json.Unmarshal(input.UploadedFiles, &references); err != nil {
		return "", nil, canonicalInvalidRequest("input.uploaded_files is invalid", "invalid_uploaded_files")
	}
	if len(references) > canonicalMaxRunUploadedFileReferences {
		return "", nil, canonicalInvalidRequest("input.uploaded_files contains too many files", "invalid_uploaded_files")
	}
	fileIDs := make([]int64, 0, len(references))
	seen := make(map[int64]struct{}, len(references))
	for index, rawReference := range references {
		field := fmt.Sprintf("input.uploaded_files[%d]", index)
		var reference canonicalRunUploadedFileReference
		if public := decodeCanonicalRawJSONObject(rawReference, &reference, field); public != nil {
			return "", nil, public
		}
		fileID, public := canonicalRunUploadedFileID(reference.FileID, field+".file_id")
		if public != nil {
			return "", nil, public
		}
		if _, exists := seen[fileID]; exists {
			return "", nil, canonicalInvalidRequest("input.uploaded_files contains duplicate files", "invalid_uploaded_files")
		}
		seen[fileID] = struct{}{}
		fileIDs = append(fileIDs, fileID)
	}
	return message.Content, fileIDs, nil
}

func canonicalRunUploadedFileID(raw json.RawMessage, field string) (int64, *canonicalError) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, canonicalInvalidRequest(field+" is required", "invalid_uploaded_file")
	}
	value := string(trimmed)
	if trimmed[0] == '"' {
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return 0, canonicalInvalidRequest(field+" is invalid", "invalid_uploaded_file")
		}
	}
	fileID, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || fileID <= 0 {
		return 0, canonicalInvalidRequest(field+" is invalid", "invalid_uploaded_file")
	}
	return fileID, nil
}

func canonicalJSONObjectPayload(raw json.RawMessage, field string) (string, *canonicalError) {
	if canonicalRawJSONNullOrOmitted(raw) {
		return "", nil
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return "", canonicalInvalidRequest(field+" must be a JSON object", "invalid_"+field)
	}
	if public := validateCanonicalPersistedRunValue(value, field, 0); public != nil {
		return "", public
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", canonicalInvalidRequest(field+" is invalid", "invalid_"+field)
	}
	return string(encoded), nil
}

func validateCanonicalPersistedRunValue(value any, field string, depth int) *canonicalError {
	if depth > 16 {
		return canonicalInvalidRequest(field+" is too deeply nested", "invalid_"+field)
	}
	switch typed := value.(type) {
	case nil, bool:
		return nil
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return canonicalInvalidRequest(field+" contains an invalid number", "invalid_"+field)
		}
		return nil
	case string:
		if len(typed) > canonicalMaxRunPersistedStringBytes {
			return canonicalInvalidRequest(field+" contains an oversized string", "invalid_"+field)
		}
		if canonicalSensitiveValuePattern.MatchString(typed) {
			return canonicalInvalidRequest(
				"Sensitive values are not accepted in "+field,
				"sensitive_"+field,
			)
		}
		return nil
	case []any:
		if len(typed) > 256 {
			return canonicalInvalidRequest(field+" contains too many values", "invalid_"+field)
		}
		for _, item := range typed {
			if public := validateCanonicalPersistedRunValue(item, field, depth+1); public != nil {
				return public
			}
		}
		return nil
	case map[string]any:
		if len(typed) > 256 {
			return canonicalInvalidRequest(field+" contains too many fields", "invalid_"+field)
		}
		for key, item := range typed {
			key = strings.TrimSpace(key)
			if key == "" || len(key) > 128 {
				return canonicalInvalidRequest(field+" contains an invalid field", "invalid_"+field)
			}
			if canonicalProtectedRunPayloadField(key) {
				return canonicalUnsupportedField(field + "." + key)
			}
			if public := validateCanonicalPersistedRunValue(item, field, depth+1); public != nil {
				return public
			}
		}
		return nil
	default:
		return canonicalInvalidRequest(field+" contains an invalid value", "invalid_"+field)
	}
}

func canonicalProtectedRunPayloadField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if canonicalUnsafePublicField(normalized) {
		return true
	}
	switch normalized {
	case "user", "user_id", "owner", "owner_id", "creator", "creator_id",
		"actor", "actor_id", "space", "space_id", "workspace", "workspace_id",
		"tenant", "tenant_id", "principal", "principal_id", "submitted_by":
		return true
	}
	return normalized == "token" || strings.HasSuffix(normalized, "_token") ||
		normalized == "cookie" || strings.HasSuffix(normalized, "_cookie") ||
		normalized == "private_key" || strings.HasSuffix(normalized, "_private_key")
}

func canonicalRunMetadataPayload(metadata map[string]any) (string, *canonicalError) {
	if len(metadata) == 0 {
		return "", nil
	}
	if len(metadata) > canonicalMaxThreadMetadataKeys {
		return "", canonicalInvalidRequest("Run metadata supports at most 16 keys", "invalid_metadata_count")
	}
	projected := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) ||
			canonicalProtectedMetadataKey(key) || canonicalProtectedRunMetadataKey(key) {
			return "", canonicalUnsupportedField("metadata." + key)
		}
		public, ok := canonicalMetadataValue(value)
		if !ok {
			return "", canonicalInvalidRequest("Invalid metadata value: "+key, "invalid_metadata_value")
		}
		projected[key] = public
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return "", canonicalInvalidRequest("metadata is invalid", "invalid_metadata")
	}
	return string(encoded), nil
}

func canonicalProtectedRunMetadataKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "appended_message_id", "source_run_id", "attempt_kind",
		"human_interaction", "checkpoint_resume", "subagent_retry",
		"_idempotency", "_message":
		return true
	default:
		return false
	}
}

func canonicalApplicationCreateRunRequest(
	threadID int64,
	submission *canonicalRunSubmission,
) *appagentthread.CreateRunRequest {
	request := &appagentthread.CreateRunRequest{
		ThreadID: threadID, AssistantID: submission.AssistantID,
		Command: submission.Command, Input: submission.Input,
		Config: submission.Config, Context: submission.Context, Metadata: submission.Metadata,
		StreamMode:        canonicalStoredRunStreamModes(submission.Options.StreamModes),
		MultitaskStrategy: submission.Options.MultitaskStrategy,
		OnDisconnect:      submission.Options.OnDisconnect, Durability: submission.Options.Durability,
		IdempotencyKey:         submission.IdempotencyKey,
		IdempotencyOperation:   submission.IdempotencyOperation,
		IdempotencyFingerprint: submission.IdempotencyFingerprint,
	}
	if submission.TopLevelRetry != nil {
		request.TopLevelRetrySourceRunID = submission.TopLevelRetry.SourceRunID
		return request
	}
	request.MessageContent = submission.MessageContent
	request.MessageMetadata = submission.MessageMetadata
	request.PersistMessageReference = true
	return request
}

func canonicalStoredRunStreamModes(modes []string) string {
	encoded, err := json.Marshal(modes)
	if err != nil {
		return `[]`
	}
	return string(encoded)
}

func decodeCanonicalRawJSONObject(raw json.RawMessage, dst any, field string) *canonicalError {
	body := bytes.TrimSpace(raw)
	if len(body) == 0 || body[0] != '{' {
		return canonicalInvalidRequest(field+" must be a JSON object", "invalid_"+field)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if unknown := canonicalUnknownJSONField(err); unknown != "" {
			return canonicalUnsupportedField(field + "." + unknown)
		}
		return canonicalInvalidRequest(field+" is invalid", "invalid_"+field)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errorsIsEOF(err) {
		return canonicalInvalidRequest(field+" is invalid", "invalid_"+field)
	}
	return nil
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}

func canonicalRawJSONNullOrOmitted(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func canonicalRawJSONArrayOrNull(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("null")) || (len(trimmed) > 0 && trimmed[0] == '[')
}

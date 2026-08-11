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
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

type canonicalTokenUsageResponse struct {
	Usage         []*canonicalProductTokenUsage             `json:"usage"`
	Total         int64                                     `json:"total"`
	HasMore       bool                                      `json:"has_more"`
	NextCursor    *string                                   `json:"next_cursor,omitempty"`
	Aggregate     canonicalProductTokenUsageAggregate       `json:"aggregate"`
	RunAggregates []*canonicalProductRunTokenUsageAggregate `json:"run_aggregates"`
}

// GetCanonicalThreadTokenUsage serves GET /api/workbench/threads/:thread_id/token_usage.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and requested Run when present. It calls
// ApplicationService.GetThreadTokenUsage or GetRunTokenUsage, and returns usage JSON.
func GetCanonicalThreadTokenUsage(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("token_usage.get", "/api/workbench/threads/:thread_id/token_usage")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "token_usage"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, spaceID, ok := canonicalUsageRetryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	page, public := canonicalProductPagination(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.Limit = page.Limit
	requestLog.Offset = page.Offset
	runID, public := canonicalQueryInt64(c, "run_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	includeChildRuns, public := canonicalProductQueryBool(c, "include_child_runs")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	source, public := canonicalTokenUsageSourceFromQuery(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	usageReq := &appagentthread.GetTokenUsageRequest{
		ThreadID:         threadID,
		RunID:            canonicalOptionalQueryID(runID),
		IncludeChildRuns: includeChildRuns,
		Source:           source,
		Page:             page.Page,
		PageSize:         page.Limit,
	}

	var (
		resp *appagentthread.GetTokenUsageResponse
		err  error
	)
	if runID != nil {
		requestLog.RunID = *runID
		if _, public, err := canonicalUsageRetryLoadRun(ctx, spaceID, threadID, *runID, true); public != nil {
			writeCanonicalError(ctx, c, public.status, *public)
			return
		} else if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, *runID)
		resp, err = appagentthread.SVC.GetRunTokenUsage(ctx, usageReq)
	} else {
		resp, err = appagentthread.SVC.GetThreadTokenUsage(ctx, usageReq)
	}
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	publicResp, err := canonicalTokenUsageResponseToAPI(resp, page)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, publicResp)
}

// RetryCanonicalSubagentRun serves POST /api/workbench/threads/:thread_id/runs/:run_id/retry.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and source Run. It calls
// ApplicationService.RetrySubagentRun, and returns canonical Run JSON.
func RetryCanonicalSubagentRun(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("subagent_retry.create", "/api/workbench/threads/:thread_id/runs/:run_id/retry")
	requestLog.ResponseBodyKind = "run"
	requestLog.ResourceType = "subagent_retry"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, spaceID, ok := canonicalUsageRetryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	sourceRunID, public := canonicalPathID(c, "run_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.RunID = sourceRunID
	requestLog.SourceRunID = sourceRunID
	requestLog.ResourceID = strconv.FormatInt(sourceRunID, 10)
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
	if public := canonicalRejectNonEmptyBody(c); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	sourceRun, public, err := canonicalUsageRetryLoadRun(ctx, spaceID, threadID, sourceRunID, false)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if public := canonicalValidateSubagentRetrySource(sourceRun); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	idempotencyKey, public := canonicalRunIdempotencyKey(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.IdempotencyKeyHash = canonicalLogHash(idempotencyKey)
	scopedKey, public := canonicalPrincipalScopedIdempotencyKey(ctx, idempotencyKey)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, sourceRunID)
	resp, err := appagentthread.SVC.RetrySubagentRun(ctx, &appagentthread.RetrySubagentRunRequest{
		ThreadID:       threadID,
		SourceRunID:    sourceRunID,
		IdempotencyKey: scopedKey,
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if resp == nil || resp.Run == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty subagent retry run"))
		return
	}
	if !canonicalRetryRunMatchesSource(resp.Run, threadID, sourceRunID) {
		writeCanonicalError(ctx, c, consts.StatusConflict, *newCanonicalError(
			consts.StatusConflict,
			"idempotency_conflict",
			"Idempotency-Key is already used by another Run",
			"idempotency_key_conflict",
			false,
		))
		return
	}
	projected, err := projectCanonicalRun(resp.Run)
	if err != nil || projected == nil {
		if err == nil {
			err = fmt.Errorf("canonical subagent retry projection returned empty run")
		}
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	requestLog.RunID = resp.Run.RunID
	requestLog.LifecycleStage = "retried"
	c.Header("Content-Location", canonicalRunPath(threadID, resp.Run.RunID))
	c.JSON(consts.StatusOK, projected)
}

func canonicalUsageRetryThreadScope(
	ctx context.Context,
	c *app.RequestContext,
	requestLog *canonicalRequestLog,
) (context.Context, int64, int64, bool) {
	if !requireCanonicalAgentThreadService(ctx, c) {
		return ctx, 0, 0, false
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return ctx, 0, 0, false
	}
	requestLog.ThreadID = threadID
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return ctx, 0, 0, false
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return ctx, 0, 0, false
	}
	return ctx, threadID, spaceID, true
}

func canonicalUsageRetryLoadRun(
	ctx context.Context,
	spaceID, threadID, runID int64,
	mismatchAsInvalid bool,
) (*appagentthread.RunSummary, *canonicalError, error) {
	run, err := appagentthread.SVC.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: runID})
	if err != nil {
		return nil, nil, err
	}
	if run == nil || run.SpaceID != spaceID {
		return nil, canonicalResourceNotFound(), nil
	}
	if run.ThreadID != threadID {
		if mismatchAsInvalid {
			return nil, canonicalInvalidRequest("run_id does not belong to thread_id", "run_thread_mismatch"), nil
		}
		return nil, canonicalResourceNotFound(), nil
	}
	summary := appagentthread.DomainRunToSummary(run)
	if summary == nil {
		return nil, nil, fmt.Errorf("agent thread application returned empty run")
	}
	return summary, nil, nil
}

func canonicalValidateSubagentRetrySource(run *appagentthread.RunSummary) *canonicalError {
	if run == nil || run.RunKind != appagentthread.RunKindSubagent || run.ParentRunID <= 0 {
		return canonicalInvalidRequest("source run must be a subagent run", "invalid_subagent_retry_source")
	}
	if run.Status != appagentthread.RunStatusFailed && run.Status != appagentthread.RunStatusCanceled {
		return newCanonicalError(
			consts.StatusConflict,
			"run_conflict",
			"Source subagent run must be failed or canceled",
			"subagent_retry_source_not_terminal",
			false,
		)
	}
	return nil
}

func canonicalTokenUsageSourceFromQuery(c *app.RequestContext) (appagentthread.TokenUsageSource, *canonicalError) {
	raw, exists := c.GetQuery("source")
	if !exists || raw == "" {
		return "", nil
	}
	if strings.TrimSpace(raw) != raw {
		return "", canonicalProductInvalidQuery("source")
	}
	source := appagentthread.TokenUsageSource(raw)
	switch source {
	case appagentthread.TokenUsageSourceLeadAgent,
		appagentthread.TokenUsageSourceSubagent,
		appagentthread.TokenUsageSourceMiddleware,
		appagentthread.TokenUsageSourceTool:
		return source, nil
	default:
		return "", canonicalProductInvalidQuery("source")
	}
}

func canonicalRejectNonEmptyBody(c *app.RequestContext) *canonicalError {
	if c == nil || len(bytes.TrimSpace(c.Request.Body())) == 0 {
		return nil
	}
	return canonicalInvalidRequest("Request body is not supported for this route", "unsupported_request_body")
}

func canonicalTokenUsageResponseToAPI(
	resp *appagentthread.GetTokenUsageResponse,
	page canonicalProductPage,
) (*canonicalTokenUsageResponse, error) {
	if resp == nil {
		resp = &appagentthread.GetTokenUsageResponse{}
	}
	usage, err := canonicalProductTokenUsageRowsToAPI(resp.Usage)
	if err != nil {
		return nil, err
	}
	runAggregates, err := canonicalProductRunTokenUsageAggregatesToAPI(resp.RunAggregates)
	if err != nil {
		return nil, err
	}
	return &canonicalTokenUsageResponse{
		Usage:         usage,
		Total:         resp.Total,
		HasMore:       page.hasMore(resp.Total),
		NextCursor:    page.nextCursor(resp.Total),
		Aggregate:     projectCanonicalProductTokenUsageAggregate(resp.Aggregate),
		RunAggregates: runAggregates,
	}, nil
}

func canonicalProductTokenUsageRowsToAPI(rows []*appagentthread.TokenUsageSummary) ([]*canonicalProductTokenUsage, error) {
	result := make([]*canonicalProductTokenUsage, 0, len(rows))
	for _, row := range rows {
		usage, err := projectCanonicalProductTokenUsage(row)
		if err != nil {
			return nil, err
		}
		if usage != nil {
			result = append(result, usage)
		}
	}
	return result, nil
}

func canonicalProductRunTokenUsageAggregatesToAPI(
	rows []*appagentthread.RunTokenUsageAggregateSummary,
) ([]*canonicalProductRunTokenUsageAggregate, error) {
	result := make([]*canonicalProductRunTokenUsageAggregate, 0, len(rows))
	for _, row := range rows {
		aggregate, err := projectCanonicalProductRunTokenUsageAggregate(row)
		if err != nil {
			return nil, err
		}
		if aggregate != nil {
			result = append(result, aggregate)
		}
	}
	return result, nil
}

func canonicalRetryRunMatchesSource(run *appagentthread.RunSummary, threadID, sourceRunID int64) bool {
	if run == nil || run.ThreadID != threadID || run.RunKind != appagentthread.RunKindTask || run.ParentRunID != 0 {
		return false
	}
	metadata := canonicalJSONObject(run.Metadata)
	if canonicalString(metadata["source"]) != "subagent_retry" ||
		canonicalIDPointerToInt64(canonicalMetadataID(metadata["source_run_id"])) != sourceRunID {
		return false
	}
	if canonicalMetadataObjectSchema(
		metadata,
		"subagent_retry",
		"coze.subagent_retry.metadata.v1",
	) == nil {
		return false
	}
	command := canonicalMetadataObjectSchema(
		canonicalJSONObject(run.Command),
		"subagent_retry",
		"coze.subagent_retry.v1",
	)
	return command != nil && canonicalIDPointerToInt64(canonicalMetadataID(command["source_run_id"])) == sourceRunID
}

func canonicalIDPointerToInt64(value *string) int64 {
	if value == nil {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(*value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func canonicalResourceNotFound() *canonicalError {
	return newCanonicalError(
		consts.StatusNotFound,
		"resource_not_found",
		"Resource not found",
		"resource_not_found",
		false,
	)
}

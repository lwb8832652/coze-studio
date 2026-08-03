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
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

type canonicalArtifactListResponse struct {
	Artifacts   []*canonicalProductArtifact           `json:"artifacts"`
	Total       int64                                 `json:"total"`
	HasMore     bool                                  `json:"has_more"`
	NextCursor  *string                               `json:"next_cursor,omitempty"`
	Collections []*canonicalProductArtifactCollection `json:"collections,omitempty"`
}

type canonicalArtifactSignedURLResponse struct {
	ArtifactID       string `json:"artifact_id"`
	URL              string `json:"url"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
	ContentType      string `json:"content_type"`
	PreviewMode      string `json:"preview_mode"`
}

type canonicalArtifactRestoreResponse struct {
	Artifact *canonicalProductArtifact `json:"artifact"`
	Restored bool                      `json:"restored"`
}

type canonicalArtifactScanReviewResponse struct {
	ArtifactID string `json:"artifact_id"`
	Decision   string `json:"decision"`
	ScanStatus string `json:"scan_status"`
	Reviewed   bool   `json:"reviewed"`
}

type canonicalArtifactScanJobListResponse struct {
	Jobs       []*canonicalProductArtifactScanJob `json:"jobs"`
	Total      int64                              `json:"total"`
	HasMore    bool                               `json:"has_more"`
	NextCursor *string                            `json:"next_cursor,omitempty"`
}

type canonicalArtifactScanJobRetryResponse struct {
	Job     *canonicalProductArtifactScanJob `json:"job"`
	Retried bool                             `json:"retried"`
}

// ListCanonicalThreadArtifacts serves GET /api/workbench/threads/:thread_id/artifacts.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ListArtifacts, and returns canonical artifact-list JSON.
func ListCanonicalThreadArtifacts(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.list", "/api/workbench/threads/:thread_id/artifacts")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentArtifactService(ctx, c) {
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
	deletedOnly, public := canonicalProductQueryBool(c, "deleted_only")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	collectionID, public := canonicalArtifactCollectionQuery(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if collectionID != nil && runID == nil {
		public = canonicalProductInvalidQuery("run_id")
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.ListArtifacts(ctx, &appagentthread.ListArtifactsRequest{
		ThreadID:     threadID,
		RunID:        runID,
		CollectionID: collectionID,
		DeletedOnly:  deletedOnly,
		SpaceID:      spaceID,
		ViewerID:     workbenchViewerIDFromCtx(ctx),
		Page:         page.Page,
		PageSize:     page.Limit,
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	artifacts, err := canonicalProductArtifactsToAPI(resp.Artifacts)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical artifacts: %w", err))
		return
	}
	collections, err := canonicalProductArtifactCollectionsToAPI(resp.Collections)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical artifact collections: %w", err))
		return
	}
	c.JSON(consts.StatusOK, &canonicalArtifactListResponse{
		Artifacts:   artifacts,
		Total:       resp.Total,
		HasMore:     page.hasMore(resp.Total),
		NextCursor:  page.nextCursor(resp.Total),
		Collections: collections,
	})
}

// GetCanonicalThreadArtifactContent serves GET /api/workbench/threads/:thread_id/artifacts/:artifact_id/content.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and artifact. It calls
// ApplicationService.ReadArtifactContent, and streams the reviewed artifact bytes.
func GetCanonicalThreadArtifactContent(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.content.get", "/api/workbench/threads/:thread_id/artifacts/:artifact_id/content")
	requestLog.ResponseBodyKind = "bytes"
	requestLog.ResourceType = "artifact_content"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	threadID, artifactID, spaceID, ok := canonicalArtifactRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	if !requireCanonicalArtifactObjectStorage(ctx, c) {
		return
	}

	hasRange, rangeStart, rangeEnd, public := canonicalArtifactRange(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.ReadArtifactContent(ctx, &appagentthread.ReadArtifactContentRequest{
		ThreadID:   threadID,
		ArtifactID: artifactID,
		Mode:       canonicalArtifactContentMode(c),
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
		TraceID:    canonicalTraceID(ctx),
		HasRange:   hasRange,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
	})
	if err != nil {
		writeCanonicalArtifactApplicationError(ctx, c, err)
		return
	}
	if resp == nil || resp.Stream == nil || resp.ContentLength == 0 {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty artifact content"))
		return
	}
	requestLog.LifecycleStage = "read"
	statusCode := consts.StatusOK
	if resp.Partial {
		statusCode = consts.StatusPartialContent
		c.Response.Header.Set("Content-Range", fmt.Sprintf(
			"bytes %d-%d/%d",
			resp.RangeStart,
			resp.RangeEnd,
			resp.TotalSize,
		))
	}
	c.SetStatusCode(statusCode)
	c.SetContentType(resp.ContentType)
	c.Response.Header.Set("Content-Disposition", canonicalArtifactContentDisposition(resp.FileName, resp.Attachment))
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	if resp.TotalSize > 0 {
		c.Response.Header.Set("Accept-Ranges", "bytes")
	}
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.Response.Header.Set("Referrer-Policy", "no-referrer")
	c.Response.SetBodyStream(resp.Stream, int(resp.ContentLength))
}

func canonicalArtifactContentDisposition(fileName string, attachment bool) string {
	disposition := "inline"
	if attachment {
		disposition = "attachment"
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "artifact"
	}
	return disposition + "; filename*=UTF-8''" + url.PathEscape(fileName)
}

// GetCanonicalThreadArtifactSignedURL serves GET /api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and artifact. It calls
// ApplicationService.CreateArtifactSignedURL, and returns signed-URL receipt JSON.
func GetCanonicalThreadArtifactSignedURL(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.signed_url.create", "/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact_signed_url"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	threadID, artifactID, spaceID, ok := canonicalArtifactRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	ttlSeconds, public := canonicalProductQueryInt64Value(c, "ttl_seconds")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	if !requireCanonicalArtifactObjectSigner(ctx, c) {
		return
	}

	resp, err := appagentthread.SVC.CreateArtifactSignedURL(ctx, &appagentthread.CreateArtifactSignedURLRequest{
		ThreadID:   threadID,
		ArtifactID: artifactID,
		Mode:       canonicalArtifactContentMode(c),
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
		TraceID:    canonicalTraceID(ctx),
		TTLSeconds: ttlSeconds,
	})
	if err != nil {
		writeCanonicalArtifactApplicationError(ctx, c, err)
		return
	}
	if resp == nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty artifact signed url"))
		return
	}
	publicArtifactID := strconv.FormatInt(artifactID, 10)
	if resp.Artifact != nil && resp.Artifact.ArtifactID > 0 {
		publicArtifactID = strconv.FormatInt(resp.Artifact.ArtifactID, 10)
	}
	requestLog.LifecycleStage = "signed"
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.Response.Header.Set("Referrer-Policy", "no-referrer")
	c.JSON(consts.StatusOK, &canonicalArtifactSignedURLResponse{
		ArtifactID:       publicArtifactID,
		URL:              resp.URL,
		ExpiresInSeconds: resp.ExpiresInSeconds,
		ContentType:      canonicalCleanString(resp.ContentType, 128),
		PreviewMode:      canonicalProductIdentifier(string(resp.PreviewMode)),
	})
}

// CopyCanonicalThreadArtifactLink issues a short-lived, audited download grant.
func CopyCanonicalThreadArtifactLink(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.copy_link.create", "/api/workbench/threads/:thread_id/artifacts/:artifact_id/copy_link")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact_copy_link"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	threadID, artifactID, spaceID, ok := canonicalArtifactRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	if !requireCanonicalArtifactObjectSigner(ctx, c) {
		return
	}
	resp, err := appagentthread.SVC.CopyArtifactLink(ctx, &appagentthread.CopyArtifactLinkRequest{
		ThreadID:   threadID,
		ArtifactID: artifactID,
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
		TraceID:    canonicalTraceID(ctx),
	})
	if err != nil {
		writeCanonicalArtifactApplicationError(ctx, c, err)
		return
	}
	if resp == nil || strings.TrimSpace(resp.CopyURL) == "" {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("agent thread application returned empty artifact copy link"))
		return
	}
	expiresAt, err := canonicalProductRequiredTime(resp.ExpiresAt, "artifact copy link expires_at")
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	requestLog.LifecycleStage = "copied"
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.Response.Header.Set("Referrer-Policy", "no-referrer")
	c.JSON(consts.StatusOK, map[string]string{
		"artifact_id": strconv.FormatInt(resp.ArtifactID, 10),
		"copy_url":    resp.CopyURL,
		"expires_at":  expiresAt,
	})
}

// DeleteCanonicalThreadArtifact serves DELETE /api/workbench/threads/:thread_id/artifacts/:artifact_id.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and artifact. It calls
// ApplicationService.DeleteArtifact, and returns 204 with an empty body.
func DeleteCanonicalThreadArtifact(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.delete", "/api/workbench/threads/:thread_id/artifacts/:artifact_id")
	requestLog.ResponseBodyKind = "empty"
	requestLog.ResourceType = "artifact"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	threadID, artifactID, spaceID, ok := canonicalArtifactRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}

	resp, err := appagentthread.SVC.DeleteArtifact(ctx, &appagentthread.DeleteArtifactRequest{
		ThreadID:   threadID,
		ArtifactID: artifactID,
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Deleted {
		writeCanonicalArtifactNotFound(ctx, c)
		return
	}
	requestLog.LifecycleStage = "deleted"
	c.Status(consts.StatusNoContent)
}

// RestoreCanonicalThreadArtifact serves POST /api/workbench/threads/:thread_id/artifacts/:artifact_id/restore.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and artifact. It calls
// ApplicationService.RestoreArtifact, and returns canonical restore-result JSON.
func RestoreCanonicalThreadArtifact(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.restore", "/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	threadID, artifactID, spaceID, ok := canonicalArtifactRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}

	resp, err := appagentthread.SVC.RestoreArtifact(ctx, &appagentthread.RestoreArtifactRequest{
		ThreadID:   threadID,
		ArtifactID: artifactID,
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Restored || resp.Artifact == nil {
		writeCanonicalArtifactNotFound(ctx, c)
		return
	}
	artifact, err := projectCanonicalProductArtifact(resp.Artifact)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical restored artifact: %w", err))
		return
	}
	requestLog.LifecycleStage = "restored"
	c.JSON(consts.StatusOK, &canonicalArtifactRestoreResponse{Artifact: artifact, Restored: resp.Restored})
}

// ReviewCanonicalThreadArtifactScan serves POST /api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and artifact. It calls
// ApplicationService.ReviewArtifactScan, and returns canonical scan-review JSON.
func ReviewCanonicalThreadArtifactScan(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact.scan_review.create", "/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact_scan_review"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	threadID, artifactID, spaceID, ok := canonicalArtifactRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason,omitempty"`
	}
	if public := decodeCanonicalProductJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	resp, err := appagentthread.SVC.ReviewArtifactScan(ctx, &appagentthread.ReviewArtifactScanRequest{
		ThreadID:   threadID,
		ArtifactID: artifactID,
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
		Decision:   req.Decision,
		Reason:     req.Reason,
	})
	if err != nil {
		writeCanonicalArtifactApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Reviewed {
		writeCanonicalArtifactNotFound(ctx, c)
		return
	}
	requestLog.LifecycleStage = "reviewed"
	c.JSON(consts.StatusOK, &canonicalArtifactScanReviewResponse{
		ArtifactID: strconv.FormatInt(resp.ArtifactID, 10),
		Decision:   canonicalProductIdentifier(resp.Decision),
		ScanStatus: canonicalProductScanStatus(resp.ScanStatus),
		Reviewed:   resp.Reviewed,
	})
}

// ListCanonicalThreadArtifactScanJobs serves GET /api/workbench/threads/:thread_id/artifact_scan_jobs.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ListArtifactScanJobs, and returns canonical scan-job-list JSON.
func ListCanonicalThreadArtifactScanJobs(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact_scan_job.list", "/api/workbench/threads/:thread_id/artifact_scan_jobs")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact_scan_job"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentArtifactService(ctx, c) {
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
	artifactID, public := canonicalQueryInt64(c, "artifact_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	status, public := canonicalProductQueryScanJobStatus(c, "status")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	scanner, public := canonicalProductQueryToken(c, "scanner")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.ListArtifactScanJobs(ctx, &appagentthread.ListArtifactScanJobsRequest{
		ThreadID:   threadID,
		RunID:      runID,
		ArtifactID: artifactID,
		Status:     status,
		Scanner:    scanner,
		SpaceID:    spaceID,
		ViewerID:   workbenchViewerIDFromCtx(ctx),
		Page:       page.Page,
		PageSize:   page.Limit,
	})
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	jobs, err := canonicalProductArtifactScanJobsToAPI(resp.Jobs)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical artifact scan jobs: %w", err))
		return
	}
	c.JSON(consts.StatusOK, &canonicalArtifactScanJobListResponse{
		Jobs:       jobs,
		Total:      resp.Total,
		HasMore:    page.hasMore(resp.Total),
		NextCursor: page.nextCursor(resp.Total),
	})
}

// RetryCanonicalThreadArtifactScanJob serves POST /api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and scan job. It calls
// ApplicationService.RetryArtifactScanJob, and returns canonical retry-result JSON.
func RetryCanonicalThreadArtifactScanJob(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("artifact_scan_job.retry", "/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "artifact_scan_job"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalAgentArtifactService(ctx, c) {
		return
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID = threadID
	jobID, public := canonicalProductPathID(c, "job_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ResourceID = strconv.FormatInt(jobID, 10)
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}

	resp, err := appagentthread.SVC.RetryArtifactScanJob(ctx, &appagentthread.RetryArtifactScanJobRequest{
		ThreadID: threadID,
		JobID:    jobID,
		SpaceID:  spaceID,
		ViewerID: workbenchViewerIDFromCtx(ctx),
	})
	if err != nil {
		writeCanonicalArtifactApplicationError(ctx, c, err)
		return
	}
	if resp == nil || resp.Job == nil {
		writeCanonicalArtifactNotFound(ctx, c)
		return
	}
	job, err := projectCanonicalProductArtifactScanJob(resp.Job)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical retried artifact scan job: %w", err))
		return
	}
	requestLog.LifecycleStage = "retried"
	c.JSON(consts.StatusOK, &canonicalArtifactScanJobRetryResponse{Job: job, Retried: resp.Retried})
}

func canonicalArtifactRouteScope(
	ctx context.Context,
	c *app.RequestContext,
	requestLog *canonicalRequestLog,
) (int64, int64, int64, bool) {
	if !requireCanonicalAgentArtifactService(ctx, c) {
		return 0, 0, 0, false
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return 0, 0, 0, false
	}
	requestLog.ThreadID = threadID
	artifactID, public := canonicalProductPathID(c, "artifact_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return 0, 0, 0, false
	}
	requestLog.ResourceID = strconv.FormatInt(artifactID, 10)
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return 0, 0, 0, false
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return 0, 0, 0, false
	}
	return threadID, artifactID, spaceID, true
}

func requireCanonicalAgentArtifactService(ctx context.Context, c *app.RequestContext) bool {
	if !requireCanonicalAgentThreadService(ctx, c) {
		return false
	}
	if appagentthread.SVC.ArtifactSVC != nil {
		return true
	}
	writeCanonicalArtifactDependencyUnavailable(ctx, c, "agent_artifact_service_unavailable")
	return false
}

func requireCanonicalArtifactObjectStorage(ctx context.Context, c *app.RequestContext) bool {
	if appagentthread.SVC != nil && appagentthread.SVC.ArtifactObjectStorage != nil {
		return true
	}
	writeCanonicalArtifactDependencyUnavailable(ctx, c, "artifact_object_storage_unavailable")
	return false
}

func requireCanonicalArtifactObjectSigner(ctx context.Context, c *app.RequestContext) bool {
	if appagentthread.SVC != nil && appagentthread.SVC.ArtifactObjectStorage != nil {
		if _, ok := appagentthread.SVC.ArtifactObjectStorage.(appagentthread.ArtifactObjectURLSigner); ok {
			return true
		}
	}
	writeCanonicalArtifactDependencyUnavailable(ctx, c, "artifact_object_signer_unavailable")
	return false
}

func writeCanonicalArtifactDependencyUnavailable(
	ctx context.Context,
	c *app.RequestContext,
	errorClass string,
) {
	writeCanonicalError(ctx, c, consts.StatusServiceUnavailable, *newCanonicalError(
		consts.StatusServiceUnavailable,
		"dependency_unavailable",
		"Required service is unavailable",
		errorClass,
		true,
	))
}

func canonicalArtifactContentMode(c *app.RequestContext) appagentthread.ArtifactContentMode {
	mode := appagentthread.ArtifactContentMode(strings.ToLower(strings.TrimSpace(c.Query("mode"))))
	if mode == appagentthread.ArtifactContentModeDownload {
		return appagentthread.ArtifactContentModeDownload
	}
	return appagentthread.ArtifactContentModePreview
}

func canonicalArtifactCollectionQuery(c *app.RequestContext) (*string, *canonicalError) {
	raw, exists := c.GetQuery("collection_id")
	if !exists {
		return nil, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || canonicalProductIdentifier(raw) != raw {
		return nil, canonicalProductInvalidQuery("collection_id")
	}
	return &raw, nil
}

func canonicalArtifactRange(
	c *app.RequestContext,
) (bool, int64, *int64, *canonicalError) {
	raw := strings.TrimSpace(string(c.Request.Header.Peek("Range")))
	if raw == "" {
		return false, 0, nil, nil
	}
	invalid := func() (bool, int64, *int64, *canonicalError) {
		return false, 0, nil, newCanonicalError(
			416,
			"invalid_range",
			"Requested artifact range is invalid",
			"invalid_artifact_range",
			false,
		)
	}
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return invalid()
	}
	startRaw, endRaw, ok := strings.Cut(strings.TrimPrefix(raw, "bytes="), "-")
	if !ok || startRaw == "" {
		return invalid()
	}
	start, err := strconv.ParseInt(startRaw, 10, 64)
	if err != nil || start < 0 {
		return invalid()
	}
	if endRaw == "" {
		return true, start, nil, nil
	}
	end, err := strconv.ParseInt(endRaw, 10, 64)
	if err != nil || end < start {
		return invalid()
	}
	return true, start, &end, nil
}

func canonicalProductArtifactsToAPI(
	summaries []*appagentthread.ArtifactSummary,
) ([]*canonicalProductArtifact, error) {
	artifacts := make([]*canonicalProductArtifact, 0, len(summaries))
	for _, summary := range summaries {
		artifact, err := projectCanonicalProductArtifact(summary)
		if err != nil {
			return nil, err
		}
		if artifact != nil {
			artifacts = append(artifacts, artifact)
		}
	}
	return artifacts, nil
}

func canonicalProductArtifactCollectionsToAPI(
	summaries []*appagentthread.ArtifactCollectionSummary,
) ([]*canonicalProductArtifactCollection, error) {
	collections := make([]*canonicalProductArtifactCollection, 0, len(summaries))
	for _, summary := range summaries {
		if summary == nil {
			continue
		}
		collectionID := canonicalProductIdentifier(summary.CollectionID)
		if collectionID == "" {
			return nil, fmt.Errorf("canonical artifact collection requires an id")
		}
		artifactIDs := make([]string, 0, len(summary.ArtifactIDs))
		for _, artifactID := range summary.ArtifactIDs {
			projected, err := canonicalProductRequiredID(artifactID, "artifact collection member")
			if err != nil {
				return nil, err
			}
			artifactIDs = append(artifactIDs, projected)
		}
		collections = append(collections, &canonicalProductArtifactCollection{
			CollectionID: collectionID,
			ArtifactIDs:  artifactIDs,
			CurrentIndex: summary.CurrentIndex,
			TotalCount:   summary.TotalCount,
		})
	}
	return collections, nil
}

func canonicalProductArtifactScanJobsToAPI(
	summaries []*appagentthread.ArtifactScanJobSummary,
) ([]*canonicalProductArtifactScanJob, error) {
	jobs := make([]*canonicalProductArtifactScanJob, 0, len(summaries))
	for _, summary := range summaries {
		job, err := projectCanonicalProductArtifactScanJob(summary)
		if err != nil {
			return nil, err
		}
		if job != nil {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

func canonicalProductQueryBool(c *app.RequestContext, name string) (bool, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists {
		return false, nil
	}
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, canonicalProductInvalidQuery(name)
	}
}

func canonicalProductQueryInt64Value(c *app.RequestContext, name string) (int64, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists || raw == "" {
		return 0, nil
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, canonicalProductInvalidQuery(name)
		}
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, canonicalProductInvalidQuery(name)
	}
	return value, nil
}

func canonicalProductQueryToken(c *app.RequestContext, name string) (string, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists {
		return "", nil
	}
	if raw == "" || strings.TrimSpace(raw) != raw {
		return "", canonicalProductInvalidQuery(name)
	}
	cleaned := canonicalProductIdentifier(raw)
	if cleaned == "" || cleaned != raw {
		return "", canonicalProductInvalidQuery(name)
	}
	return cleaned, nil
}

func canonicalProductQueryScanJobStatus(c *app.RequestContext, name string) (string, *canonicalError) {
	value, public := canonicalProductQueryToken(c, name)
	if public != nil || value == "" {
		return value, public
	}
	switch value {
	case "pending", "processing", "succeeded", "failed":
		return value, nil
	default:
		return "", canonicalProductInvalidQuery(name)
	}
}

func canonicalProductInvalidQuery(name string) *canonicalError {
	return newCanonicalError(
		consts.StatusUnprocessableEntity,
		"invalid_query_parameter",
		"Invalid query parameter: "+name,
		"invalid_query_parameter",
		false,
	)
}

func writeCanonicalArtifactApplicationError(ctx context.Context, c *app.RequestContext, err error) {
	var scanBlocked *appagentthread.ArtifactContentBlockedByScanError
	switch {
	case canonicalArtifactDependencyUnavailable(err):
		writeCanonicalArtifactDependencyUnavailable(ctx, c, "artifact_dependency_unavailable")
	case errors.As(err, &scanBlocked):
		writeCanonicalError(ctx, c, consts.StatusConflict, *newCanonicalError(
			consts.StatusConflict,
			"artifact_content_blocked",
			"Artifact content is blocked by scan policy",
			"artifact_content_blocked",
			false,
		))
	case errors.Is(err, appagentthread.ErrArtifactScanJobRetryNotAllowed):
		writeCanonicalError(ctx, c, consts.StatusConflict, *newCanonicalError(
			consts.StatusConflict,
			"artifact_scan_job_not_retryable",
			"Artifact scan job cannot be retried",
			"artifact_scan_job_not_retryable",
			false,
		))
	case errors.Is(err, appagentthread.ErrArtifactSignedURLNotSupported):
		writeCanonicalError(ctx, c, consts.StatusConflict, *newCanonicalError(
			consts.StatusConflict,
			"artifact_signed_url_not_supported",
			"Artifact signed URL is not supported for this mode",
			"artifact_signed_url_not_supported",
			false,
		))
	case errors.Is(err, appagentthread.ErrArtifactContentRangeInvalid):
		writeCanonicalError(ctx, c, 416, *newCanonicalError(
			416,
			"invalid_range",
			"Requested artifact range is invalid",
			"invalid_artifact_range",
			false,
		))
	case errors.Is(err, appagentthread.ErrArtifactTrustedMetadataUnavailable):
		writeCanonicalError(ctx, c, consts.StatusConflict, *newCanonicalError(
			consts.StatusConflict,
			"artifact_content_unavailable",
			"Artifact content is not ready for delivery",
			"artifact_trusted_metadata_unavailable",
			true,
		))
	case errors.Is(err, appagentthread.ErrArtifactScanReviewDecisionInvalid):
		writeCanonicalError(ctx, c, consts.StatusBadRequest, *newCanonicalError(
			consts.StatusBadRequest,
			"invalid_request",
			"Artifact scan review decision is invalid",
			"invalid_artifact_scan_review_decision",
			false,
		))
	default:
		writeCanonicalApplicationError(ctx, c, err)
	}
}

func canonicalArtifactDependencyUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, marker := range []string{
		"agent artifact service is not initialized",
		"artifact object storage is not configured",
		"artifact object storage streaming is not configured",
		"artifact object storage signing is not configured",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func writeCanonicalArtifactNotFound(ctx context.Context, c *app.RequestContext) {
	public := newCanonicalError(
		consts.StatusNotFound,
		"resource_not_found",
		"Resource not found",
		"resource_not_found",
		false,
	)
	writeCanonicalError(ctx, c, public.status, *public)
}

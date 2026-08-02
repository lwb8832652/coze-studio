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
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

type canonicalMemoryListResponse struct {
	Memories   []*canonicalProductMemory `json:"memories"`
	Total      int64                     `json:"total"`
	HasMore    bool                      `json:"has_more"`
	NextCursor *string                   `json:"next_cursor,omitempty"`
}

type canonicalMemoryUpdateResponse struct {
	Memory  *canonicalProductMemory `json:"memory"`
	Updated bool                    `json:"updated"`
}

type canonicalMemoryRestoreResponse struct {
	Memory   *canonicalProductMemory `json:"memory"`
	Restored bool                    `json:"restored"`
}

type canonicalMemoryClearResponse struct {
	Deleted int64 `json:"deleted"`
}

type canonicalMemoryImportResponse struct {
	Imported int64                     `json:"imported"`
	Skipped  int64                     `json:"skipped"`
	Memories []*canonicalProductMemory `json:"memories"`
}

type canonicalMemoryExportResponse struct {
	Schema     string                    `json:"schema"`
	ThreadID   string                    `json:"thread_id"`
	ExportedAt string                    `json:"exported_at"`
	Total      int64                     `json:"total"`
	Memories   []*canonicalProductMemory `json:"memories"`
}

type canonicalMemoryAuditEventListResponse struct {
	Events     []*canonicalProductMemoryAudit `json:"events"`
	Total      int64                          `json:"total"`
	HasMore    bool                           `json:"has_more"`
	NextCursor *string                        `json:"next_cursor,omitempty"`
}

type canonicalGuardrailAuditEventListResponse struct {
	Events     []*canonicalProductGuardrailAudit `json:"events"`
	Total      int64                             `json:"total"`
	HasMore    bool                              `json:"has_more"`
	NextCursor *string                           `json:"next_cursor,omitempty"`
}

type canonicalGuardrailAuditExportResponse struct {
	Schema     string                            `json:"schema"`
	ThreadID   string                            `json:"thread_id"`
	ExportedAt string                            `json:"exported_at"`
	Total      int64                             `json:"total"`
	Events     []*canonicalProductGuardrailAudit `json:"events"`
}

type canonicalMCPRuntimeAuditEventListResponse struct {
	Events     []*canonicalProductMCPRuntimeAudit `json:"events"`
	Total      int64                              `json:"total"`
	HasMore    bool                               `json:"has_more"`
	NextCursor *string                            `json:"next_cursor,omitempty"`
}

// ListCanonicalThreadMemories serves GET /api/workbench/threads/:thread_id/memories.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ListMemories, and returns canonical memory-list JSON.
func ListCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.list", "/api/workbench/threads/:thread_id/memories")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
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
	scopes, public := canonicalMemoryScopesFromQuery(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	includeExpired, public := canonicalProductQueryBool(c, "include_expired")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	includeDeleted, public := canonicalProductQueryBool(c, "include_deleted")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}

	resp, err := appagentthread.SVC.ListMemories(ctx, &appagentthread.ListMemoriesRequest{
		ThreadID:       threadID,
		ViewerID:       workbenchViewerIDFromCtx(ctx),
		RunID:          canonicalOptionalQueryID(runID),
		Scopes:         scopes,
		Query:          canonicalMemoryQuery(c),
		IncludeExpired: includeExpired,
		IncludeDeleted: includeDeleted,
		Page:           page.Page,
		PageSize:       page.Limit,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	memories, err := canonicalProductMemoriesToAPI(resp.Memories)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical memories: %w", err))
		return
	}
	c.JSON(consts.StatusOK, &canonicalMemoryListResponse{
		Memories:   memories,
		Total:      resp.Total,
		HasMore:    page.hasMore(resp.Total),
		NextCursor: page.nextCursor(resp.Total),
	})
}

// UpdateCanonicalThreadMemory serves PUT /api/workbench/threads/:thread_id/memories/:memory_id.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and memory. It calls
// ApplicationService.UpdateMemory, and returns canonical update-result JSON.
func UpdateCanonicalThreadMemory(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.update", "/api/workbench/threads/:thread_id/memories/:memory_id")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, memoryID, ok := canonicalMemoryRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	var req canonicalMemoryMutationBody
	if public := decodeCanonicalProductJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	input, public := req.toUpdateRequest(threadID, memoryID, workbenchViewerIDFromCtx(ctx))
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.UpdateMemory(ctx, input)
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Updated || resp.Memory == nil {
		writeCanonicalMemoryNotFound(ctx, c)
		return
	}
	memory, err := projectCanonicalProductMemory(resp.Memory)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical updated memory: %w", err))
		return
	}
	requestLog.LifecycleStage = "updated"
	c.JSON(consts.StatusOK, &canonicalMemoryUpdateResponse{Memory: memory, Updated: resp.Updated})
}

// DeleteCanonicalThreadMemory serves DELETE /api/workbench/threads/:thread_id/memories/:memory_id.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and memory. It calls
// ApplicationService.DeleteMemory, and returns 204 with an empty body.
func DeleteCanonicalThreadMemory(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.delete", "/api/workbench/threads/:thread_id/memories/:memory_id")
	requestLog.ResponseBodyKind = "empty"
	requestLog.ResourceType = "memory"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, memoryID, ok := canonicalMemoryRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	viewerID := workbenchViewerIDFromCtx(ctx)
	resp, err := appagentthread.SVC.DeleteMemory(ctx, &appagentthread.DeleteMemoryRequest{
		ThreadID: threadID,
		MemoryID: memoryID,
		ActorID:  viewerID,
		ViewerID: viewerID,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Deleted {
		writeCanonicalMemoryNotFound(ctx, c)
		return
	}
	requestLog.LifecycleStage = "deleted"
	c.Status(consts.StatusNoContent)
}

// RestoreCanonicalThreadMemory serves POST /api/workbench/threads/:thread_id/memories/:memory_id/restore.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread and memory. It calls
// ApplicationService.RestoreMemory, and returns canonical restore-result JSON.
func RestoreCanonicalThreadMemory(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.restore", "/api/workbench/threads/:thread_id/memories/:memory_id/restore")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, memoryID, ok := canonicalMemoryRouteScope(ctx, c, requestLog)
	if !ok {
		return
	}
	viewerID := workbenchViewerIDFromCtx(ctx)
	resp, err := appagentthread.SVC.RestoreMemory(ctx, &appagentthread.RestoreMemoryRequest{
		ThreadID: threadID,
		MemoryID: memoryID,
		ActorID:  viewerID,
		ViewerID: viewerID,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	if resp == nil || !resp.Restored || resp.Memory == nil {
		writeCanonicalMemoryNotFound(ctx, c)
		return
	}
	memory, err := projectCanonicalProductMemory(resp.Memory)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical restored memory: %w", err))
		return
	}
	requestLog.LifecycleStage = "restored"
	c.JSON(consts.StatusOK, &canonicalMemoryRestoreResponse{Memory: memory, Restored: resp.Restored})
}

// ClearCanonicalThreadMemories serves POST /api/workbench/threads/:thread_id/memories/clear.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ClearMemories, and returns canonical deleted-count JSON.
func ClearCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.clear", "/api/workbench/threads/:thread_id/memories/clear")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	var req struct {
		RunID  json.RawMessage `json:"run_id,omitempty"`
		Scopes []string        `json:"scopes,omitempty"`
	}
	if public := decodeCanonicalProductJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	runID, public := canonicalMemoryOptionalBodyID(req.RunID, "run_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	scopes, public := canonicalMemoryScopes(req.Scopes...)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	viewerID := workbenchViewerIDFromCtx(ctx)
	resp, err := appagentthread.SVC.ClearMemories(ctx, &appagentthread.ClearMemoriesRequest{
		ThreadID: threadID,
		RunID:    runID,
		Scopes:   scopes,
		ActorID:  viewerID,
		ViewerID: viewerID,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	requestLog.LifecycleStage = "deleted"
	c.JSON(consts.StatusOK, &canonicalMemoryClearResponse{Deleted: resp.Deleted})
}

// ExportCanonicalThreadMemories serves GET /api/workbench/threads/:thread_id/memories/export.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ExportMemories, and returns canonical memory-export JSON.
func ExportCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.export", "/api/workbench/threads/:thread_id/memories/export")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory_export"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	limit, public := canonicalMemoryExportLimit(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	requestLog.Limit = limit
	runID, public := canonicalQueryInt64(c, "run_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	scopes, public := canonicalMemoryScopesFromQuery(c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	includeExpired, public := canonicalProductQueryBool(c, "include_expired")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	includeDeleted, public := canonicalProductQueryBool(c, "include_deleted")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.ExportMemories(ctx, &appagentthread.ExportMemoriesRequest{
		ThreadID:       threadID,
		ViewerID:       workbenchViewerIDFromCtx(ctx),
		RunID:          canonicalOptionalQueryID(runID),
		Scopes:         scopes,
		Query:          canonicalMemoryQuery(c),
		IncludeExpired: includeExpired,
		IncludeDeleted: includeDeleted,
		Limit:          limit,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	memories, err := canonicalProductMemoriesToAPI(resp.Memories)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical memory export: %w", err))
		return
	}
	threadIDText, err := canonicalProductRequiredID(resp.ThreadID, "memory export thread")
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	exportedAt, err := canonicalProductRequiredTime(resp.ExportedAt, "memory export exported_at")
	if err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return
	}
	requestLog.LifecycleStage = "export"
	c.JSON(consts.StatusOK, &canonicalMemoryExportResponse{
		Schema: resp.Schema, ThreadID: threadIDText, ExportedAt: exportedAt,
		Total: resp.Total, Memories: memories,
	})
}

// ImportCanonicalThreadMemories serves POST /api/workbench/threads/:thread_id/memories/import.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ImportMemories, and returns canonical memory-import JSON.
func ImportCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory.import", "/api/workbench/threads/:thread_id/memories/import")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	var req struct {
		Memories []canonicalMemoryImportItemBody `json:"memories"`
	}
	if public := decodeCanonicalProductJSON(c, &req); public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	items, public := canonicalMemoryImportItems(req.Memories)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	viewerID := workbenchViewerIDFromCtx(ctx)
	resp, err := appagentthread.SVC.ImportMemories(ctx, &appagentthread.ImportMemoriesRequest{
		ThreadID: threadID,
		ActorID:  viewerID,
		ViewerID: viewerID,
		Memories: items,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	memories, err := canonicalProductMemoriesToAPI(resp.Memories)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical imported memories: %w", err))
		return
	}
	requestLog.LifecycleStage = "import"
	c.JSON(consts.StatusOK, &canonicalMemoryImportResponse{
		Imported: resp.Imported, Skipped: resp.Skipped, Memories: memories,
	})
}

// ListCanonicalThreadMemoryAuditEvents serves GET /api/workbench/threads/:thread_id/memories/audit_events.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ListMemoryAuditEvents, and returns canonical audit-event-list JSON.
func ListCanonicalThreadMemoryAuditEvents(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("memory_audit.list", "/api/workbench/threads/:thread_id/memories/audit_events")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "memory_audit"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
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
	memoryID, public := canonicalQueryInt64(c, "memory_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return
	}
	resp, err := appagentthread.SVC.ListMemoryAuditEvents(ctx, &appagentthread.ListMemoryAuditEventsRequest{
		ThreadID: threadID,
		MemoryID: canonicalOptionalQueryID(memoryID),
		ViewerID: workbenchViewerIDFromCtx(ctx),
		Page:     page.Page,
		PageSize: page.Limit,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	events, err := canonicalProductMemoryAuditsToAPI(resp.Events)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical memory audits: %w", err))
		return
	}
	c.JSON(consts.StatusOK, &canonicalMemoryAuditEventListResponse{
		Events: events, Total: resp.Total, HasMore: page.hasMore(resp.Total), NextCursor: page.nextCursor(resp.Total),
	})
}

// ListCanonicalThreadGuardrailAuditEvents serves GET /api/workbench/threads/:thread_id/guardrail_audit_events.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ListGuardrailAuditEvents, and returns canonical audit-event-list JSON.
func ListCanonicalThreadGuardrailAuditEvents(ctx context.Context, c *app.RequestContext) {
	canonicalListGuardrailAuditEvents(ctx, c, false)
}

// ExportCanonicalThreadGuardrailAuditEvents serves GET /api/workbench/threads/:thread_id/guardrail_audit_events/export.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ExportGuardrailAuditEvents, and returns canonical audit-export JSON.
func ExportCanonicalThreadGuardrailAuditEvents(ctx context.Context, c *app.RequestContext) {
	canonicalListGuardrailAuditEvents(ctx, c, true)
}

// ListCanonicalThreadMCPRuntimeAuditEvents serves GET /api/workbench/threads/:thread_id/mcp_runtime_audit_events.
// It authorizes the authenticated session principal through server-authorized X-Coze-Space-ID
// and against the path Thread. It calls
// ApplicationService.ListMCPRuntimeAuditEvents, and returns canonical audit-event-list JSON.
func ListCanonicalThreadMCPRuntimeAuditEvents(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog("mcp_runtime_audit.list", "/api/workbench/threads/:thread_id/mcp_runtime_audit_events")
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = "mcp_runtime_audit"
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	if !requireCanonicalMCPRuntimeAuditRepository(ctx, c) {
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
	resp, err := appagentthread.SVC.ListMCPRuntimeAuditEvents(ctx, &appagentthread.ListMCPRuntimeAuditEventsRequest{
		ThreadID: threadID,
		RunID:    canonicalOptionalQueryID(runID),
		ViewerID: workbenchViewerIDFromCtx(ctx),
		Page:     page.Page,
		PageSize: page.Limit,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	events, err := canonicalProductMCPRuntimeAuditsToAPI(resp.Events)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical mcp runtime audits: %w", err))
		return
	}
	c.JSON(consts.StatusOK, &canonicalMCPRuntimeAuditEventListResponse{
		Events: events, Total: resp.Total, HasMore: page.hasMore(resp.Total), NextCursor: page.nextCursor(resp.Total),
	})
}

func canonicalListGuardrailAuditEvents(ctx context.Context, c *app.RequestContext, export bool) {
	operation := "guardrail_audit.list"
	route := "/api/workbench/threads/:thread_id/guardrail_audit_events"
	resourceType := "guardrail_audit"
	if export {
		operation = "guardrail_audit.export"
		route = "/api/workbench/threads/:thread_id/guardrail_audit_events/export"
		resourceType = "guardrail_export"
	}
	requestLog := beginCanonicalRequestLog(operation, route)
	requestLog.ResponseBodyKind = "values"
	requestLog.ResourceType = resourceType
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
	if !ok {
		return
	}
	if !requireCanonicalGuardrailAuditRepository(ctx, c) {
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
	if export {
		resp, err := appagentthread.SVC.ExportGuardrailAuditEvents(ctx, &appagentthread.ExportGuardrailAuditEventsRequest{
			ThreadID: threadID,
			RunID:    canonicalOptionalQueryID(runID),
			ViewerID: workbenchViewerIDFromCtx(ctx),
			Page:     page.Page,
			PageSize: page.Limit,
		})
		if err != nil {
			writeCanonicalMemoryAuditApplicationError(ctx, c, err)
			return
		}
		events, err := canonicalProductGuardrailAuditsToAPI(resp.Events)
		if err != nil {
			writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical guardrail export: %w", err))
			return
		}
		threadIDText, err := canonicalProductRequiredID(resp.ThreadID, "guardrail export thread")
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		exportedAt, err := canonicalProductRequiredTime(resp.ExportedAt, "guardrail export exported_at")
		if err != nil {
			writeCanonicalApplicationError(ctx, c, err)
			return
		}
		requestLog.LifecycleStage = "export"
		c.JSON(consts.StatusOK, &canonicalGuardrailAuditExportResponse{
			Schema: resp.Schema, ThreadID: threadIDText, ExportedAt: exportedAt, Total: resp.Total, Events: events,
		})
		return
	}
	resp, err := appagentthread.SVC.ListGuardrailAuditEvents(ctx, &appagentthread.ListGuardrailAuditEventsRequest{
		ThreadID: threadID,
		RunID:    canonicalOptionalQueryID(runID),
		ViewerID: workbenchViewerIDFromCtx(ctx),
		Page:     page.Page,
		PageSize: page.Limit,
	})
	if err != nil {
		writeCanonicalMemoryAuditApplicationError(ctx, c, err)
		return
	}
	events, err := canonicalProductGuardrailAuditsToAPI(resp.Events)
	if err != nil {
		writeCanonicalApplicationError(ctx, c, fmt.Errorf("project canonical guardrail audits: %w", err))
		return
	}
	c.JSON(consts.StatusOK, &canonicalGuardrailAuditEventListResponse{
		Events: events, Total: resp.Total, HasMore: page.hasMore(resp.Total), NextCursor: page.nextCursor(resp.Total),
	})
}

type canonicalMemoryMutationBody struct {
	RunID                json.RawMessage `json:"run_id,omitempty"`
	Scope                string          `json:"scope"`
	Content              string          `json:"content"`
	Metadata             json.RawMessage `json:"metadata,omitempty"`
	Score                float64         `json:"score,omitempty"`
	Confidence           float64         `json:"confidence,omitempty"`
	SourceType           string          `json:"source_type,omitempty"`
	SourceID             string          `json:"source_id,omitempty"`
	CorrectionOfMemoryID json.RawMessage `json:"correction_of_memory_id,omitempty"`
	CorrectedAt          string          `json:"corrected_at,omitempty"`
	ExpiresAt            string          `json:"expires_at,omitempty"`
}

type canonicalMemoryImportItemBody canonicalMemoryMutationBody

func (b canonicalMemoryMutationBody) toUpdateRequest(
	threadID int64,
	memoryID int64,
	viewerID int64,
) (*appagentthread.UpdateMemoryRequest, *canonicalError) {
	scope, public := canonicalMemoryRequiredScope(b.Scope)
	if public != nil {
		return nil, public
	}
	runID, public := canonicalMemoryOptionalBodyID(b.RunID, "run_id")
	if public != nil {
		return nil, public
	}
	metadata, public := canonicalMemoryMetadataPayload(b.Metadata)
	if public != nil {
		return nil, public
	}
	correctionOfMemoryID, public := canonicalMemoryOptionalBodyID(b.CorrectionOfMemoryID, "correction_of_memory_id")
	if public != nil {
		return nil, public
	}
	correctedAt, public := canonicalMemoryBodyTime(b.CorrectedAt, "corrected_at")
	if public != nil {
		return nil, public
	}
	expiresAt, public := canonicalMemoryBodyTime(b.ExpiresAt, "expires_at")
	if public != nil {
		return nil, public
	}
	return &appagentthread.UpdateMemoryRequest{
		ThreadID: threadID, MemoryID: memoryID, ActorID: viewerID, ViewerID: viewerID,
		RunID: runID, Scope: scope, Content: b.Content,
		Metadata: metadata, Score: b.Score, Confidence: b.Confidence,
		SourceType: b.SourceType, SourceID: b.SourceID,
		CorrectionOfMemoryID: correctionOfMemoryID, CorrectedAt: correctedAt, ExpiresAt: expiresAt,
	}, nil
}

func canonicalMemoryImportItems(
	items []canonicalMemoryImportItemBody,
) ([]appagentthread.ImportMemoryItem, *canonicalError) {
	if len(items) == 0 {
		return nil, canonicalInvalidRequest("Memory import items are required", "empty_memory_import")
	}
	if len(items) > 100 {
		return nil, canonicalInvalidRequest("Memory import item count exceeds limit", "memory_import_limit_exceeded")
	}
	result := make([]appagentthread.ImportMemoryItem, 0, len(items))
	for _, item := range items {
		body := canonicalMemoryMutationBody(item)
		runID, public := canonicalMemoryOptionalBodyID(body.RunID, "run_id")
		if public != nil {
			return nil, public
		}
		metadata, public := canonicalMemoryMetadataPayload(body.Metadata)
		if public != nil {
			return nil, public
		}
		correctionOfMemoryID, public := canonicalMemoryOptionalBodyID(body.CorrectionOfMemoryID, "correction_of_memory_id")
		if public != nil {
			return nil, public
		}
		correctedAt, public := canonicalMemoryBodyTime(body.CorrectedAt, "corrected_at")
		if public != nil {
			return nil, public
		}
		expiresAt, public := canonicalMemoryBodyTime(body.ExpiresAt, "expires_at")
		if public != nil {
			return nil, public
		}
		result = append(result, appagentthread.ImportMemoryItem{
			RunID: runID, Scope: appagentthread.MemoryScope(body.Scope), Content: body.Content,
			Metadata: metadata, Score: body.Score, Confidence: body.Confidence,
			SourceType: body.SourceType, SourceID: body.SourceID,
			CorrectionOfMemoryID: correctionOfMemoryID, CorrectedAt: correctedAt, ExpiresAt: expiresAt,
		})
	}
	return result, nil
}

func canonicalMemoryThreadScope(
	ctx context.Context,
	c *app.RequestContext,
	requestLog *canonicalRequestLog,
) (context.Context, int64, bool) {
	if !requireCanonicalAgentThreadService(ctx, c) {
		return ctx, 0, false
	}
	threadID, public := canonicalPathID(c, "thread_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return ctx, 0, false
	}
	requestLog.ThreadID = threadID
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return ctx, 0, false
	}
	ctx = canonicalProductThreadAccessContext(ctx, spaceID, threadID, 0)
	if err := authorizeCanonicalProductThreadAccess(ctx, spaceID, threadID, 0); err != nil {
		writeCanonicalApplicationError(ctx, c, err)
		return ctx, 0, false
	}
	return ctx, threadID, true
}

func canonicalMemoryRouteScope(
	ctx context.Context,
	c *app.RequestContext,
	requestLog *canonicalRequestLog,
) (context.Context, int64, int64, bool) {
	ctx, threadID, ok := canonicalMemoryThreadScope(ctx, c, requestLog)
	if !ok {
		return ctx, 0, 0, false
	}
	memoryID, public := canonicalProductPathID(c, "memory_id")
	if public != nil {
		writeCanonicalError(ctx, c, public.status, *public)
		return ctx, 0, 0, false
	}
	requestLog.ResourceID = strconv.FormatInt(memoryID, 10)
	return ctx, threadID, memoryID, true
}

func canonicalMemoryQuery(c *app.RequestContext) string {
	return canonicalCleanString(strings.TrimSpace(c.Query("q")), 512)
}

func canonicalOptionalQueryID(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func canonicalMemoryScopesFromQuery(c *app.RequestContext) ([]appagentthread.MemoryScope, *canonicalError) {
	values := make([]string, 0)
	if scope := strings.TrimSpace(c.Query("scope")); scope != "" {
		values = append(values, scope)
	}
	for _, raw := range c.QueryArgs().PeekAll("scopes") {
		values = append(values, strings.Split(string(raw), ",")...)
	}
	return canonicalMemoryScopes(values...)
}

func canonicalMemoryScopes(values ...string) ([]appagentthread.MemoryScope, *canonicalError) {
	result := make([]appagentthread.MemoryScope, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		scope := strings.TrimSpace(value)
		if scope == "" {
			continue
		}
		switch scope {
		case string(appagentthread.MemoryScopeThread), string(appagentthread.MemoryScopeRun), string(appagentthread.MemoryScopeLongTerm):
		default:
			return nil, canonicalInvalidRequest("Memory scope is invalid", "invalid_memory_scope")
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, appagentthread.MemoryScope(scope))
	}
	return result, nil
}

func canonicalMemoryRequiredScope(value string) (appagentthread.MemoryScope, *canonicalError) {
	scope := strings.TrimSpace(value)
	if scope == "" {
		return "", canonicalInvalidRequest("Memory scope is required", "missing_memory_scope")
	}
	scopes, public := canonicalMemoryScopes(scope)
	if public != nil {
		return "", public
	}
	if len(scopes) != 1 {
		return "", canonicalInvalidRequest("Memory scope is required", "missing_memory_scope")
	}
	return scopes[0], nil
}

func canonicalMemoryOptionalBodyID(raw json.RawMessage, name string) (int64, *canonicalError) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, nil
	}
	value := string(trimmed)
	if trimmed[0] == '"' {
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return 0, canonicalInvalidRequest(name+" must be a positive decimal ID", "invalid_"+name)
		}
	}
	id, public := parseCanonicalPositiveID(
		strings.TrimSpace(value),
		"invalid_request",
		name+" must be a positive decimal ID",
		"invalid_"+name,
	)
	if public != nil {
		public.status = consts.StatusUnprocessableEntity
		return 0, public
	}
	return id, nil
}

func canonicalMemoryExportLimit(c *app.RequestContext) (int32, *canonicalError) {
	value, public := canonicalProductQueryInt64Value(c, "limit")
	if public != nil {
		return 0, public
	}
	if value == 0 {
		return 100, nil
	}
	if value > math.MaxInt32 {
		return 0, canonicalProductInvalidQuery("limit")
	}
	return int32(value), nil
}

func canonicalMemoryMetadataPayload(raw json.RawMessage) (string, *canonicalError) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil || metadata == nil {
		return "", canonicalInvalidRequest("Memory metadata must be an object", "invalid_metadata")
	}
	if len(metadata) == 0 {
		return "", nil
	}
	if len(metadata) > canonicalMaxThreadMetadataKeys {
		return "", canonicalInvalidRequest("Memory metadata supports at most 16 keys", "invalid_metadata_count")
	}
	projected := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) || canonicalProtectedMetadataKey(key) || canonicalProtectedRunMetadataKey(key) {
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

func canonicalMemoryBodyTime(value string, name string) (int64, *canonicalError) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, canonicalInvalidRequest("Invalid time field: "+name, "invalid_time")
	}
	return parsed.UTC().UnixMilli(), nil
}

func canonicalProductMemoriesToAPI(summaries []*appagentthread.MemorySummary) ([]*canonicalProductMemory, error) {
	result := make([]*canonicalProductMemory, 0, len(summaries))
	for _, summary := range summaries {
		memory, err := projectCanonicalProductMemory(summary)
		if err != nil {
			return nil, err
		}
		if memory != nil {
			result = append(result, memory)
		}
	}
	return result, nil
}

func canonicalProductMemoryAuditsToAPI(summaries []*appagentthread.MemoryAuditEventSummary) ([]*canonicalProductMemoryAudit, error) {
	result := make([]*canonicalProductMemoryAudit, 0, len(summaries))
	for _, summary := range summaries {
		event, err := projectCanonicalProductMemoryAudit(summary)
		if err != nil {
			return nil, err
		}
		if event != nil {
			result = append(result, event)
		}
	}
	return result, nil
}

func canonicalProductGuardrailAuditsToAPI(summaries []*appagentthread.GuardrailAuditEventSummary) ([]*canonicalProductGuardrailAudit, error) {
	result := make([]*canonicalProductGuardrailAudit, 0, len(summaries))
	for _, summary := range summaries {
		event, err := projectCanonicalProductGuardrailAudit(summary)
		if err != nil {
			return nil, err
		}
		if event != nil {
			result = append(result, event)
		}
	}
	return result, nil
}

func canonicalProductMCPRuntimeAuditsToAPI(summaries []*appagentthread.MCPRuntimeAuditEventSummary) ([]*canonicalProductMCPRuntimeAudit, error) {
	result := make([]*canonicalProductMCPRuntimeAudit, 0, len(summaries))
	for _, summary := range summaries {
		event, err := projectCanonicalProductMCPRuntimeAudit(summary)
		if err != nil {
			return nil, err
		}
		if event != nil {
			result = append(result, event)
		}
	}
	return result, nil
}

func requireCanonicalGuardrailAuditRepository(ctx context.Context, c *app.RequestContext) bool {
	if appagentthread.SVC != nil && appagentthread.SVC.GuardrailAuditRepository != nil {
		return true
	}
	writeCanonicalAuditDependencyUnavailable(ctx, c, "guardrail_audit_repository_unavailable")
	return false
}

func requireCanonicalMCPRuntimeAuditRepository(ctx context.Context, c *app.RequestContext) bool {
	if appagentthread.SVC != nil && appagentthread.SVC.MCPRuntimeAuditRepository != nil {
		return true
	}
	writeCanonicalAuditDependencyUnavailable(ctx, c, "mcp_runtime_audit_repository_unavailable")
	return false
}

func writeCanonicalAuditDependencyUnavailable(ctx context.Context, c *app.RequestContext, errorClass string) {
	writeCanonicalError(ctx, c, consts.StatusServiceUnavailable, *newCanonicalError(
		consts.StatusServiceUnavailable,
		"dependency_unavailable",
		"Required service is unavailable",
		errorClass,
		true,
	))
}

func writeCanonicalMemoryAuditApplicationError(ctx context.Context, c *app.RequestContext, err error) {
	if canonicalMemoryAuditDependencyUnavailable(err) {
		writeCanonicalAuditDependencyUnavailable(ctx, c, "memory_audit_dependency_unavailable")
		return
	}
	writeCanonicalApplicationError(ctx, c, err)
}

func canonicalMemoryAuditDependencyUnavailable(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "guardrail audit repository is not initialized") ||
		strings.Contains(message, "mcp runtime audit repository is not initialized") ||
		strings.Contains(message, "agent thread service is not initialized")
}

func writeCanonicalMemoryNotFound(ctx context.Context, c *app.RequestContext) {
	writeCanonicalError(ctx, c, consts.StatusNotFound, *newCanonicalError(
		consts.StatusNotFound,
		"resource_not_found",
		"Resource not found",
		"resource_not_found",
		false,
	))
}

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
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
)

const (
	defaultWorkbenchThreadTitle     = "新建任务"
	defaultRunEventStreamIntervalMs = int64(1000)
	defaultRunEventStreamTimeoutMs  = int64(30000)
	minRunEventStreamIntervalMs     = int64(10)
	maxRunEventStreamIntervalMs     = int64(5000)
	minRunEventStreamTimeoutMs      = int64(1)
	maxRunEventStreamTimeoutMs      = int64(60000)
)

func workbenchThreadErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	if errors.Is(err, appagentthread.ErrInvalidRuntimeConfig) {
		c.JSON(consts.StatusBadRequest, map[string]any{
			"code": consts.StatusBadRequest,
			"msg":  "invalid agent runtime configuration",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrActiveRunExists) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code": consts.StatusConflict,
			"msg":  "thread already has an active run",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrUnsupportedMultitaskStrategy) {
		c.JSON(consts.StatusNotImplemented, map[string]any{
			"code": consts.StatusNotImplemented,
			"msg":  "multitask strategy is not supported",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrThreadAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "thread access denied",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrArtifactAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "artifact access denied",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrMemoryAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "memory access denied",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrGuardrailAuditAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "guardrail audit access denied",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrMCPRuntimeAuditAccessDenied) {
		c.JSON(consts.StatusForbidden, map[string]any{
			"code": consts.StatusForbidden,
			"msg":  "mcp runtime audit access denied",
		})
		return
	}
	var scanBlocked *appagentthread.ArtifactContentBlockedByScanError
	if errors.As(err, &scanBlocked) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code":   consts.StatusConflict,
			"msg":    "artifact content blocked by scan policy",
			"reason": scanBlocked.Reason,
		})
		return
	}
	if errors.Is(err, appagentthread.ErrArtifactScanJobRetryNotAllowed) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code": consts.StatusConflict,
			"msg":  "artifact scan job cannot be retried",
		})
		return
	}
	if errors.Is(err, appagentthread.ErrArtifactSignedURLNotSupported) {
		c.JSON(consts.StatusConflict, map[string]any{
			"code": consts.StatusConflict,
			"msg":  "artifact signed url is not supported",
		})
		return
	}
	internalServerErrorResponse(ctx, c, err)
}

func workbenchViewerIDFromCtx(ctx context.Context) int64 {
	if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
		return *uid
	}
	if apiKey := ctxutil.GetApiAuthFromCtx(ctx); apiKey != nil {
		return apiKey.UserID
	}
	return 0
}

func workbenchThreadAccessContext(
	ctx context.Context,
	threadID int64,
	runID int64,
) context.Context {
	return appagentthread.WithThreadAccessRequest(ctx, appagentthread.ThreadAccessRequest{
		ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID,
		RunID:    runID,
	})
}

func isWorkbenchRunTerminal(status appagentthread.RunStatus) bool {
	return status == appagentthread.RunStatusSucceeded ||
		status == appagentthread.RunStatusFailed ||
		status == appagentthread.RunStatusInterrupted ||
		status == appagentthread.RunStatusCanceled
}

func clampRunEventStreamDuration(valueMs, defaultMs, minMs, maxMs int64) time.Duration {
	if valueMs <= 0 {
		valueMs = defaultMs
	}
	if valueMs < minMs {
		valueMs = minMs
	}
	if valueMs > maxMs {
		valueMs = maxMs
	}

	return time.Duration(valueMs) * time.Millisecond
}

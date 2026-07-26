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
)

func serveCanonicalEntrypoint(ctx context.Context, c *app.RequestContext) {
	if !requireCanonicalAPI(ctx, c) {
		return
	}

	writeCanonicalError(ctx, c, consts.StatusNotImplemented, canonicalError{
		Detail:     "Canonical Workbench API is not implemented",
		Code:       "canonical_not_implemented",
		Retryable:  false,
		errorClass: "canonical_not_implemented",
	})
}

// CreateCanonicalThread exposes the gated canonical thread-create contract.
func CreateCanonicalThread(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// SearchCanonicalThreads exposes the gated canonical thread-search contract.
func SearchCanonicalThreads(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalThread exposes the gated canonical thread-read contract.
func GetCanonicalThread(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// PatchCanonicalThread exposes the gated canonical thread-patch contract.
func PatchCanonicalThread(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// DeleteCanonicalThread exposes the gated canonical thread-delete contract.
func DeleteCanonicalThread(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalThreadState exposes the gated canonical state-read contract.
func GetCanonicalThreadState(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// UpdateCanonicalThreadState exposes the gated canonical state-update contract.
func UpdateCanonicalThreadState(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalThreadHistory exposes the gated canonical history-read contract.
func GetCanonicalThreadHistory(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// PostCanonicalThreadHistory exposes the gated canonical history-search contract.
func PostCanonicalThreadHistory(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadMessages exposes the gated canonical thread-message contract.
func ListCanonicalThreadMessages(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalRuns exposes the gated canonical run-list contract.
func ListCanonicalRuns(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// CreateCanonicalRun exposes the gated canonical run-create contract.
func CreateCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// StreamCanonicalRun exposes the gated canonical run-create stream contract.
func StreamCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// WaitCanonicalRun exposes the gated canonical run-wait contract.
func WaitCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalRun exposes the gated canonical run-read contract.
func GetCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ReconnectCanonicalRunStream exposes the gated canonical reconnect contract.
func ReconnectCanonicalRunStream(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// JoinCanonicalRun exposes the gated canonical run-join contract.
func JoinCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// CancelCanonicalRun exposes the gated canonical run-cancel contract.
func CancelCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ResumeCanonicalRun exposes the gated canonical run-resume contract.
func ResumeCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalRunEvents exposes the gated canonical run-event contract.
func ListCanonicalRunEvents(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalRunMessages exposes the gated canonical run-message contract.
func ListCanonicalRunMessages(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

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

// StreamCanonicalRun exposes the gated canonical run-create stream contract.
func StreamCanonicalRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ReconnectCanonicalRunStream exposes the gated canonical reconnect contract.
func ReconnectCanonicalRunStream(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

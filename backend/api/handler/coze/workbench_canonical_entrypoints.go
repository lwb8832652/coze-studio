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

// AppendCanonicalThreadMessage exposes the gated canonical message append contract.
func AppendCanonicalThreadMessage(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GenerateCanonicalThreadSuggestions exposes the gated canonical suggestion contract.
func GenerateCanonicalThreadSuggestions(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadArtifacts exposes the gated canonical artifact list contract.
func ListCanonicalThreadArtifacts(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalThreadArtifactContent exposes the gated canonical artifact content contract.
func GetCanonicalThreadArtifactContent(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalThreadArtifactSignedURL exposes the gated canonical signed URL contract.
func GetCanonicalThreadArtifactSignedURL(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// DeleteCanonicalThreadArtifact exposes the gated canonical artifact delete contract.
func DeleteCanonicalThreadArtifact(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// RestoreCanonicalThreadArtifact exposes the gated canonical artifact restore contract.
func RestoreCanonicalThreadArtifact(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ReviewCanonicalThreadArtifactScan exposes the gated canonical scan review contract.
func ReviewCanonicalThreadArtifactScan(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadArtifactScanJobs exposes the gated canonical scan job list contract.
func ListCanonicalThreadArtifactScanJobs(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// RetryCanonicalThreadArtifactScanJob exposes the gated canonical scan job retry contract.
func RetryCanonicalThreadArtifactScanJob(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// GetCanonicalThreadTokenUsage exposes the gated canonical token usage contract.
func GetCanonicalThreadTokenUsage(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadMemories exposes the gated canonical memory list contract.
func ListCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// UpdateCanonicalThreadMemory exposes the gated canonical memory update contract.
func UpdateCanonicalThreadMemory(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// DeleteCanonicalThreadMemory exposes the gated canonical memory delete contract.
func DeleteCanonicalThreadMemory(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// RestoreCanonicalThreadMemory exposes the gated canonical memory restore contract.
func RestoreCanonicalThreadMemory(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ClearCanonicalThreadMemories exposes the gated canonical memory clear contract.
func ClearCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ExportCanonicalThreadMemories exposes the gated canonical memory export contract.
func ExportCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ImportCanonicalThreadMemories exposes the gated canonical memory import contract.
func ImportCanonicalThreadMemories(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadMemoryAuditEvents exposes the gated canonical memory audit contract.
func ListCanonicalThreadMemoryAuditEvents(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadGuardrailAuditEvents exposes the gated canonical guardrail audit contract.
func ListCanonicalThreadGuardrailAuditEvents(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ExportCanonicalThreadGuardrailAuditEvents exposes the gated canonical guardrail export contract.
func ExportCanonicalThreadGuardrailAuditEvents(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// ListCanonicalThreadMCPRuntimeAuditEvents exposes the gated canonical MCP audit contract.
func ListCanonicalThreadMCPRuntimeAuditEvents(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

// RetryCanonicalSubagentRun exposes the gated canonical subagent retry contract.
func RetryCanonicalSubagentRun(ctx context.Context, c *app.RequestContext) {
	serveCanonicalEntrypoint(ctx, c)
}

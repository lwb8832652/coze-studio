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
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestRegisterIncludesWorkbenchTaskThreadRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	list := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads?space_id=1", nil)
	createThread := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads", nil)
	detail := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1", nil)
	messages := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/messages", nil)
	appendMessage := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/messages", nil)
	suggestions := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/suggestions", nil)
	runs := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/runs", nil)
	createRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/runs", nil)
	resumeRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/runs/2/resume", nil)
	retrySubagentRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/runs/2/retry", nil)
	runEvents := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events", nil)
	runEventsStream := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events/stream?timeout_ms=1", nil)
	tokenUsage := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/token_usage", nil)
	memories := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/memories", nil)
	exportMemories := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/memories/export", nil)
	importMemories := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/memories/import", nil)
	memoryAudits := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/memories/audit_events", nil)
	guardrailAudits := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/guardrail_audit_events", nil)
	exportGuardrailAudits := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/guardrail_audit_events/export", nil)
	updateMemory := ut.PerformRequest(h.Engine, http.MethodPut, "/api/workbench/task_threads/1/memories/2", nil)
	deleteMemory := ut.PerformRequest(h.Engine, http.MethodDelete, "/api/workbench/task_threads/1/memories/2", nil)
	restoreMemory := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/memories/2/restore", nil)
	clearMemories := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/memories/clear", nil)
	artifacts := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/artifacts", nil)
	artifactScanJobs := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/artifact_scan_jobs", nil)
	retryArtifactScanJob := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/artifact_scan_jobs/2/retry", nil)
	reviewArtifactScan := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/artifacts/2/scan_review", nil)
	artifactContent := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/artifacts/2/content", nil)
	artifactSignedURL := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/artifacts/2/signed_url", nil)
	deleteArtifact := ut.PerformRequest(h.Engine, http.MethodDelete, "/api/workbench/task_threads/1/artifacts/2", nil)
	restoreArtifact := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/artifacts/2/restore", nil)

	require.NotEqual(t, http.StatusNotFound, list.Code)
	require.NotEqual(t, http.StatusNotFound, createThread.Code)
	require.NotEqual(t, http.StatusNotFound, detail.Code)
	require.NotEqual(t, http.StatusNotFound, messages.Code)
	require.NotEqual(t, http.StatusNotFound, appendMessage.Code)
	require.NotEqual(t, http.StatusNotFound, suggestions.Code)
	require.NotEqual(t, http.StatusNotFound, runs.Code)
	require.NotEqual(t, http.StatusNotFound, createRun.Code)
	require.NotEqual(t, http.StatusNotFound, resumeRun.Code)
	require.NotEqual(t, http.StatusNotFound, retrySubagentRun.Code)
	require.NotEqual(t, http.StatusNotFound, runEvents.Code)
	require.NotEqual(t, http.StatusNotFound, runEventsStream.Code)
	require.NotEqual(t, http.StatusNotFound, tokenUsage.Code)
	require.NotEqual(t, http.StatusNotFound, memories.Code)
	require.NotEqual(t, http.StatusNotFound, exportMemories.Code)
	require.NotEqual(t, http.StatusNotFound, importMemories.Code)
	require.NotEqual(t, http.StatusNotFound, memoryAudits.Code)
	require.NotEqual(t, http.StatusNotFound, guardrailAudits.Code)
	require.NotEqual(t, http.StatusNotFound, exportGuardrailAudits.Code)
	require.NotEqual(t, http.StatusNotFound, updateMemory.Code)
	require.NotEqual(t, http.StatusNotFound, deleteMemory.Code)
	require.NotEqual(t, http.StatusNotFound, restoreMemory.Code)
	require.NotEqual(t, http.StatusNotFound, clearMemories.Code)
	require.NotEqual(t, http.StatusNotFound, artifacts.Code)
	require.NotEqual(t, http.StatusNotFound, artifactScanJobs.Code)
	require.NotEqual(t, http.StatusNotFound, retryArtifactScanJob.Code)
	require.NotEqual(t, http.StatusNotFound, reviewArtifactScan.Code)
	require.NotEqual(t, http.StatusNotFound, artifactContent.Code)
	require.NotEqual(t, http.StatusNotFound, artifactSignedURL.Code)
	require.NotEqual(t, http.StatusNotFound, deleteArtifact.Code)
	require.NotEqual(t, http.StatusNotFound, restoreArtifact.Code)
}

func TestRegisterIncludesLangGraphThreadRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	createThread := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads", nil)
	getThread := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1", nil)
	patchThread := ut.PerformRequest(h.Engine, http.MethodPatch, "/api/threads/1", nil)
	deleteThread := ut.PerformRequest(h.Engine, http.MethodDelete, "/api/threads/1", nil)
	getThreadState := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/state", nil)
	postThreadState := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/state", nil)
	getThreadHistory := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/history", nil)
	postThreadHistory := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/history", nil)
	getCheckpointResume := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/checkpoints/2/resume", nil)
	searchThreads := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/search", nil)

	require.NotEqual(t, http.StatusNotFound, createThread.Code)
	require.NotEqual(t, http.StatusNotFound, getThread.Code)
	require.NotEqual(t, http.StatusNotFound, patchThread.Code)
	require.NotEqual(t, http.StatusNotFound, deleteThread.Code)
	require.NotEqual(t, http.StatusNotFound, getThreadState.Code)
	require.NotEqual(t, http.StatusNotFound, postThreadState.Code)
	require.NotEqual(t, http.StatusNotFound, getThreadHistory.Code)
	require.NotEqual(t, http.StatusNotFound, postThreadHistory.Code)
	require.NotEqual(t, http.StatusNotFound, getCheckpointResume.Code)
	require.NotEqual(t, http.StatusNotFound, searchThreads.Code)
}

func TestRegisterIncludesLangGraphRunRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	createStatelessRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs", nil)
	createStatelessRunStream := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/stream", nil)
	createStatelessRunWait := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/wait", nil)
	getStatelessRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2", nil)
	statelessRunMessages := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2/messages", nil)
	statelessRunFeedback := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2/feedback", nil)
	cancelStatelessRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/2/cancel", nil)
	streamStatelessRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2/stream", nil)
	joinStatelessRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/2/join", nil)
	joinStatelessRunStream := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2/join", nil)
	createRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs", nil)
	createRunStream := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/stream", nil)
	createRunWait := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/wait", nil)
	threadMessages := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/messages", nil)
	listRuns := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs", nil)
	getRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2", nil)
	cancelRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/2/cancel", nil)
	streamRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2/stream", nil)
	postStreamRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/2/stream", nil)
	runMessages := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2/messages", nil)
	runEvents := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2/events", nil)
	joinRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/2/join", nil)
	joinRunStream := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2/join", nil)

	require.NotEqual(t, http.StatusNotFound, createStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, createStatelessRunStream.Code)
	require.NotEqual(t, http.StatusNotFound, createStatelessRunWait.Code)
	require.NotEqual(t, http.StatusNotFound, getStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, statelessRunMessages.Code)
	require.NotEqual(t, http.StatusNotFound, statelessRunFeedback.Code)
	require.NotEqual(t, http.StatusNotFound, cancelStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, streamStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinStatelessRunStream.Code)
	require.NotEqual(t, http.StatusNotFound, createRun.Code)
	require.NotEqual(t, http.StatusNotFound, createRunStream.Code)
	require.NotEqual(t, http.StatusNotFound, createRunWait.Code)
	require.NotEqual(t, http.StatusNotFound, threadMessages.Code)
	require.NotEqual(t, http.StatusNotFound, listRuns.Code)
	require.NotEqual(t, http.StatusNotFound, getRun.Code)
	require.NotEqual(t, http.StatusNotFound, cancelRun.Code)
	require.NotEqual(t, http.StatusNotFound, streamRun.Code)
	require.NotEqual(t, http.StatusNotFound, postStreamRun.Code)
	require.NotEqual(t, http.StatusNotFound, runMessages.Code)
	require.NotEqual(t, http.StatusNotFound, runEvents.Code)
	require.NotEqual(t, http.StatusNotFound, joinRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinRunStream.Code)
}

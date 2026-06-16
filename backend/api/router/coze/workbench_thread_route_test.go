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
	detail := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1", nil)
	messages := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/messages", nil)
	appendMessage := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/messages", nil)
	runs := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/runs", nil)
	createRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/task_threads/1/runs", nil)
	runEvents := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events", nil)
	runEventsStream := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/run_events/stream?timeout_ms=1", nil)
	tokenUsage := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/task_threads/1/token_usage", nil)

	require.NotEqual(t, http.StatusNotFound, list.Code)
	require.NotEqual(t, http.StatusNotFound, detail.Code)
	require.NotEqual(t, http.StatusNotFound, messages.Code)
	require.NotEqual(t, http.StatusNotFound, appendMessage.Code)
	require.NotEqual(t, http.StatusNotFound, runs.Code)
	require.NotEqual(t, http.StatusNotFound, createRun.Code)
	require.NotEqual(t, http.StatusNotFound, runEvents.Code)
	require.NotEqual(t, http.StatusNotFound, runEventsStream.Code)
	require.NotEqual(t, http.StatusNotFound, tokenUsage.Code)
}

func TestRegisterIncludesLangGraphThreadRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	createThread := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads", nil)
	getThread := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1", nil)
	searchThreads := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/search", nil)

	require.NotEqual(t, http.StatusNotFound, createThread.Code)
	require.NotEqual(t, http.StatusNotFound, getThread.Code)
	require.NotEqual(t, http.StatusNotFound, searchThreads.Code)
}

func TestRegisterIncludesLangGraphRunRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	createStatelessRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs", nil)
	createStatelessRunStream := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/stream", nil)
	getStatelessRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2", nil)
	cancelStatelessRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/2/cancel", nil)
	streamStatelessRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2/stream", nil)
	joinStatelessRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/runs/2/join", nil)
	joinStatelessRunStream := ut.PerformRequest(h.Engine, http.MethodGet, "/api/runs/2/join", nil)
	createRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs", nil)
	createRunStream := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/stream", nil)
	listRuns := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs", nil)
	getRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2", nil)
	cancelRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/2/cancel", nil)
	streamRun := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2/stream", nil)
	joinRun := ut.PerformRequest(h.Engine, http.MethodPost, "/api/threads/1/runs/2/join", nil)
	joinRunStream := ut.PerformRequest(h.Engine, http.MethodGet, "/api/threads/1/runs/2/join", nil)

	require.NotEqual(t, http.StatusNotFound, createStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, createStatelessRunStream.Code)
	require.NotEqual(t, http.StatusNotFound, getStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, cancelStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, streamStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinStatelessRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinStatelessRunStream.Code)
	require.NotEqual(t, http.StatusNotFound, createRun.Code)
	require.NotEqual(t, http.StatusNotFound, createRunStream.Code)
	require.NotEqual(t, http.StatusNotFound, listRuns.Code)
	require.NotEqual(t, http.StatusNotFound, getRun.Code)
	require.NotEqual(t, http.StatusNotFound, cancelRun.Code)
	require.NotEqual(t, http.StatusNotFound, streamRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinRun.Code)
	require.NotEqual(t, http.StatusNotFound, joinRunStream.Code)
}

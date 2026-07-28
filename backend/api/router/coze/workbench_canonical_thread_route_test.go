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
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

type routeExpectation struct {
	method string
	path   string
}

var canonicalThreadRoutes = []routeExpectation{
	{http.MethodPost, "/api/workbench/threads"},
	{http.MethodPost, "/api/workbench/threads/search"},
	{http.MethodGet, "/api/workbench/threads/:thread_id"},
	{http.MethodPatch, "/api/workbench/threads/:thread_id"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/state"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/state"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/history"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/history"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/messages"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/stream"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/wait"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/stream"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/join"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/cancel"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/resume"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/events"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/runs/:run_id/messages"},
}

var canonicalProductRoutes = []routeExpectation{
	{http.MethodPost, "/api/workbench/threads/:thread_id/messages"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/suggestions"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/uploads"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/uploads"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id/uploads/:file_id"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifacts"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/content"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id/artifacts/:artifact_id"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/artifact_scan_jobs"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/token_usage"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/memories"},
	{http.MethodPut, "/api/workbench/threads/:thread_id/memories/:memory_id"},
	{http.MethodDelete, "/api/workbench/threads/:thread_id/memories/:memory_id"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/memories/:memory_id/restore"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/memories/clear"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/memories/export"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/memories/import"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/memories/audit_events"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/guardrail_audit_events"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/guardrail_audit_events/export"},
	{http.MethodGet, "/api/workbench/threads/:thread_id/mcp_runtime_audit_events"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/retry"},
}

func canonicalRouteSnapshot() []routeExpectation {
	result := append([]routeExpectation{}, canonicalThreadRoutes...)
	return append(result, canonicalProductRoutes...)
}

var forbiddenCanonicalThreadRoutes = []routeExpectation{
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/stream"},
	{http.MethodPost, "/api/workbench/threads/:thread_id/runs/:run_id/join"},
}

var workbenchTaskThreadRouteSnapshot = []routeExpectation{
	{http.MethodGet, "/api/workbench/task_threads"},
	{http.MethodPost, "/api/workbench/task_threads"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/artifact_scan_jobs"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/artifact_scan_jobs/:job_id/retry"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/artifacts"},
	{http.MethodDelete, "/api/workbench/task_threads/:thread_id/artifacts/:artifact_id"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/content"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/restore"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/scan_review"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/guardrail_audit_events"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/guardrail_audit_events/export"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/mcp_runtime_audit_events"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/memories"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/memories/audit_events"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/memories/clear"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/memories/export"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/memories/import"},
	{http.MethodDelete, "/api/workbench/task_threads/:thread_id/memories/:memory_id"},
	{http.MethodPut, "/api/workbench/task_threads/:thread_id/memories/:memory_id"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/memories/:memory_id/restore"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/messages"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/messages"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/run_events"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/run_events/stream"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/runs"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/runs"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/runs/:run_id/cancel"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/runs/:run_id/resume"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/runs/:run_id/retry"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/suggestions"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/token_usage"},
	{http.MethodGet, "/api/workbench/task_threads/:thread_id/uploads"},
	{http.MethodPost, "/api/workbench/task_threads/:thread_id/uploads"},
	{http.MethodDelete, "/api/workbench/task_threads/:thread_id/uploads/:filename"},
}

var langGraphThreadRouteSnapshot = []routeExpectation{
	{http.MethodPost, "/api/threads"},
	{http.MethodPost, "/api/threads/search"},
	{http.MethodGet, "/api/threads/:thread_id"},
	{http.MethodPatch, "/api/threads/:thread_id"},
	{http.MethodDelete, "/api/threads/:thread_id"},
	{http.MethodGet, "/api/threads/:thread_id/checkpoints/:checkpoint_id/resume"},
	{http.MethodGet, "/api/threads/:thread_id/history"},
	{http.MethodPost, "/api/threads/:thread_id/history"},
	{http.MethodGet, "/api/threads/:thread_id/messages"},
	{http.MethodGet, "/api/threads/:thread_id/runs"},
	{http.MethodPost, "/api/threads/:thread_id/runs"},
	{http.MethodPost, "/api/threads/:thread_id/runs/stream"},
	{http.MethodPost, "/api/threads/:thread_id/runs/wait"},
	{http.MethodGet, "/api/threads/:thread_id/runs/:run_id"},
	{http.MethodPost, "/api/threads/:thread_id/runs/:run_id/cancel"},
	{http.MethodGet, "/api/threads/:thread_id/runs/:run_id/events"},
	{http.MethodGet, "/api/threads/:thread_id/runs/:run_id/join"},
	{http.MethodPost, "/api/threads/:thread_id/runs/:run_id/join"},
	{http.MethodGet, "/api/threads/:thread_id/runs/:run_id/messages"},
	{http.MethodGet, "/api/threads/:thread_id/runs/:run_id/stream"},
	{http.MethodPost, "/api/threads/:thread_id/runs/:run_id/stream"},
	{http.MethodGet, "/api/threads/:thread_id/state"},
	{http.MethodPost, "/api/threads/:thread_id/state"},
}

var langGraphStatelessRunRouteSnapshot = []routeExpectation{
	{http.MethodPost, "/api/runs"},
	{http.MethodPost, "/api/runs/stream"},
	{http.MethodPost, "/api/runs/wait"},
	{http.MethodGet, "/api/runs/:run_id"},
	{http.MethodGet, "/api/runs/:run_id/messages"},
	{http.MethodGet, "/api/runs/:run_id/feedback"},
	{http.MethodPost, "/api/runs/:run_id/cancel"},
	{http.MethodGet, "/api/runs/:run_id/stream"},
	{http.MethodPost, "/api/runs/:run_id/join"},
	{http.MethodGet, "/api/runs/:run_id/join"},
}

var retiredChatTaskPaths = []string{
	"/api/workbench/tasks",
	"/api/workbench/tasks/1",
	"/api/workbench/chat",
}

type recordingHandlerBoundary struct {
	matchedHandlerCount int
}

func (r *recordingHandlerBoundary) record(ctx context.Context, c *app.RequestContext) {
	// Global middleware runs only after Hertz resolves a route into a handler chain.
	if c.FullPath() != "" {
		r.matchedHandlerCount++
	}
	c.Next(ctx)
}

func TestWorkbenchCanonicalThreadRoutes(t *testing.T) {
	handlerBoundary := &recordingHandlerBoundary{}
	h := server.Default()
	h.Use(handlerBoundary.record)
	Register(h)
	RegisterCustomRoutes(h)

	t.Run("registers the canonical route surface", func(t *testing.T) {
		requireExactRouteSnapshot(t, h, "/api/workbench/threads", canonicalRouteSnapshot())
	})

	t.Run("excludes forbidden canonical POST variants", func(t *testing.T) {
		for _, route := range forbiddenCanonicalThreadRoutes {
			route := route
			t.Run(route.method+" "+route.path, func(t *testing.T) {
				requireUnreachableRoute(
					t,
					h,
					handlerBoundary,
					route.method,
					concreteRoutePath(route.path),
				)
			})
		}
	})

	t.Run("preserves the workbench task thread source surface", func(t *testing.T) {
		requireExactRouteSnapshot(t, h, "/api/workbench/task_threads", workbenchTaskThreadRouteSnapshot)
	})

	t.Run("preserves the LangGraph thread source surface", func(t *testing.T) {
		requireExactRouteSnapshot(t, h, "/api/threads", langGraphThreadRouteSnapshot)
	})

	t.Run("preserves the LangGraph stateless run source surface", func(t *testing.T) {
		requireExactRouteSnapshot(t, h, "/api/runs", langGraphStatelessRunRouteSnapshot)
	})

	t.Run("keeps retired ChatTask paths unreachable", func(t *testing.T) {
		for _, path := range retiredChatTaskPaths {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				requireUnreachableRoute(t, h, handlerBoundary, method, path)
			}
		}
		require.Zero(t, handlerBoundary.matchedHandlerCount)
	})
}

func requireUnreachableRoute(
	t *testing.T,
	h *server.Hertz,
	handlerBoundary *recordingHandlerBoundary,
	method string,
	path string,
) {
	t.Helper()
	matchedBefore := handlerBoundary.matchedHandlerCount
	response := ut.PerformRequest(h.Engine, method, path, nil)
	require.Equal(t, http.StatusNotFound, response.Code, "%s %s", method, path)
	require.Equal(
		t,
		matchedBefore,
		handlerBoundary.matchedHandlerCount,
		"request entered a handler chain: %s %s",
		method,
		path,
	)
}

func requireExactRouteSnapshot(
	t *testing.T,
	h *server.Hertz,
	prefix string,
	expectedRoutes []routeExpectation,
) {
	t.Helper()
	actualRoutes := make([]routeExpectation, 0, len(expectedRoutes))
	for _, route := range h.Routes() {
		if route.Path == prefix || strings.HasPrefix(route.Path, prefix+"/") {
			actualRoutes = append(actualRoutes, routeExpectation{method: route.Method, path: route.Path})
		}
	}

	require.Equal(t, sortedRouteExpectations(expectedRoutes), sortedRouteExpectations(actualRoutes))
}

func sortedRouteExpectations(routes []routeExpectation) []routeExpectation {
	sortedRoutes := append([]routeExpectation(nil), routes...)
	sort.Slice(sortedRoutes, func(i, j int) bool {
		if sortedRoutes[i].path == sortedRoutes[j].path {
			return sortedRoutes[i].method < sortedRoutes[j].method
		}
		return sortedRoutes[i].path < sortedRoutes[j].path
	})
	return sortedRoutes
}

func concreteRoutePath(path string) string {
	path = strings.ReplaceAll(path, ":thread_id", "1")
	return strings.ReplaceAll(path, ":run_id", "2")
}

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
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestRegisterIncludesWorkbenchTaskThreadRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	requireExactRouteSnapshot(t, h, "/api/workbench/task_threads", workbenchTaskThreadRouteSnapshot)
}

func TestRegisterIncludesLangGraphThreadRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	requireExactRouteSnapshot(t, h, "/api/threads", langGraphThreadRouteSnapshot)
}

func TestRegisterIncludesLangGraphRunRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	requireExactRouteSnapshot(t, h, "/api/runs", langGraphStatelessRunRouteSnapshot)
}

func TestWorkbenchSourceRouteResolution(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		requestPath  string
		wantTemplate string
	}{
		{
			name:         "LangGraph thread search stays on the static route",
			method:       http.MethodPost,
			requestPath:  "/api/threads/search",
			wantTemplate: "/api/threads/search",
		},
		{
			name:         "stateless run stream stays on the static route",
			method:       http.MethodPost,
			requestPath:  "/api/runs/stream",
			wantTemplate: "/api/runs/stream",
		},
		{
			name:         "stateless run wait stays on the static route",
			method:       http.MethodPost,
			requestPath:  "/api/runs/wait",
			wantTemplate: "/api/runs/wait",
		},
		{
			name:         "TaskThread memory export stays on the static route",
			method:       http.MethodGet,
			requestPath:  "/api/workbench/task_threads/1/memories/export",
			wantTemplate: "/api/workbench/task_threads/:thread_id/memories/export",
		},
	}

	h := server.Default()
	matchedTemplate := ""
	h.Use(func(_ context.Context, c *app.RequestContext) {
		matchedTemplate = c.FullPath()
		c.Abort()
	})
	Register(h)
	RegisterCustomRoutes(h)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matchedTemplate = ""
			ut.PerformRequest(h.Engine, test.method, test.requestPath, nil)
			require.Equal(t, test.wantTemplate, matchedTemplate)
		})
	}
}

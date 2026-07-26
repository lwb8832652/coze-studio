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

func TestRegisterExcludesLegacyWorkbenchChatRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	legacyRoutes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/workbench/chat"},
		{method: http.MethodPost, path: "/api/workbench/tasks"},
		{method: http.MethodGet, path: "/api/workbench/tasks"},
		{method: http.MethodGet, path: "/api/workbench/tasks/1"},
		{method: http.MethodPost, path: "/api/workbench/tasks/1/cancel"},
		{method: http.MethodPost, path: "/api/workbench/tasks/1/retry"},
		{method: http.MethodGet, path: "/api/workbench/tasks/1/events"},
	}

	for _, route := range legacyRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := ut.PerformRequest(h.Engine, route.method, route.path, nil)
			require.Equal(t, http.StatusNotFound, response.Code)
		})
	}
}

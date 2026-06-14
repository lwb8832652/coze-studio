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

func TestRegisterIncludesWorkbenchMCPToolRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	list := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools?space_id=1", nil)
	create := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/mcp_tools", nil)
	detail := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/mcp_tools/100", nil)
	testCall := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/mcp_tools/100/test_call", nil)

	require.NotEqual(t, http.StatusNotFound, list.Code)
	require.NotEqual(t, http.StatusNotFound, create.Code)
	require.NotEqual(t, http.StatusNotFound, detail.Code)
	require.NotEqual(t, http.StatusNotFound, testCall.Code)
}

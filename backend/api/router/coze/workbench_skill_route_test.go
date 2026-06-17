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

func TestRegisterIncludesWorkbenchSkillVersionRoutes(t *testing.T) {
	h := server.Default()
	Register(h)

	versions := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/skills/100/versions", nil)
	resources := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/skills/100/versions/200/resources", nil)
	versionExport := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/skills/100/versions/200/export", nil)
	rollback := ut.PerformRequest(h.Engine, http.MethodPost, "/api/workbench/skills/100/versions/200/rollback", nil)

	require.NotEqual(t, http.StatusNotFound, versions.Code)
	require.NotEqual(t, http.StatusNotFound, resources.Code)
	require.NotEqual(t, http.StatusNotFound, versionExport.Code)
	require.NotEqual(t, http.StatusNotFound, rollback.Code)
}

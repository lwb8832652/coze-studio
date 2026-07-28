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
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
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

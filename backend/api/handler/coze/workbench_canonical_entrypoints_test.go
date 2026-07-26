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
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

var canonicalEntrypoints = []struct {
	name    string
	handler app.HandlerFunc
}{
	{"CreateCanonicalThread", CreateCanonicalThread},
	{"SearchCanonicalThreads", SearchCanonicalThreads},
	{"GetCanonicalThread", GetCanonicalThread},
	{"PatchCanonicalThread", PatchCanonicalThread},
	{"DeleteCanonicalThread", DeleteCanonicalThread},
	{"GetCanonicalThreadState", GetCanonicalThreadState},
	{"UpdateCanonicalThreadState", UpdateCanonicalThreadState},
	{"GetCanonicalThreadHistory", GetCanonicalThreadHistory},
	{"PostCanonicalThreadHistory", PostCanonicalThreadHistory},
	{"ListCanonicalThreadMessages", ListCanonicalThreadMessages},
	{"ListCanonicalRuns", ListCanonicalRuns},
	{"CreateCanonicalRun", CreateCanonicalRun},
	{"StreamCanonicalRun", StreamCanonicalRun},
	{"WaitCanonicalRun", WaitCanonicalRun},
	{"GetCanonicalRun", GetCanonicalRun},
	{"ReconnectCanonicalRunStream", ReconnectCanonicalRunStream},
	{"JoinCanonicalRun", JoinCanonicalRun},
	{"CancelCanonicalRun", CancelCanonicalRun},
	{"ResumeCanonicalRun", ResumeCanonicalRun},
	{"ListCanonicalRunEvents", ListCanonicalRunEvents},
	{"ListCanonicalRunMessages", ListCanonicalRunMessages},
}

func TestCanonicalEntrypointDefaultsToNotFound(t *testing.T) {
	t.Setenv(canonicalEntrypointEnabledEnv, "")

	for _, entrypoint := range canonicalEntrypoints {
		entrypoint := entrypoint
		t.Run(entrypoint.name, func(t *testing.T) {
			var c app.RequestContext
			entrypoint.handler(context.Background(), &c)
			require.Equal(t, consts.StatusNotFound, c.Response.StatusCode())
			require.Empty(t, c.Response.Body())
		})
	}
}

func TestCanonicalEntrypointRequiresExplicitTrue(t *testing.T) {
	t.Setenv(canonicalEntrypointEnabledEnv, "TRUE")

	var c app.RequestContext
	CreateCanonicalThread(context.Background(), &c)
	require.Equal(t, consts.StatusNotFound, c.Response.StatusCode())
}

func TestCanonicalEntrypointEnabledFailsClosed(t *testing.T) {
	t.Setenv(canonicalEntrypointEnabledEnv, "true")

	for _, entrypoint := range canonicalEntrypoints {
		entrypoint := entrypoint
		t.Run(entrypoint.name, func(t *testing.T) {
			var c app.RequestContext
			entrypoint.handler(context.Background(), &c)
			require.Equal(t, consts.StatusNotImplemented, c.Response.StatusCode())

			var response canonicalEntrypointError
			require.NoError(t, sonic.Unmarshal(c.Response.Body(), &response))
			require.Equal(t, "canonical_not_implemented", response.Code)
			require.False(t, response.Retryable)
		})
	}
}

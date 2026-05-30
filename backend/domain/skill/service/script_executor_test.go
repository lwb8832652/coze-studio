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

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
)

func TestScriptExecutorUsesDeclarationExecutorCode(t *testing.T) {
	runner := &fakeCodeRunner{response: &coderunner.RunResponse{Result: map[string]any{"ok": true}}}
	executor := &ScriptExecutor{Runner: runner, Code: "fallback"}
	decl := &Declaration{Executor: ExecutorDeclaration{Code: " print('inline') "}}

	result, err := executor.Run(context.Background(), decl, map[string]any{"topic": "sales"})

	require.NoError(t, err)
	require.Equal(t, map[string]any{"ok": true}, result)
	require.Equal(t, "print('inline')", runner.request.Code)
	require.Equal(t, coderunner.Python, runner.request.Language)
	require.Equal(t, "sales", runner.request.Params["topic"])
}

func TestScriptExecutorRequiresCodeBeforeCallingRunner(t *testing.T) {
	runner := &fakeCodeRunner{response: &coderunner.RunResponse{Result: map[string]any{"ok": true}}}
	executor := &ScriptExecutor{Runner: runner}
	decl := &Declaration{Executor: ExecutorDeclaration{Entry: ""}}

	_, err := executor.Run(context.Background(), decl, nil)

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.ErrorContains(t, err, "script code is required")
	require.Zero(t, runner.calls)
}

type fakeCodeRunner struct {
	request  *coderunner.RunRequest
	response *coderunner.RunResponse
	calls    int
}

func (r *fakeCodeRunner) Run(ctx context.Context, request *coderunner.RunRequest) (*coderunner.RunResponse, error) {
	r.calls++
	r.request = request
	return r.response, nil
}

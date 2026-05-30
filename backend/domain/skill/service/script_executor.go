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
	"fmt"

	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
)

type ScriptExecutor struct {
	Runner coderunner.Runner
	Code   string
}

func (e *ScriptExecutor) Run(ctx context.Context, skill *Declaration, input map[string]any) (map[string]any, error) {
	if e.Runner == nil {
		return nil, fmt.Errorf("script runner is required")
	}
	resp, err := e.Runner.Run(ctx, &coderunner.RunRequest{Code: e.Code, Params: input, Language: coderunner.Python})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("script runner returned nil response")
	}
	return resp.Result, nil
}

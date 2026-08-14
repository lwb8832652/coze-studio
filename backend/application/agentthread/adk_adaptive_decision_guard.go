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

package agentthread

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk"
)

const adkMiddlewareDirectDecisionGuard ADKMiddlewareName = "direct_decision_guard"

var errADKDirectDecisionToolCall = errors.New(
	"direct adaptive decision returned a tool call",
)

type adkDirectDecisionGuard struct {
	*adk.BaseChatModelAgentMiddleware
}

func newADKDirectDecisionGuard() *adkDirectDecisionGuard {
	return &adkDirectDecisionGuard{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
	}
}

func (g *adkDirectDecisionGuard) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if g == nil || state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	last := state.Messages[len(state.Messages)-1]
	if last == nil || len(last.ToolCalls) == 0 {
		return ctx, state, nil
	}

	return ctx, state, fmt.Errorf(
		"%w: tool calls are unavailable in direct mode",
		errADKDirectDecisionToolCall,
	)
}

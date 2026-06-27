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
	"fmt"

	"github.com/cloudwego/eino/adk"
)

type ADKToolDefinitionBudgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	tokenLimit int
}

func NewADKToolDefinitionBudgetMiddleware(
	budget ADKContextBudget,
) (*ADKToolDefinitionBudgetMiddleware, error) {
	if budget.ToolDefinitionTokens <= 0 {
		return nil, fmt.Errorf("tool definition token budget must be positive")
	}
	return &ADKToolDefinitionBudgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		tokenLimit:                   budget.ToolDefinitionTokens,
	}, nil
}

func (m *ADKToolDefinitionBudgetMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || m.tokenLimit <= 0 {
		return nil, nil, fmt.Errorf("tool definition budget middleware is invalid")
	}
	if state == nil {
		return ctx, state, nil
	}

	visibleTokens := estimateADKMessagesTokens(nil, state.ToolInfos)
	deferredTokens := estimateADKMessagesTokens(nil, state.DeferredToolInfos)
	total := visibleTokens + deferredTokens
	if total > m.tokenLimit {
		return nil, nil, fmt.Errorf(
			"tool definitions exceed token budget %d: estimated %d tokens",
			m.tokenLimit,
			total,
		)
	}
	return ctx, state, nil
}

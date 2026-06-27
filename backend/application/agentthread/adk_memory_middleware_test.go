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
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKMemoryMiddlewareInjectsDeduplicatedMemoryWithinBudget(t *testing.T) {
	provider := &recordingMemoryProvider{memories: []AgentMemory{
		{
			ID:       "1",
			Scope:    "long_term",
			Content:  "preferred fact",
			Metadata: `{"user_id":99,"path":"/private/secret"}`,
			Score:    0.9,
		},
		{
			ID:      "2",
			Scope:   "thread",
			Content: " preferred fact ",
			Score:   0.8,
		},
		{
			ID:      "3",
			Scope:   "thread",
			Content: strings.Repeat("x", 400),
			Score:   0.7,
		},
	}}
	run := &RunSummary{RunID: 20, ThreadID: 10}
	middleware, err := NewADKMemoryMiddleware(run, provider, ADKContextBudget{
		ContextWindowTokens:   128000,
		SummarizationTokens:   120000,
		SummarizationMessages: 200,
		MemoryTokens:          32,
	})
	require.NoError(t, err)
	runCtx := &adk.ChatModelAgentContext{Instruction: "base instruction"}

	_, got, err := middleware.BeforeAgent(context.Background(), runCtx)

	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Same(t, run, provider.run)
	require.Equal(t, 1, strings.Count(got.Instruction, "preferred fact"))
	require.NotContains(t, got.Instruction, strings.Repeat("x", 400))
	require.NotContains(t, got.Instruction, "user_id")
	require.NotContains(t, got.Instruction, "/private/secret")
	require.NotContains(t, got.Instruction, `"id"`)

	_, gotAgain, err := middleware.BeforeAgent(context.Background(), got)
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Equal(t, got.Instruction, gotAgain.Instruction)
}

func TestADKMemoryMiddlewareReturnsProviderError(t *testing.T) {
	provider := &recordingMemoryProvider{err: errors.New("memory unavailable")}
	middleware, err := NewADKMemoryMiddleware(
		&RunSummary{RunID: 20, ThreadID: 10},
		provider,
		ADKContextBudget{
			ContextWindowTokens:   128000,
			SummarizationTokens:   120000,
			SummarizationMessages: 200,
			MemoryTokens:          4000,
		},
	)
	require.NoError(t, err)

	_, _, err = middleware.BeforeAgent(
		context.Background(),
		&adk.ChatModelAgentContext{},
	)

	require.ErrorContains(t, err, "recall adk memory context")
	require.ErrorContains(t, err, "memory unavailable")
}

func TestADKMemoryMiddlewareAssemblerUsesConfiguredProvider(t *testing.T) {
	provider := &recordingMemoryProvider{memories: []AgentMemory{{
		ID:      "1",
		Scope:   "thread",
		Content: "deployment region is APAC",
	}}}
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		MemoryProvider: provider,
	})

	bundle, err := assembler.Build(context.Background(), ADKMiddlewareBuildInput{
		Run: &RunSummary{
			RunID:    20,
			ThreadID: 10,
			Config: `{
				"context_budget":{
					"memory_tokens":100
				}
			}`,
		},
		Model: &recordingChatModel{resp: schema.AssistantMessage("done", nil)},
	})
	require.NoError(t, err)

	runCtx := &adk.ChatModelAgentContext{Instruction: "base"}
	_, got, err := bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareMemory)].
		BeforeAgent(context.Background(), runCtx)

	require.NoError(t, err)
	require.Contains(t, got.Instruction, "deployment region is APAC")
	require.Equal(t, 1, provider.calls)
}

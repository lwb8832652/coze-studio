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
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKMemoryMiddlewareInjectsDeerFlowReminderBeforeFirstUser(t *testing.T) {
	provider := &recordingMemoryProvider{memories: []AgentMemory{
		{
			ID:         "1",
			Scope:      "long_term",
			Content:    "preferred fact",
			Metadata:   `{"user_id":99,"path":"/private/secret"}`,
			Score:      0.9,
			Confidence: 0.9,
		},
		{
			ID:         "2",
			Scope:      "thread",
			Content:    " preferred fact ",
			Score:      0.8,
			Confidence: 0.8,
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
	middleware.now = func() time.Time {
		return time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	}
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Same(t, run, provider.run)
	require.Len(t, got.Messages, 2)
	reminder := got.Messages[0]
	require.Equal(t, schema.User, reminder.Role)
	require.Equal(t, true, reminder.Extra["hide_from_ui"])
	require.Equal(t, true, reminder.Extra["dynamic_context_reminder"])
	require.Contains(t, reminder.Content, "<system-reminder>")
	require.Contains(t, reminder.Content, "<memory>")
	require.Contains(t, reminder.Content, "Facts:")
	require.Equal(t, 1, strings.Count(reminder.Content, "preferred fact"))
	require.Contains(t, reminder.Content, "- [long_term | 0.90] preferred fact")
	require.Contains(t, reminder.Content, "<current_date>2026-07-01, Wednesday</current_date>")
	require.NotContains(t, reminder.Content, strings.Repeat("x", 400))
	require.NotContains(t, reminder.Content, "user_id")
	require.NotContains(t, reminder.Content, "/private/secret")
	require.NotContains(t, reminder.Content, `"id"`)
	require.Equal(t, "hello", got.Messages[1].Content)
}

func TestADKMemoryMiddlewareSkipsDuplicateReminderForSameDate(t *testing.T) {
	provider := &recordingMemoryProvider{memories: []AgentMemory{{
		ID:      "1",
		Scope:   "thread",
		Content: "deployment region is APAC",
	}}}
	middleware, err := NewADKMemoryMiddleware(
		&RunSummary{RunID: 20, ThreadID: 10},
		provider,
		ADKContextBudget{
			ContextWindowTokens:   128000,
			SummarizationTokens:   120000,
			SummarizationMessages: 200,
			MemoryTokens:          100,
		},
	)
	require.NoError(t, err)
	middleware.now = func() time.Time {
		return time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	}
	reminder := schema.UserMessage("<system-reminder>\n<current_date>2026-07-01, Wednesday</current_date>\n</system-reminder>")
	reminder.Extra = map[string]any{
		"hide_from_ui":             true,
		"dynamic_context_reminder": true,
	}
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{reminder, schema.UserMessage("hello again")},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Equal(t, 0, provider.calls)
	require.Len(t, got.Messages, 2)
	require.Same(t, reminder, got.Messages[0])
	require.Equal(t, "hello again", got.Messages[1].Content)
}

func TestADKMemoryMiddlewareInjectsDateOnlyReminderWhenDateChanged(t *testing.T) {
	provider := &recordingMemoryProvider{memories: []AgentMemory{{
		ID:      "1",
		Scope:   "thread",
		Content: "deployment region is APAC",
	}}}
	middleware, err := NewADKMemoryMiddleware(
		&RunSummary{RunID: 20, ThreadID: 10},
		provider,
		ADKContextBudget{
			ContextWindowTokens:   128000,
			SummarizationTokens:   120000,
			SummarizationMessages: 200,
			MemoryTokens:          100,
		},
	)
	require.NoError(t, err)
	middleware.now = func() time.Time {
		return time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC)
	}
	oldReminder := schema.UserMessage("<system-reminder>\n<current_date>2026-07-01, Wednesday</current_date>\n</system-reminder>")
	oldReminder.Extra = map[string]any{
		"hide_from_ui":             true,
		"dynamic_context_reminder": true,
	}
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{oldReminder, schema.UserMessage("new day question")},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Equal(t, 0, provider.calls)
	require.Len(t, got.Messages, 3)
	require.Same(t, oldReminder, got.Messages[0])
	dateReminder := got.Messages[1]
	require.Equal(t, true, dateReminder.Extra["hide_from_ui"])
	require.Equal(t, true, dateReminder.Extra["dynamic_context_reminder"])
	require.Contains(t, dateReminder.Content, "<current_date>2026-07-02, Thursday</current_date>")
	require.NotContains(t, dateReminder.Content, "<memory>")
	require.Equal(t, "new day question", got.Messages[2].Content)
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

	_, _, err = middleware.BeforeModelRewriteState(
		context.Background(),
		&adk.ChatModelAgentState{
			Messages: []*schema.Message{schema.UserMessage("hello")},
		},
		&adk.ModelContext{},
	)

	require.ErrorContains(t, err, "recall adk memory context")
	require.ErrorContains(t, err, "memory unavailable")
}

func TestADKMemoryMiddlewareSkipsInjectionWhenRecallTimesOut(t *testing.T) {
	provider := &timeoutMemoryProvider{}
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
	middleware.timeout = time.Nanosecond
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Len(t, got.Messages, 1)
	require.Equal(t, "hello", got.Messages[0].Content)
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

	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	}
	_, got, err := bundle.Handlers[adkMiddlewareIndex(ADKMiddlewareMemory)].
		BeforeModelRewriteState(context.Background(), state, &adk.ModelContext{})

	require.NoError(t, err)
	require.Len(t, got.Messages, 2)
	require.Contains(t, got.Messages[0].Content, "deployment region is APAC")
	require.Equal(t, 1, provider.calls)
}

type timeoutMemoryProvider struct {
	calls int
}

func (p *timeoutMemoryProvider) Recall(ctx context.Context, run *RunSummary) ([]AgentMemory, error) {
	p.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

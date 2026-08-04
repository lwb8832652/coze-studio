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
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

const (
	adkPlanCompletionGuardToolName     = "coze_plan_completion_guard"
	adkPlanCompletionGuardToolCallID   = "coze-plan-completion-guard"
	adkPlanCompletionGuardMaxReminders = 2
	adkPlanCompletionGuardSchema       = "coze.plan_completion_guard.v1"
)

type adkPlanCompletionGuardArgs struct {
	Reminder string `json:"reminder"`
}

type adkPlanCompletionGuardMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	backend      plantask.Backend
	maxReminders int

	mu        sync.Mutex
	reminders int
}

func newADKPlanCompletionGuardMiddleware(
	backend plantask.Backend,
) (*adkPlanCompletionGuardMiddleware, error) {
	if backend == nil {
		return nil, fmt.Errorf("eino adk plan backend is required")
	}
	return &adkPlanCompletionGuardMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		backend:                      backend,
		maxReminders:                 adkPlanCompletionGuardMaxReminders,
	}, nil
}

func (m *adkPlanCompletionGuardMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx == nil {
		runCtx = &adk.ChatModelAgentContext{}
	}
	controlTool, err := m.controlTool()
	if err != nil {
		return ctx, nil, err
	}
	next := *runCtx
	next.Tools = append(next.Tools, controlTool)
	return ctx, &next, nil
}

func (m *adkPlanCompletionGuardMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil {
		return ctx, state, nil
	}
	state.ToolInfos = filterADKPlanCompletionGuardToolInfos(state.ToolInfos)
	if state.DeferredToolInfos != nil {
		state.DeferredToolInfos = filterADKPlanCompletionGuardToolInfos(
			state.DeferredToolInfos,
		)
	}
	return ctx, state, nil
}

func (m *adkPlanCompletionGuardMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	replacement, err := m.rewritePrematureFinal(ctx, state.Messages[len(state.Messages)-1])
	if err != nil {
		return ctx, nil, err
	}
	state.Messages[len(state.Messages)-1] = replacement
	return ctx, state, nil
}

func (m *adkPlanCompletionGuardMiddleware) WrapModel(
	ctx context.Context,
	base model.BaseModel[*schema.Message],
	mc *adk.ModelContext,
) (model.BaseModel[*schema.Message], error) {
	return &adkPlanCompletionGuardModel{
		inner: base,
		guard: m,
	}, nil
}

func (m *adkPlanCompletionGuardMiddleware) rewritePrematureFinal(
	ctx context.Context,
	message *schema.Message,
) (*schema.Message, error) {
	if !isADKPlanCompletionGuardCandidate(message) {
		return message, nil
	}
	tasks, err := m.incompleteTasks(ctx)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return message, nil
	}
	if !m.reserveReminder() {
		return message, nil
	}
	reminder := formatADKPlanCompletionReminder(tasks)
	arguments, err := json.Marshal(adkPlanCompletionGuardArgs{
		Reminder: reminder,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal plan completion reminder: %w", err)
	}
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:   fmt.Sprintf("%s-%d", adkPlanCompletionGuardToolCallID, m.count()),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      adkPlanCompletionGuardToolName,
				Arguments: string(arguments),
			},
		}},
		Extra: map[string]any{
			"hide_from_ui": true,
			"schema":       adkPlanCompletionGuardSchema,
		},
	}, nil
}

func (m *adkPlanCompletionGuardMiddleware) AfterAgent(
	ctx context.Context,
	state *adk.ChatModelAgentState,
) (context.Context, error) {
	m.mu.Lock()
	m.reminders = 0
	m.mu.Unlock()
	return ctx, nil
}

func (m *adkPlanCompletionGuardMiddleware) controlTool() (tool.InvokableTool, error) {
	return toolutils.InferTool(
		adkPlanCompletionGuardToolName,
		"Internal control tool used by Coze to continue an agent run until active plan tasks are completed. Do not call directly.",
		func(_ context.Context, input adkPlanCompletionGuardArgs) (string, error) {
			if strings.TrimSpace(input.Reminder) == "" {
				return "", fmt.Errorf("plan completion reminder is required")
			}
			return input.Reminder, nil
		},
	)
}

func (m *adkPlanCompletionGuardMiddleware) incompleteTasks(
	ctx context.Context,
) ([]ADKPlanTask, error) {
	files, err := m.backend.LsInfo(ctx, &plantask.LsInfoRequest{
		Path: adkPlanBaseDir,
	})
	if err != nil {
		return nil, fmt.Errorf("list eino adk plan tasks: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	tasks := make([]ADKPlanTask, 0, len(files))
	for _, file := range files {
		if file.Path == path.Join(adkPlanBaseDir, adkPlanHighWatermark) {
			continue
		}
		content, err := m.backend.Read(ctx, &plantask.ReadRequest{
			FilePath: file.Path,
		})
		if err != nil {
			return nil, fmt.Errorf("read eino adk plan task %s: %w", file.Path, err)
		}
		if content == nil {
			return nil, fmt.Errorf("read eino adk plan task %s returned empty content", file.Path)
		}
		var task ADKPlanTask
		if err := json.Unmarshal([]byte(content.Content), &task); err != nil {
			return nil, fmt.Errorf("parse eino adk plan task %s: %w", file.Path, err)
		}
		switch task.Status {
		case "completed", "deleted":
			continue
		default:
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *adkPlanCompletionGuardMiddleware) reserveReminder() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reminders >= m.maxReminders {
		return false
	}
	m.reminders++
	return true
}

func (m *adkPlanCompletionGuardMiddleware) hasReminderCapacity() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reminders < m.maxReminders
}

func (m *adkPlanCompletionGuardMiddleware) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reminders
}

func isADKPlanCompletionGuardCandidate(message *schema.Message) bool {
	if message == nil || message.Role != schema.Assistant ||
		len(message.ToolCalls) > 0 {
		return false
	}
	if message.ResponseMeta != nil {
		switch message.ResponseMeta.FinishReason {
		case "tool_calls", "function_call":
			return false
		}
	}
	return true
}

func formatADKPlanCompletionReminder(tasks []ADKPlanTask) string {
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		status := strings.TrimSpace(task.Status)
		if status == "" {
			status = "pending"
		}
		subject := strings.TrimSpace(task.Subject)
		if subject == "" {
			subject = strings.TrimSpace(task.ID)
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s", status, subject))
	}
	return "<system_reminder>\n" +
		"You have incomplete todo items that must be finished before giving your final response:\n\n" +
		strings.Join(lines, "\n") +
		"\n\nPlease continue working on these tasks. Use the plan task tools to mark items as completed as you finish them, and only respond when all items are done.\n" +
		"</system_reminder>"
}

func filterADKPlanCompletionGuardToolInfos(
	infos []*schema.ToolInfo,
) []*schema.ToolInfo {
	if len(infos) == 0 {
		return infos
	}
	filtered := infos[:0]
	for _, info := range infos {
		if info != nil && info.Name == adkPlanCompletionGuardToolName {
			continue
		}
		filtered = append(filtered, info)
	}
	return filtered
}

type adkPlanTaskWithCompletionGuardMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	plan  adk.ChatModelAgentMiddleware
	guard *adkPlanCompletionGuardMiddleware
}

func newADKPlanTaskWithCompletionGuardMiddleware(
	plan adk.ChatModelAgentMiddleware,
	guard *adkPlanCompletionGuardMiddleware,
) (*adkPlanTaskWithCompletionGuardMiddleware, error) {
	if plan == nil {
		return nil, fmt.Errorf("eino adk plan task middleware is required")
	}
	if guard == nil {
		return nil, fmt.Errorf("eino adk plan completion guard middleware is required")
	}
	return &adkPlanTaskWithCompletionGuardMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		plan:                         plan,
		guard:                        guard,
	}, nil
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	var err error
	ctx, runCtx, err = m.plan.BeforeAgent(ctx, runCtx)
	if err != nil {
		return ctx, runCtx, err
	}
	return m.guard.BeforeAgent(ctx, runCtx)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) AfterAgent(
	ctx context.Context,
	state *adk.ChatModelAgentState,
) (context.Context, error) {
	var err error
	ctx, err = m.plan.AfterAgent(ctx, state)
	if err != nil {
		return ctx, err
	}
	return m.guard.AfterAgent(ctx, state)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	var err error
	ctx, state, err = m.plan.BeforeModelRewriteState(ctx, state, mc)
	if err != nil {
		return ctx, state, err
	}
	return m.guard.BeforeModelRewriteState(ctx, state, mc)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	var err error
	ctx, state, err = m.plan.AfterModelRewriteState(ctx, state, mc)
	if err != nil {
		return ctx, state, err
	}
	return m.guard.AfterModelRewriteState(ctx, state, mc)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) WrapInvokableToolCall(
	ctx context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return m.plan.WrapInvokableToolCall(ctx, endpoint, tCtx)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) WrapStreamableToolCall(
	ctx context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return m.plan.WrapStreamableToolCall(ctx, endpoint, tCtx)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) WrapEnhancedInvokableToolCall(
	ctx context.Context,
	endpoint adk.EnhancedInvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.EnhancedInvokableToolCallEndpoint, error) {
	return m.plan.WrapEnhancedInvokableToolCall(ctx, endpoint, tCtx)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) WrapEnhancedStreamableToolCall(
	ctx context.Context,
	endpoint adk.EnhancedStreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.EnhancedStreamableToolCallEndpoint, error) {
	return m.plan.WrapEnhancedStreamableToolCall(ctx, endpoint, tCtx)
}

func (m *adkPlanTaskWithCompletionGuardMiddleware) WrapModel(
	ctx context.Context,
	base model.BaseModel[*schema.Message],
	mc *adk.ModelContext,
) (model.BaseModel[*schema.Message], error) {
	wrapped, err := m.plan.WrapModel(ctx, base, mc)
	if err != nil {
		return nil, err
	}
	return m.guard.WrapModel(ctx, wrapped, mc)
}

type adkPlanCompletionGuardModel struct {
	inner model.BaseModel[*schema.Message]
	guard *adkPlanCompletionGuardMiddleware
}

func (m *adkPlanCompletionGuardModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.Message, error) {
	result, err := m.inner.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return m.guard.rewritePrematureFinal(ctx, result)
}

func (m *adkPlanCompletionGuardModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	shouldBuffer, err := m.shouldBufferStream(ctx)
	if err != nil {
		return nil, err
	}
	stream, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	if !shouldBuffer {
		return stream, nil
	}
	result, err := schema.ConcatMessageStream(stream)
	if err != nil {
		return nil, err
	}
	result, err = m.guard.rewritePrematureFinal(ctx, result)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{result}), nil
}

func (m *adkPlanCompletionGuardModel) shouldBufferStream(
	ctx context.Context,
) (bool, error) {
	if !m.guard.hasReminderCapacity() {
		return false, nil
	}
	tasks, err := m.guard.incompleteTasks(ctx)
	if err != nil {
		return false, err
	}
	return len(tasks) > 0, nil
}

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
	adkPlanExecutionGuardSchema        = "coze.plan_execution_guard.v1"
)

type adkPlanCompletionGuardArgs struct {
	Reminder string `json:"reminder"`
}

type adkPlanCompletionGuardMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	backend      plantask.Backend
	maxReminders int

	mu                 sync.Mutex
	reminders          int
	executionReminders int
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
	replacement, err := m.rewriteModelOutput(ctx, state.Messages[len(state.Messages)-1])
	if err != nil {
		return ctx, nil, err
	}
	state.Messages[len(state.Messages)-1] = replacement
	return ctx, state, nil
}

func (m *adkPlanCompletionGuardMiddleware) rewriteModelOutput(
	ctx context.Context,
	message *schema.Message,
) (*schema.Message, error) {
	replacement, err := m.rewriteExecutionWithoutActivePlan(ctx, message)
	if err != nil || replacement != message {
		return replacement, err
	}
	return m.rewritePrematureFinal(ctx, message)
}

func (m *adkPlanCompletionGuardMiddleware) rewriteExecutionWithoutActivePlan(
	ctx context.Context,
	message *schema.Message,
) (*schema.Message, error) {
	if !isADKPlanExecutionGuardCandidate(message) {
		return message, nil
	}
	mixedTransition := hasADKPlanTransitionAndExecution(message)
	tasks, err := m.incompleteTasks(ctx)
	if err != nil {
		return nil, err
	}
	activeCount := 0
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Status), "in_progress") {
			activeCount++
		}
	}
	if activeCount == 1 && !mixedTransition {
		m.resetExecutionReminders()
		return message, nil
	}
	reminderNumber, ok := m.reserveExecutionReminder()
	if !ok {
		return nil, fmt.Errorf(
			"agent attempted execution without exactly one active plan task after %d reminders",
			adkPlanCompletionGuardMaxReminders,
		)
	}
	reminder := formatADKPlanExecutionReminder(len(tasks), activeCount)
	if mixedTransition {
		reminder = formatADKPlanTransitionReminder()
	}
	arguments, err := json.Marshal(adkPlanCompletionGuardArgs{Reminder: reminder})
	if err != nil {
		return nil, fmt.Errorf("marshal plan execution reminder: %w", err)
	}
	return newADKPlanGuardControlMessage(message, schema.ToolCall{
		ID: fmt.Sprintf(
			"%s-execution-%d",
			adkPlanCompletionGuardToolCallID,
			reminderNumber,
		),
		Type: "function",
		Function: schema.FunctionCall{
			Name:      adkPlanCompletionGuardToolName,
			Arguments: string(arguments),
		},
	}, adkPlanExecutionGuardSchema), nil
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
	return newADKPlanGuardControlMessage(message, schema.ToolCall{
		ID:   fmt.Sprintf("%s-%d", adkPlanCompletionGuardToolCallID, m.count()),
		Type: "function",
		Function: schema.FunctionCall{
			Name:      adkPlanCompletionGuardToolName,
			Arguments: string(arguments),
		},
	}, adkPlanCompletionGuardSchema), nil
}

func newADKPlanGuardControlMessage(
	source *schema.Message,
	toolCall schema.ToolCall,
	schemaName string,
) *schema.Message {
	replacement := &schema.Message{Role: schema.Assistant}
	if source != nil {
		*replacement = *source
	}
	replacement.Role = schema.Assistant
	replacement.Content = ""
	replacement.MultiContent = nil
	replacement.UserInputMultiContent = nil
	replacement.AssistantGenMultiContent = adkPlanGuardReasoningParts(
		replacement.AssistantGenMultiContent,
	)
	replacement.ToolCalls = []schema.ToolCall{toolCall}
	replacement.ToolCallID = ""
	replacement.ToolName = ""
	replacement.Extra = make(map[string]any, len(replacement.Extra)+2)
	if source != nil {
		for key, value := range source.Extra {
			replacement.Extra[key] = value
		}
	}
	replacement.Extra["hide_from_ui"] = true
	replacement.Extra["schema"] = schemaName
	return replacement
}

func adkPlanGuardReasoningParts(
	parts []schema.MessageOutputPart,
) []schema.MessageOutputPart {
	if len(parts) == 0 {
		return nil
	}
	reasoning := make([]schema.MessageOutputPart, 0, len(parts))
	for _, part := range parts {
		if part.Type == schema.ChatMessagePartTypeReasoning {
			reasoning = append(reasoning, part)
		}
	}
	return reasoning
}

func (m *adkPlanCompletionGuardMiddleware) AfterAgent(
	ctx context.Context,
	state *adk.ChatModelAgentState,
) (context.Context, error) {
	m.mu.Lock()
	m.reminders = 0
	m.executionReminders = 0
	m.mu.Unlock()
	return ctx, nil
}

func (m *adkPlanCompletionGuardMiddleware) controlTool() (tool.InvokableTool, error) {
	return toolutils.InferTool(
		adkPlanCompletionGuardToolName,
		"Internal control tool used by Coze to enforce the plan lifecycle. Do not call directly.",
		func(_ context.Context, input adkPlanCompletionGuardArgs) (string, error) {
			if strings.TrimSpace(input.Reminder) == "" {
				return "", fmt.Errorf("plan completion reminder is required")
			}
			return input.Reminder, nil
		},
	)
}

func (m *adkPlanCompletionGuardMiddleware) reserveExecutionReminder() (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.executionReminders >= adkPlanCompletionGuardMaxReminders {
		return 0, false
	}
	m.executionReminders++
	return m.executionReminders, true
}

func (m *adkPlanCompletionGuardMiddleware) resetExecutionReminders() {
	m.mu.Lock()
	m.executionReminders = 0
	m.mu.Unlock()
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

func formatADKPlanExecutionReminder(taskCount, activeCount int) string {
	if taskCount == 0 {
		return "<system_reminder>\n" +
			"This run is in plan mode. Before using an execution tool, create concise major steps with TaskCreate, then set exactly one current step to in_progress with TaskUpdate. Direct answers without execution tools do not need a plan.\n" +
			"</system_reminder>"
	}
	return fmt.Sprintf(
		"<system_reminder>\nThis run has %d incomplete plan tasks and %d active tasks. Before using an execution tool, set exactly one current major step to in_progress with TaskUpdate. Child operations must run under that active step.\n</system_reminder>",
		taskCount,
		activeCount,
	)
}

func formatADKPlanTransitionReminder() string {
	return "<system_reminder>\n" +
		"Do not combine a plan status change and child execution operations in the same tool-call batch. First use TaskCreate or TaskUpdate to establish exactly one in_progress major step. In the next model turn, run only the child operations for that active step.\n" +
		"</system_reminder>"
}

func isADKPlanExecutionGuardCandidate(message *schema.Message) bool {
	if message == nil || message.Role != schema.Assistant || len(message.ToolCalls) == 0 {
		return false
	}
	for _, call := range message.ToolCalls {
		if !isADKPlanExecutionExemptTool(call.Function.Name) {
			return true
		}
	}
	return false
}

func hasADKPlanTransitionAndExecution(message *schema.Message) bool {
	if message == nil {
		return false
	}
	hasTransition := false
	hasExecution := false
	for _, call := range message.ToolCalls {
		name := call.Function.Name
		if isADKPlanMutationTool(name) {
			hasTransition = true
		} else if !isADKPlanExecutionExemptTool(name) {
			hasExecution = true
		}
	}
	return hasTransition && hasExecution
}

func isADKPlanMutationTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case strings.ToLower(plantask.TaskCreateToolName),
		strings.ToLower(plantask.TaskUpdateToolName):
		return true
	default:
		return false
	}
}

func isADKPlanExecutionExemptTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case strings.ToLower(plantask.TaskCreateToolName),
		strings.ToLower(plantask.TaskGetToolName),
		strings.ToLower(plantask.TaskUpdateToolName),
		strings.ToLower(plantask.TaskListToolName),
		strings.ToLower(adkPlanCompletionGuardToolName),
		strings.ToLower(adkClarificationToolName),
		strings.ToLower(adkDeerFlowClarificationToolName),
		strings.ToLower(adkConfirmationToolName),
		strings.ToLower(adkPresentFilesToolName):
		return true
	default:
		return false
	}
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
	return m.guard.rewriteModelOutput(ctx, result)
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
	result, err = m.guard.rewriteModelOutput(ctx, result)
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

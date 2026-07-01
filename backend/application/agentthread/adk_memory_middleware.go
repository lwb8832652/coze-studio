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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
)

const (
	adkSystemReminderStart = "<system-reminder>"
	adkSystemReminderEnd   = "</system-reminder>"
	adkMemoryContextStart  = "<memory>"
	adkMemoryContextEnd    = "</memory>"

	adkHideFromUIExtraKey                = "hide_from_ui"
	adkDynamicContextReminderExtraKey    = "dynamic_context_reminder"
	adkAgentsMDContentExtraKey           = "__agentsmd_content__"
	adkDynamicContextReminderDatePattern = `<current_date>([^<]+)</current_date>`
)

var adkDynamicContextReminderDateRE = regexp.MustCompile(adkDynamicContextReminderDatePattern)

type ADKMemoryMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run      *RunSummary
	provider MemoryProvider
	budget   ADKContextBudget
	now      func() time.Time
	timeout  time.Duration
}

func NewADKMemoryMiddleware(
	run *RunSummary,
	provider MemoryProvider,
	budget ADKContextBudget,
) (*ADKMemoryMiddleware, error) {
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if provider == nil {
		return nil, fmt.Errorf("memory provider is required")
	}
	if err := validateADKMemoryBudget(budget); err != nil {
		return nil, err
	}
	return &ADKMemoryMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		run:                          run,
		provider:                     provider,
		budget:                       budget,
		now:                          time.Now,
		timeout:                      5 * time.Second,
	}, nil
}

func (m *ADKMemoryMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx == nil {
		runCtx = &adk.ChatModelAgentContext{}
	}
	return ctx, runCtx, nil
}

func (m *ADKMemoryMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil {
		state = &adk.ChatModelAgentState{}
	}
	if len(state.Messages) == 0 {
		return ctx, state, nil
	}

	currentDate := m.currentDate()
	lastDate := lastADKDynamicContextReminderDate(state.Messages)
	if lastDate == currentDate {
		return ctx, state, nil
	}
	if lastDate != "" {
		targetIndex := lastADKUserInjectionTargetIndex(state.Messages)
		if targetIndex < 0 {
			return ctx, state, nil
		}
		nState := *state
		nState.Messages = insertADKMessage(
			state.Messages,
			targetIndex,
			newADKDynamicContextReminder(buildADKDateUpdateReminder(currentDate)),
		)
		return ctx, &nState, nil
	}

	targetIndex := firstADKUserInjectionTargetIndex(state.Messages)
	if targetIndex < 0 {
		return ctx, state, nil
	}

	memories, err := m.recallMemories(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return ctx, state, nil
		}
		return ctx, state, fmt.Errorf("recall adk memory context: %w", err)
	}
	block, err := buildADKMemoryContext(memories, m.budget.MemoryTokens)
	if err != nil {
		return ctx, state, err
	}
	nState := *state
	nState.Messages = insertADKMessage(
		state.Messages,
		targetIndex,
		newADKDynamicContextReminder(buildADKFullReminder(block, currentDate)),
	)
	return ctx, &nState, nil
}

func buildADKMemoryContext(memories []AgentMemory, tokenBudget int) (string, error) {
	normalized := normalizeMemoryContext(memories)
	if tokenBudget <= 0 || len(normalized.Items) == 0 {
		return "", nil
	}

	usedTokens := estimateADKTextTokens(adkMemoryContextStart) +
		estimateADKTextTokens(adkMemoryContextEnd) +
		estimateADKTextTokens("Facts:")
	lines := make([]string, 0, len(normalized.Items))
	seen := make(map[string]struct{}, len(normalized.Items))
	items := append([]AgentMemory(nil), normalized.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		return adkMemoryConfidence(items[i]) > adkMemoryConfidence(items[j])
	})
	for _, memory := range items {
		content := strings.TrimSpace(memory.Content)
		key := strings.ToLower(strings.Join(strings.Fields(content), " "))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		category := strings.TrimSpace(memory.Scope)
		if category == "" {
			category = "context"
		}
		line := fmt.Sprintf("- [%s | %.2f] %s", category, adkMemoryConfidence(memory), content)
		lineTokens := estimateADKTextTokens(line)
		if usedTokens+lineTokens > tokenBudget {
			continue
		}
		lines = append(lines, line)
		usedTokens += lineTokens
	}
	if len(lines) == 0 {
		return "", nil
	}

	return adkMemoryContextStart + "\n" +
		"Facts:\n" +
		strings.Join(lines, "\n") + "\n" +
		adkMemoryContextEnd, nil
}

func (m *ADKMemoryMiddleware) currentDate() string {
	now := time.Now
	if m != nil && m.now != nil {
		now = m.now
	}
	return now().Format("2006-01-02, Monday")
}

func (m *ADKMemoryMiddleware) recallMemories(ctx context.Context) ([]AgentMemory, error) {
	timeout := 5 * time.Second
	if m != nil && m.timeout > 0 {
		timeout = m.timeout
	}
	recallCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return m.provider.Recall(recallCtx, m.run)
}

func buildADKFullReminder(memoryContext string, currentDate string) string {
	lines := []string{adkSystemReminderStart}
	if memoryContext = strings.TrimSpace(memoryContext); memoryContext != "" {
		lines = append(lines, memoryContext, "")
	}
	lines = append(
		lines,
		fmt.Sprintf("<current_date>%s</current_date>", currentDate),
		adkSystemReminderEnd,
	)
	return strings.Join(lines, "\n")
}

func buildADKDateUpdateReminder(currentDate string) string {
	return strings.Join([]string{
		adkSystemReminderStart,
		fmt.Sprintf("<current_date>%s</current_date>", currentDate),
		adkSystemReminderEnd,
	}, "\n")
}

func newADKDynamicContextReminder(content string) *schema.Message {
	message := schema.UserMessage(content)
	message.Extra = map[string]any{
		adkHideFromUIExtraKey:             true,
		adkDynamicContextReminderExtraKey: true,
	}
	return message
}

func lastADKDynamicContextReminderDate(messages []*schema.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]
		if !isADKDynamicContextReminder(message) {
			continue
		}
		match := adkDynamicContextReminderDateRE.FindStringSubmatch(message.Content)
		if len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

func firstADKUserInjectionTargetIndex(messages []*schema.Message) int {
	for i, message := range messages {
		if isADKUserInjectionTarget(message) {
			return i
		}
	}
	return -1
}

func lastADKUserInjectionTargetIndex(messages []*schema.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if isADKUserInjectionTarget(messages[i]) {
			return i
		}
	}
	return -1
}

func isADKUserInjectionTarget(message *schema.Message) bool {
	if message == nil || message.Role != schema.User || message.Name == "summary" {
		return false
	}
	if isADKDynamicContextReminder(message) {
		return false
	}
	if message.Extra != nil {
		if hidden, _ := message.Extra[adkHideFromUIExtraKey].(bool); hidden {
			return false
		}
		if agentsMD, _ := message.Extra[adkAgentsMDContentExtraKey].(bool); agentsMD {
			return false
		}
	}
	return true
}

func isADKDynamicContextReminder(message *schema.Message) bool {
	if message == nil || message.Role != schema.User || message.Extra == nil {
		return false
	}
	dynamic, _ := message.Extra[adkDynamicContextReminderExtraKey].(bool)
	return dynamic
}

func insertADKMessage(messages []*schema.Message, index int, message *schema.Message) []*schema.Message {
	if index < 0 || index > len(messages) {
		index = len(messages)
	}
	next := make([]*schema.Message, 0, len(messages)+1)
	next = append(next, messages[:index]...)
	next = append(next, message)
	next = append(next, messages[index:]...)
	return next
}

func adkMemoryConfidence(memory AgentMemory) float64 {
	if memory.Confidence > 0 {
		return memory.Confidence
	}
	if memory.Score > 0 {
		return memory.Score
	}
	return 0
}

func finalizeADKSummarizationWithDynamicContextReminders(
	ctx context.Context,
	originalMessages []*schema.Message,
	summary *schema.Message,
) ([]*schema.Message, error) {
	filteredMessages, reminders := filterADKDynamicContextReminders(originalMessages)
	finalMessages, err := summarization.DefaultFinalize[*schema.Message](
		ctx,
		filteredMessages,
		summary,
	)
	if err != nil {
		return nil, err
	}
	if len(reminders) == 0 {
		return finalMessages, nil
	}

	insertAt := len(finalMessages)
	for i, message := range finalMessages {
		if message == nil || message.Role != schema.System {
			insertAt = i
			break
		}
	}
	result := make([]*schema.Message, 0, len(finalMessages)+len(reminders))
	result = append(result, finalMessages[:insertAt]...)
	result = append(result, reminders...)
	result = append(result, finalMessages[insertAt:]...)
	return result, nil
}

func filterADKDynamicContextReminders(
	messages []*schema.Message,
) ([]*schema.Message, []*schema.Message) {
	if len(messages) == 0 {
		return nil, nil
	}
	filtered := make([]*schema.Message, 0, len(messages))
	reminders := make([]*schema.Message, 0, 1)
	for _, message := range messages {
		if isADKDynamicContextReminder(message) {
			reminders = append(reminders, message)
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered, reminders
}

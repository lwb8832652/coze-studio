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
	"strings"

	"github.com/cloudwego/eino/adk"
)

const (
	adkMemoryContextStart = "<memory_context>"
	adkMemoryContextEnd   = "</memory_context>"
)

type ADKMemoryMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run      *RunSummary
	provider MemoryProvider
	budget   ADKContextBudget
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
	}, nil
}

func (m *ADKMemoryMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if runCtx == nil {
		runCtx = &adk.ChatModelAgentContext{}
	}
	if strings.Contains(runCtx.Instruction, adkMemoryContextStart) {
		return ctx, runCtx, nil
	}

	memories, err := m.provider.Recall(ctx, m.run)
	if err != nil {
		return ctx, runCtx, fmt.Errorf("recall adk memory context: %w", err)
	}
	block, err := buildADKMemoryContext(memories, m.budget.MemoryTokens)
	if err != nil {
		return ctx, runCtx, err
	}
	if block == "" {
		return ctx, runCtx, nil
	}

	instruction := strings.TrimSpace(runCtx.Instruction)
	if instruction == "" {
		runCtx.Instruction = block
	} else {
		runCtx.Instruction = instruction + "\n\n" + block
	}
	return ctx, runCtx, nil
}

func buildADKMemoryContext(memories []AgentMemory, tokenBudget int) (string, error) {
	normalized := normalizeMemoryContext(memories)
	if tokenBudget <= 0 || len(normalized.Items) == 0 {
		return "", nil
	}

	usedTokens := estimateADKTextTokens(adkMemoryContextStart) +
		estimateADKTextTokens(adkMemoryContextEnd)
	lines := make([]string, 0, len(normalized.Items))
	seen := make(map[string]struct{}, len(normalized.Items))
	for _, memory := range normalized.Items {
		content := strings.TrimSpace(memory.Content)
		key := strings.ToLower(strings.Join(strings.Fields(content), " "))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		raw, err := json.Marshal(struct {
			Scope   string `json:"scope,omitempty"`
			Content string `json:"content"`
		}{
			Scope:   strings.TrimSpace(memory.Scope),
			Content: content,
		})
		if err != nil {
			return "", fmt.Errorf("marshal adk memory context: %w", err)
		}
		line := "- " + string(raw)
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
		strings.Join(lines, "\n") + "\n" +
		adkMemoryContextEnd, nil
}

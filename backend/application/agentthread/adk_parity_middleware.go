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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const adkSummarizationContentTypeExtraKey = "_eino_summarization_content_type"

type ADKParityStateMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	tracker      *ADKParityStateTracker
	runID        int64
	catalogHash  string
	dynamicNames map[string]struct{}
}

func NewADKParityStateMiddleware(
	ctx context.Context,
	dynamicTools []tool.BaseTool,
) (*ADKParityStateMiddleware, error) {
	tracker := adkParityStateTrackerFromContext(ctx)
	if tracker == nil {
		return nil, fmt.Errorf("eino adk parity state tracker is required")
	}
	catalogHash, names, err := adkParityToolCatalog(ctx, dynamicTools)
	if err != nil {
		return nil, err
	}
	return &ADKParityStateMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		tracker:                      tracker,
		runID:                        tracker.Snapshot().LastRunID,
		catalogHash:                  catalogHash,
		dynamicNames:                 names,
	}, nil
}

func (m *ADKParityStateMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || m.tracker == nil || state == nil {
		return ctx, state, nil
	}
	before := m.tracker.Snapshot()
	messages := make([]ADKParityMessage, 0, len(state.Messages))
	var summary *ADKParitySummaryBoundary
	for _, message := range state.Messages {
		if message == nil {
			continue
		}
		if adkParitySummaryMessage(message) {
			digest := sha256.Sum256([]byte(message.Content))
			summary = &ADKParitySummaryBoundary{
				Digest:               hex.EncodeToString(digest[:8]),
				OriginalMessageCount: len(before.Messages),
			}
			continue
		}
		if adkParityHiddenMessage(message) {
			continue
		}
		role := string(message.Role)
		if message.Role != schema.User && message.Role != schema.Assistant {
			continue
		}
		content := stripADKUploadedFilesContext(message.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		id := ""
		if message.Extra != nil {
			id, _ = message.Extra[einoMessageIDExtraKey].(string)
		}
		messages = append(messages, ADKParityMessage{
			ID: id, RunID: m.runID, Role: role, Content: content,
		})
	}
	if summary == nil {
		summary = before.Summary
	} else {
		if summary.OriginalMessageCount < len(messages) {
			summary.OriginalMessageCount = len(messages)
		}
		summary.ActiveMessageCount = len(messages)
	}
	if err := m.tracker.ReplaceMessages(messages, summary); err != nil {
		return ctx, state, fmt.Errorf("record eino adk parity messages: %w", err)
	}
	if m.catalogHash != "" {
		promoted := make([]string, 0, len(m.dynamicNames))
		seen := make(map[string]struct{}, len(m.dynamicNames))
		for _, info := range state.ToolInfos {
			if info == nil {
				continue
			}
			if _, ok := m.dynamicNames[info.Name]; !ok {
				continue
			}
			if _, ok := seen[info.Name]; ok {
				continue
			}
			seen[info.Name] = struct{}{}
			promoted = append(promoted, info.Name)
		}
		if err := m.tracker.MergePromotedTools(&ADKParityPromotedTools{
			CatalogHash: m.catalogHash,
			Names:       promoted,
		}); err != nil {
			return ctx, state, fmt.Errorf("record eino adk promoted tools: %w", err)
		}
	}
	return ctx, state, nil
}

func adkParityToolCatalog(
	ctx context.Context,
	tools []tool.BaseTool,
) (string, map[string]struct{}, error) {
	if len(tools) == 0 {
		return "", map[string]struct{}{}, nil
	}
	type catalogEntry struct {
		Name   string           `json:"name"`
		Schema *schema.ToolInfo `json:"schema"`
	}
	entries := make([]catalogEntry, 0, len(tools))
	names := make(map[string]struct{}, len(tools))
	for _, candidate := range tools {
		if candidate == nil {
			return "", nil, fmt.Errorf("eino adk parity dynamic tool is required")
		}
		info, err := candidate.Info(ctx)
		if err != nil {
			return "", nil, fmt.Errorf("load eino adk parity dynamic tool info: %w", err)
		}
		if info == nil || strings.TrimSpace(info.Name) == "" {
			return "", nil, fmt.Errorf("eino adk parity dynamic tool info is invalid")
		}
		if _, ok := names[info.Name]; ok {
			return "", nil, fmt.Errorf("duplicate eino adk parity dynamic tool: %s", info.Name)
		}
		names[info.Name] = struct{}{}
		entries = append(entries, catalogEntry{Name: info.Name, Schema: info})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	raw, err := json.Marshal(entries)
	if err != nil {
		return "", nil, fmt.Errorf("marshal eino adk parity tool catalog: %w", err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:8]), names, nil
}

func adkParitySummaryMessage(message *schema.Message) bool {
	if message == nil || message.Extra == nil {
		return false
	}
	value, _ := message.Extra[adkSummarizationContentTypeExtraKey].(string)
	return value == "summary"
}

func adkParityHiddenMessage(message *schema.Message) bool {
	if message == nil || message.Extra == nil {
		return false
	}
	for key, value := range message.Extra {
		if hidden, ok := value.(bool); ok && hidden {
			switch {
			case key == adkHideFromUIExtraKey,
				key == adkDynamicContextReminderExtraKey,
				key == adkAgentsMDContentExtraKey,
				strings.HasPrefix(key, "__toolsearch_"):
				return true
			}
		}
	}
	return false
}

func stripADKUploadedFilesContext(content string) string {
	for {
		start := strings.Index(content, adkUploadedFilesStart)
		if start < 0 {
			break
		}
		endOffset := strings.Index(content[start:], adkUploadedFilesEnd)
		if endOffset < 0 {
			break
		}
		end := start + endOffset + len(adkUploadedFilesEnd)
		content = content[:start] + content[end:]
	}
	return strings.TrimSpace(content)
}

var _ adk.ChatModelAgentMiddleware = (*ADKParityStateMiddleware)(nil)

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
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const defaultThreadMemoryProviderLimit int32 = 8

type threadMemoryRetrievalOptions struct {
	Limit          int32
	CandidateLimit int32
	Scopes         []MemoryScope
	MinConfidence  float64
	Query          string
}

type threadMemoryRetrievalConfig struct {
	Limit          *int32   `json:"limit"`
	CandidateLimit *int32   `json:"candidate_limit"`
	Scopes         []string `json:"scopes"`
	MinConfidence  *float64 `json:"min_confidence"`
	Query          string   `json:"query"`
}

type threadMemoryRetrievalRunConfig struct {
	MemoryRetrieval threadMemoryRetrievalConfig `json:"memory_retrieval"`
}

var validMemoryRetrievalScopes = map[MemoryScope]struct{}{
	MemoryScopeThread:   {},
	MemoryScopeRun:      {},
	MemoryScopeLongTerm: {},
}

type ThreadMemoryProvider struct {
	app   *ApplicationService
	limit int32
}

func NewThreadMemoryProvider(app *ApplicationService, limit int32) *ThreadMemoryProvider {
	if limit <= 0 {
		limit = defaultThreadMemoryProviderLimit
	}

	return &ThreadMemoryProvider{
		app:   app,
		limit: limit,
	}
}

func (p *ThreadMemoryProvider) Recall(ctx context.Context, run *RunSummary) ([]AgentMemory, error) {
	if p == nil || p.app == nil {
		return nil, fmt.Errorf("agent thread memory provider application service is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.ThreadID <= 0 {
		return nil, fmt.Errorf("run thread id is required")
	}
	options, err := threadMemoryRetrievalOptionsFromRun(run, p.limit)
	if err != nil {
		return nil, err
	}

	resp, err := p.app.RecallMemories(ctx, &RecallMemoriesRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Scopes:   options.Scopes,
		Limit:    options.CandidateLimit,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Memories) == 0 {
		return nil, nil
	}

	memories := make([]AgentMemory, 0, len(resp.Memories))
	for _, memory := range resp.Memories {
		if memory == nil {
			continue
		}
		memories = append(memories, AgentMemory{
			ID:                   strconv.FormatInt(memory.MemoryID, 10),
			Scope:                string(memory.Scope),
			Content:              memory.Content,
			Metadata:             memory.Metadata,
			Score:                memory.Score,
			Confidence:           memory.Confidence,
			SourceType:           memory.SourceType,
			SourceID:             memory.SourceID,
			CorrectionOfMemoryID: memory.CorrectionOfMemoryID,
			CorrectedAt:          memory.CorrectedAt,
		})
	}

	return applyThreadMemoryRetrievalOptions(memories, options), nil
}

func threadMemoryRetrievalOptionsFromRun(
	run *RunSummary,
	defaultLimit int32,
) (threadMemoryRetrievalOptions, error) {
	if defaultLimit <= 0 {
		defaultLimit = defaultThreadMemoryProviderLimit
	}
	options := threadMemoryRetrievalOptions{
		Limit:          defaultLimit,
		CandidateLimit: defaultLimit,
		Query:          threadMemoryRetrievalQueryFromRun(run),
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return options, nil
	}

	var payload threadMemoryRetrievalRunConfig
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return threadMemoryRetrievalOptions{}, fmt.Errorf("decode memory retrieval config: %w", err)
	}
	config := payload.MemoryRetrieval
	if config.Limit != nil && *config.Limit > 0 {
		options.Limit = clampThreadMemoryRetrievalLimit(*config.Limit)
	}
	if config.CandidateLimit != nil && *config.CandidateLimit > 0 {
		options.CandidateLimit = clampThreadMemoryRetrievalLimit(*config.CandidateLimit)
	}
	if options.CandidateLimit < options.Limit {
		options.CandidateLimit = options.Limit
	}
	if config.MinConfidence != nil {
		if *config.MinConfidence < 0 || *config.MinConfidence > 1 {
			return threadMemoryRetrievalOptions{}, fmt.Errorf(
				"memory retrieval min confidence must be between 0 and 1",
			)
		}
		options.MinConfidence = *config.MinConfidence
	}
	if query := strings.TrimSpace(config.Query); query != "" {
		options.Query = query
	}
	if len(config.Scopes) > 0 {
		scopes := make([]MemoryScope, 0, len(config.Scopes))
		seen := make(map[MemoryScope]struct{}, len(config.Scopes))
		for _, rawScope := range config.Scopes {
			scope := MemoryScope(strings.TrimSpace(rawScope))
			if scope == "" {
				continue
			}
			if _, ok := validMemoryRetrievalScopes[scope]; !ok {
				return threadMemoryRetrievalOptions{}, fmt.Errorf(
					"memory retrieval scope is invalid",
				)
			}
			if _, exists := seen[scope]; exists {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
		options.Scopes = scopes
	}
	return options, nil
}

func clampThreadMemoryRetrievalLimit(limit int32) int32 {
	if limit <= 0 {
		return defaultThreadMemoryProviderLimit
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func applyThreadMemoryRetrievalOptions(
	memories []AgentMemory,
	options threadMemoryRetrievalOptions,
) []AgentMemory {
	if len(memories) == 0 {
		return nil
	}
	correctedIDs := make(map[string]struct{})
	for _, memory := range memories {
		if memory.CorrectionOfMemoryID > 0 {
			correctedIDs[strconv.FormatInt(memory.CorrectionOfMemoryID, 10)] = struct{}{}
		}
	}

	filtered := make([]AgentMemory, 0, len(memories))
	for _, memory := range memories {
		if _, corrected := correctedIDs[strings.TrimSpace(memory.ID)]; corrected {
			continue
		}
		if options.MinConfidence > 0 && memory.Confidence > 0 &&
			memory.Confidence < options.MinConfidence {
			continue
		}
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		filtered = append(filtered, memory)
	}
	if len(filtered) == 0 {
		return nil
	}

	queryTerms := threadMemoryTerms(options.Query)
	if len(queryTerms) > 0 {
		sort.SliceStable(filtered, func(i, j int) bool {
			leftRelevance := threadMemoryRelevance(filtered[i], queryTerms)
			rightRelevance := threadMemoryRelevance(filtered[j], queryTerms)
			if leftRelevance != rightRelevance {
				return leftRelevance > rightRelevance
			}
			leftScore := threadMemoryWeightedScore(filtered[i])
			rightScore := threadMemoryWeightedScore(filtered[j])
			if leftScore != rightScore {
				return leftScore > rightScore
			}
			if filtered[i].CorrectedAt != filtered[j].CorrectedAt {
				return filtered[i].CorrectedAt > filtered[j].CorrectedAt
			}
			return filtered[i].ID > filtered[j].ID
		})
	}

	limit := options.Limit
	if limit <= 0 {
		limit = defaultThreadMemoryProviderLimit
	}
	if len(filtered) > int(limit) {
		filtered = filtered[:limit]
	}
	return filtered
}

func threadMemoryWeightedScore(memory AgentMemory) float64 {
	confidence := memory.Confidence
	if confidence <= 0 {
		confidence = 1
	}
	return memory.Score * confidence
}

func threadMemoryRelevance(memory AgentMemory, queryTerms map[string]struct{}) int {
	if len(queryTerms) == 0 {
		return 0
	}
	score := 0
	memoryTerms := threadMemoryTerms(memory.Content)
	for term := range memoryTerms {
		if _, ok := queryTerms[term]; ok {
			score++
		}
	}
	return score
}

func threadMemoryRetrievalQueryFromRun(run *RunSummary) string {
	if run == nil {
		return ""
	}
	input := strings.TrimSpace(run.Input)
	if input == "" {
		return ""
	}
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return input
	}
	for i := len(payload.Messages) - 1; i >= 0; i-- {
		message := payload.Messages[i]
		if strings.TrimSpace(message.Role) != "" &&
			strings.TrimSpace(message.Role) != string(MessageRoleUser) {
			continue
		}
		if text := threadMemoryContentText(message.Content); text != "" {
			return text
		}
	}
	return strings.TrimSpace(payload.Message)
}

func threadMemoryContentText(content any) string {
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			switch itemPayload := item.(type) {
			case string:
				if text := strings.TrimSpace(itemPayload); text != "" {
					parts = append(parts, text)
				}
			case map[string]any:
				if text := strings.TrimSpace(fmt.Sprint(itemPayload["text"])); text != "" &&
					text != "<nil>" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, " ")
	default:
		return strings.TrimSpace(fmt.Sprint(content))
	}
}

func threadMemoryTerms(text string) map[string]struct{} {
	terms := make(map[string]struct{})
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		term := strings.ToLower(builder.String())
		builder.Reset()
		if len([]rune(term)) <= 1 {
			return
		}
		terms[term] = struct{}{}
	}
	for _, current := range text {
		if unicode.IsLetter(current) || unicode.IsNumber(current) || current == '_' {
			builder.WriteRune(current)
			continue
		}
		flush()
	}
	flush()
	return terms
}

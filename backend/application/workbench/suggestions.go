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

package workbench

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const (
	defaultSuggestionCount = 3
	minSuggestionCount     = 1
	maxSuggestionCount     = 5
)

var (
	suggestionThinkBlockPattern = regexp.MustCompile(`(?is)<think\b[^>]*>.*?</think\s*>`)
	suggestionOpenThinkPattern  = regexp.MustCompile(`(?is)<think\b[^>]*>`)
)

type SuggestionMessage struct {
	Role    string
	Content string
}

type GenerateSuggestionsRequest struct {
	Messages  []SuggestionMessage
	N         int
	ModelName string
	ModelType int64
}

type GenerateSuggestionsResponse struct {
	Suggestions []string
}

func (s *ApplicationService) GenerateSuggestions(ctx context.Context, req *GenerateSuggestionsRequest) (*GenerateSuggestionsResponse, error) {
	if req == nil || len(req.Messages) == 0 {
		return &GenerateSuggestionsResponse{Suggestions: []string{}}, nil
	}

	n := normalizeSuggestionCount(req.N)
	conversation := formatSuggestionConversation(req.Messages)
	if conversation == "" {
		return &GenerateSuggestionsResponse{Suggestions: []string{}}, nil
	}

	var provider chatModelProvider
	if s != nil {
		provider = s.chatModelProvider
	}
	if provider == nil {
		provider = defaultChatModelProvider
	}

	cm, configured, err := provider(ctx, req.ModelType)
	if err != nil {
		return nil, err
	}
	if !configured || cm == nil {
		return nil, fmt.Errorf("workbench suggestion model is not configured")
	}

	resp, err := cm.Generate(ctx, suggestionPromptMessages(conversation, n))
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &GenerateSuggestionsResponse{Suggestions: []string{}}, nil
	}

	return &GenerateSuggestionsResponse{
		Suggestions: cleanSuggestionList(parseSuggestionJSONList(resp.Content), n),
	}, nil
}

func normalizeSuggestionCount(n int) int {
	if n <= 0 {
		return defaultSuggestionCount
	}
	if n < minSuggestionCount {
		return minSuggestionCount
	}
	if n > maxSuggestionCount {
		return maxSuggestionCount
	}
	return n
}

func formatSuggestionConversation(messages []SuggestionMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}

		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "user", "human":
			parts = append(parts, "User: "+content)
		case "assistant", "ai":
			parts = append(parts, "Assistant: "+content)
		default:
			role := strings.TrimSpace(message.Role)
			if role == "" {
				role = "Message"
			}
			parts = append(parts, role+": "+content)
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func suggestionPromptMessages(conversation string, n int) []*schema.Message {
	systemInstruction := fmt.Sprintf(
		"You are generating follow-up questions to help the user continue the conversation.\n"+
			"Based on the conversation below, produce EXACTLY %d short questions the user might ask next.\n"+
			"Requirements:\n"+
			"- Questions must be relevant to the preceding conversation.\n"+
			"- Questions must be written in the same language as the user.\n"+
			"- Keep each question concise (ideally <= 20 words / <= 40 Chinese characters).\n"+
			"- Do NOT include numbering, markdown, or any extra text.\n"+
			"- Output MUST be a JSON array of strings only.\n",
		n,
	)
	userContent := fmt.Sprintf("Conversation Context:\n%s\n\nGenerate %d follow-up questions", conversation, n)

	return []*schema.Message{
		schema.SystemMessage(systemInstruction),
		schema.UserMessage(userContent),
	}
}

func parseSuggestionJSONList(raw string) []string {
	candidate := stripSuggestionThinkBlocks(raw)
	candidate = stripSuggestionCodeFence(candidate)
	start := strings.Index(candidate, "[")
	end := strings.LastIndex(candidate, "]")
	if start < 0 || end <= start {
		return nil
	}

	var values []string
	if err := json.Unmarshal([]byte(candidate[start:end+1]), &values); err != nil {
		return nil
	}
	return values
}

func stripSuggestionThinkBlocks(raw string) string {
	text := suggestionThinkBlockPattern.ReplaceAllString(raw, "")
	if openMatch := suggestionOpenThinkPattern.FindStringIndex(text); openMatch != nil {
		text = text[:openMatch[0]]
	}
	return strings.TrimSpace(text)
}

func stripSuggestionCodeFence(raw string) string {
	text := strings.TrimSpace(raw)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	lines := strings.Split(text, "\n")
	if len(lines) >= 3 && strings.HasPrefix(lines[0], "```") && strings.HasPrefix(lines[len(lines)-1], "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return text
}

func cleanSuggestionList(values []string, n int) []string {
	out := make([]string, 0, n)
	seen := map[string]struct{}{}
	for _, value := range values {
		suggestion := strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
		if suggestion == "" {
			continue
		}
		if _, ok := seen[suggestion]; ok {
			continue
		}
		seen[suggestion] = struct{}{}
		out = append(out, suggestion)
		if len(out) >= n {
			break
		}
	}

	return out
}

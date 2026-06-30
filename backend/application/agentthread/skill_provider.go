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
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"

	skillentity "github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	skilldomain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

const (
	defaultRuntimeSkillLimit          = 32
	defaultRuntimeSkillTotalBodyBytes = 384 << 10
)

var (
	runtimeSlashSkillPattern = regexp.MustCompile(`^/([a-z0-9]+(?:-[a-z0-9]+)*)(?:\s+|$)`)

	reservedRuntimeSlashSkillNames = map[string]struct{}{
		"bootstrap": {},
		"help":      {},
		"memory":    {},
		"models":    {},
		"new":       {},
		"status":    {},
	}
)

type RuntimeSkillProvider struct {
	catalog skilldomain.SkillService
	limit   int
}

type runtimeSkillSelectionConfig struct {
	selectors    []string
	constrained  bool
	enforceLimit bool
}

func NewRuntimeSkillProvider(catalog skilldomain.SkillService) *RuntimeSkillProvider {
	return &RuntimeSkillProvider{
		catalog: catalog,
		limit:   defaultRuntimeSkillLimit,
	}
}

func (p *RuntimeSkillProvider) Load(ctx context.Context, run *RunSummary) ([]AgentSkill, error) {
	if p == nil || p.catalog == nil {
		return nil, nil
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.SpaceID <= 0 {
		return nil, nil
	}

	selection, err := runtimeSkillSelectionFromConfig(run.Config)
	if err != nil {
		return nil, err
	}
	selectors := selection.selectors
	constrained := selection.constrained
	slashSelector, slashExplicit := runtimeSlashSkillSelector(run.Input)
	if slashExplicit && !constrained {
		selectors = []string{slashSelector}
		constrained = true
	}
	if constrained && len(selectors) == 0 {
		return []AgentSkill{}, nil
	}

	enabled := true
	skills, err := p.catalog.List(ctx, run.SpaceID, nil, &enabled)
	if err != nil {
		return nil, err
	}

	selectorSet := make(map[string]struct{}, len(selectors))
	for _, selector := range selectors {
		selectorSet[selector] = struct{}{}
	}
	selected := make([]*skillentity.Skill, 0, len(skills))
	for _, skill := range skills {
		if skill == nil || !skill.Enabled || !isPromptSkillType(skill.Type) {
			continue
		}
		if constrained {
			id := strconv.FormatInt(skill.ID, 10)
			_, nameMatched := selectorSet[skill.Name]
			_, idMatched := selectorSet[id]
			if !nameMatched && !idMatched {
				continue
			}
		}
		if slashExplicit && skill.Name != slashSelector {
			continue
		}
		selected = append(selected, skill)
	}

	if len(selected) > p.limit {
		return nil, fmt.Errorf("selected skill count %d exceeds runtime limit %d", len(selected), p.limit)
	}

	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Name == selected[j].Name {
			return selected[i].ID < selected[j].ID
		}
		return selected[i].Name < selected[j].Name
	})

	result := make([]AgentSkill, 0, len(selected))
	totalBodyBytes := 0
	for _, skill := range selected {
		versions, err := p.catalog.ListVersions(ctx, skill.ID)
		if err != nil {
			return nil, err
		}
		if len(versions) == 0 || versions[0] == nil {
			return nil, fmt.Errorf("enabled skill %s has no version snapshot", skill.Name)
		}
		version := versions[0]
		decl, err := skilldomain.ParseDeclaration("SKILL.md", []byte(version.SkillMD))
		if err != nil {
			return nil, fmt.Errorf("parse enabled skill %s: %w", skill.Name, err)
		}
		body := strings.TrimSpace(decl.Body)
		if body == "" {
			return nil, fmt.Errorf("enabled skill %s has empty instructions", skill.Name)
		}
		totalBodyBytes += len([]byte(body))
		if totalBodyBytes > defaultRuntimeSkillTotalBodyBytes {
			return nil, fmt.Errorf("skill catalog content exceeds runtime budget %d bytes", defaultRuntimeSkillTotalBodyBytes)
		}
		result = append(result, AgentSkill{
			ID:          skill.ID,
			Name:        decl.Name,
			Description: decl.Description,
			Type:        decl.Type,
			Version:     decl.Version,
			Context:     decl.Context,
			Agent:       decl.Agent,
			Model:       decl.Model,
			Body:        body,
		})
	}

	return result, nil
}

func isPromptSkillType(typ skillentity.Type) bool {
	switch typ {
	case skillentity.TypeDeerSkill, skillentity.TypePublicSkill, skillentity.TypeCustomSkill:
		return true
	default:
		return false
	}
}

func runtimeSkillSelectors(rawConfig string) ([]string, bool, error) {
	rawConfig = strings.TrimSpace(rawConfig)
	if rawConfig == "" {
		return nil, false, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
		return nil, false, fmt.Errorf("parse run config failed: %w", err)
	}
	value, ok := payload["enable_skills"]
	if !ok {
		value, ok = payload["enableSkills"]
	}
	if !ok {
		return nil, false, nil
	}

	selectors, err := runtimeStringSelectorsFromValue(value, "run config enable_skills")
	return selectors, true, err
}

func runtimeSkillSelectionFromConfig(
	rawConfig string,
) (runtimeSkillSelectionConfig, error) {
	selectors, explicit, err := runtimeSkillSelectors(rawConfig)
	if err != nil {
		return runtimeSkillSelectionConfig{}, err
	}
	if explicit {
		return runtimeSkillSelectionConfig{
			selectors:    selectors,
			constrained:  true,
			enforceLimit: true,
		}, nil
	}

	rawConfig = strings.TrimSpace(rawConfig)
	if rawConfig == "" {
		return runtimeSkillSelectionConfig{}, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
		return runtimeSkillSelectionConfig{}, fmt.Errorf("parse run config failed: %w", err)
	}

	rawSkills, ok := payload["skills"]
	if !ok {
		rawSkills, ok = payload["Skills"]
	}
	if !ok || rawSkills == nil {
		return runtimeSkillSelectionConfig{}, nil
	}

	skills, ok := rawSkills.(map[string]any)
	if !ok {
		return runtimeSkillSelectionConfig{}, fmt.Errorf("run config skills must be an object")
	}

	if enabled, ok := skills["enabled"].(bool); ok && !enabled {
		return runtimeSkillSelectionConfig{constrained: true}, nil
	}

	value, ok := skills["allowed_skills"]
	if !ok {
		value, ok = skills["allowedSkills"]
	}
	if !ok {
		return runtimeSkillSelectionConfig{}, nil
	}

	allowedSelectors, err := runtimeStringSelectorsFromValue(
		value,
		"run config skills.allowed_skills",
	)
	if err != nil {
		return runtimeSkillSelectionConfig{}, err
	}
	if len(allowedSelectors) == 0 {
		return runtimeSkillSelectionConfig{}, nil
	}

	return runtimeSkillSelectionConfig{
		selectors:   allowedSelectors,
		constrained: true,
	}, nil
}

func runtimeStringSelectorsFromValue(value any, fieldName string) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", fieldName)
	}
	selectors := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		selector, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s items must be strings", fieldName)
		}
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		if _, ok := seen[selector]; ok {
			continue
		}
		seen[selector] = struct{}{}
		selectors = append(selectors, selector)
	}

	return selectors, nil
}

func runtimeSlashSkillSelector(rawInput string) (string, bool) {
	reference, ok := runtimeSlashSkillReference(rawInput)
	if !ok {
		return "", false
	}
	return reference.name, true
}

type runtimeSlashSkillActivation struct {
	name          string
	remainingText string
}

func runtimeSlashSkillReference(rawInput string) (runtimeSlashSkillActivation, bool) {
	content, ok := runtimeLatestUserInputText(rawInput)
	if !ok {
		return runtimeSlashSkillActivation{}, false
	}

	match := runtimeSlashSkillPattern.FindStringSubmatchIndex(content)
	if len(match) != 4 {
		return runtimeSlashSkillActivation{}, false
	}
	selector := content[match[2]:match[3]]
	if _, reserved := reservedRuntimeSlashSkillNames[selector]; reserved {
		return runtimeSlashSkillActivation{}, false
	}
	return runtimeSlashSkillActivation{
		name:          selector,
		remainingText: strings.TrimLeft(content[match[1]:], " \t\r\n"),
	}, true
}

func runtimeLatestUserInputText(rawInput string) (string, bool) {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return "", false
	}

	var payload struct {
		Message  string `json:"message"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(rawInput), &payload); err != nil {
		return rawInput, true
	}

	for i := len(payload.Messages) - 1; i >= 0; i-- {
		message := payload.Messages[i]
		if strings.ToLower(strings.TrimSpace(message.Role)) != "user" {
			continue
		}
		if text, ok := runtimeMessageContentText(message.Content); ok {
			return text, true
		}
	}
	if strings.TrimSpace(payload.Message) != "" {
		return payload.Message, true
	}
	return "", false
}

func runtimeMessageContentText(content any) (string, bool) {
	switch value := content.(type) {
	case string:
		return value, true
	case []any:
		var b strings.Builder
		for _, item := range value {
			part, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text, ok := part["text"].(string)
			if !ok || text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(text)
		}
		if b.Len() == 0 {
			return "", false
		}
		return b.String(), true
	default:
		return "", false
	}
}

func runWithSkillPrompt(run *RunSummary, skills AgentSkillContext) (*RunSummary, error) {
	if run == nil || len(skills.Items) == 0 {
		return run, nil
	}

	payload := map[string]any{}
	rawConfig := strings.TrimSpace(run.Config)
	if rawConfig != "" {
		if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
			return nil, fmt.Errorf("parse run config failed: %w", err)
		}
	}
	basePrompt := firstConfigString(payload, "system_prompt", "systemPrompt")
	payload["system_prompt"] = joinSkillSystemPrompt(basePrompt, skills, run.Input)
	delete(payload, "systemPrompt")
	config, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode skill run config failed: %w", err)
	}

	cloned := *run
	cloned.Config = string(config)
	return &cloned, nil
}

func joinSkillSystemPrompt(basePrompt string, skills AgentSkillContext, rawInput string) string {
	var b strings.Builder
	if basePrompt = strings.TrimSpace(basePrompt); basePrompt != "" {
		b.WriteString(basePrompt)
		b.WriteString("\n\n")
	}
	b.WriteString("## Enabled Skills\n")
	b.WriteString("Use the following task-specific operating instructions when relevant.\n")
	for _, skill := range skills.Items {
		b.WriteString("\n### Skill: ")
		b.WriteString(skill.Name)
		if skill.Version != "" {
			b.WriteString(" (")
			b.WriteString(skill.Version)
			b.WriteString(")")
		}
		b.WriteString("\n")
		if skill.Description != "" {
			b.WriteString("Description: ")
			b.WriteString(skill.Description)
			b.WriteString("\n")
		}
		b.WriteString("Instructions:\n")
		b.WriteString(skill.Body)
		b.WriteString("\n")
	}

	if activation, ok := runtimeMatchedSlashSkillActivation(rawInput, skills); ok {
		b.WriteString("\n## Slash Skill Activation\n")
		b.WriteString("The user explicitly activated the `")
		b.WriteString(activation.name)
		b.WriteString("` skill for this turn.\n")
		b.WriteString("Treat the task text as:\n<user_request>\n")
		if activation.remainingText == "" {
			b.WriteString("No additional task text was provided after the slash skill command.")
		} else {
			b.WriteString(html.EscapeString(activation.remainingText))
		}
		b.WriteString("\n</user_request>\n")
		b.WriteString("Follow this skill before choosing a general workflow.")
	}

	return strings.TrimSpace(b.String())
}

func runtimeMatchedSlashSkillActivation(rawInput string, skills AgentSkillContext) (runtimeSlashSkillActivation, bool) {
	activation, ok := runtimeSlashSkillReference(rawInput)
	if !ok {
		return runtimeSlashSkillActivation{}, false
	}
	for _, skill := range skills.Items {
		if skill.Name == activation.name {
			return activation, true
		}
	}
	return runtimeSlashSkillActivation{}, false
}

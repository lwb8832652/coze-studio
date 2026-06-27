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

	skillentity "github.com/coze-dev/coze-studio/backend/domain/skill/entity"
	skilldomain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
)

const (
	defaultRuntimeSkillLimit      = 20
	defaultRuntimeSkillBodyBudget = 64 << 10
)

type RuntimeSkillProvider struct {
	catalog    skilldomain.SkillService
	limit      int
	bodyBudget int
}

func NewRuntimeSkillProvider(catalog skilldomain.SkillService) *RuntimeSkillProvider {
	return &RuntimeSkillProvider{
		catalog:    catalog,
		limit:      defaultRuntimeSkillLimit,
		bodyBudget: defaultRuntimeSkillBodyBudget,
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

	selectors, explicit, err := runtimeSkillSelectors(run.Config)
	if err != nil {
		return nil, err
	}
	if explicit && len(selectors) == 0 {
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
		if explicit {
			id := strconv.FormatInt(skill.ID, 10)
			_, nameMatched := selectorSet[skill.Name]
			_, idMatched := selectorSet[id]
			if !nameMatched && !idMatched {
				continue
			}
		}
		selected = append(selected, skill)
	}

	if len(selected) > p.limit {
		return nil, fmt.Errorf("enabled skill count %d exceeds runtime limit %d", len(selected), p.limit)
	}

	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Name == selected[j].Name {
			return selected[i].ID < selected[j].ID
		}
		return selected[i].Name < selected[j].Name
	})

	result := make([]AgentSkill, 0, len(selected))
	bodyBytes := 0
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
		bodyBytes += len(body)
		if bodyBytes > p.bodyBudget {
			return nil, fmt.Errorf("enabled skill instructions exceed runtime budget %d bytes", p.bodyBudget)
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

	items, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("run config enable_skills must be an array")
	}
	selectors := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		selector, ok := item.(string)
		if !ok {
			return nil, true, fmt.Errorf("run config enable_skills items must be strings")
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

	return selectors, true, nil
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
	payload["system_prompt"] = joinSkillSystemPrompt(basePrompt, skills)
	delete(payload, "systemPrompt")
	config, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode skill run config failed: %w", err)
	}

	cloned := *run
	cloned.Config = string(config)
	return &cloned, nil
}

func joinSkillSystemPrompt(basePrompt string, skills AgentSkillContext) string {
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

	return strings.TrimSpace(b.String())
}

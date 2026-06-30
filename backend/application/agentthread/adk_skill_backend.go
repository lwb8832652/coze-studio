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
	"strings"

	einoskill "github.com/cloudwego/eino/adk/middlewares/skill"
)

type adkSkillBackend struct {
	matters           []einoskill.FrontMatter
	skills            map[string]einoskill.Skill
	description       string
	run               *RunSummary
	guardrailEnforcer ADKGuardrailEnforcer
}

type ADKSkillBackendOption func(*adkSkillBackend)

func WithADKSkillBackendGuardrail(
	run *RunSummary,
	enforcer ADKGuardrailEnforcer,
) ADKSkillBackendOption {
	return func(backend *adkSkillBackend) {
		if backend == nil {
			return
		}
		if run != nil {
			snapshot := *run
			backend.run = &snapshot
		}
		if isNilADKGuardrailEnforcer(enforcer) {
			return
		}
		backend.guardrailEnforcer = enforcer
	}
}

func newADKSkillBackend(
	skills []AgentSkill,
	budget ADKContextBudget,
	options ...ADKSkillBackendOption,
) (*adkSkillBackend, error) {
	if budget.SkillCatalogTokens <= 0 {
		return nil, fmt.Errorf("skill catalog token budget must be positive")
	}
	if budget.SkillContentTokens <= 0 {
		return nil, fmt.Errorf("skill content token budget must be positive")
	}

	selected := append([]AgentSkill(nil), skills...)
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Name == selected[j].Name {
			return selected[i].ID < selected[j].ID
		}
		return selected[i].Name < selected[j].Name
	})

	backend := &adkSkillBackend{
		matters: make([]einoskill.FrontMatter, 0, len(selected)),
		skills:  make(map[string]einoskill.Skill, len(selected)),
	}
	for _, option := range options {
		if option != nil {
			option(backend)
		}
	}
	catalog := struct {
		Instruction     string `json:"instruction"`
		AvailableSkills []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"available_skills"`
	}{
		Instruction: "Load one of the available task skills by exact name.",
		AvailableSkills: make([]struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}, 0, len(selected)),
	}
	for _, item := range selected {
		name := strings.TrimSpace(item.Name)
		description := strings.TrimSpace(item.Description)
		body := strings.TrimSpace(item.Body)
		if item.ID <= 0 || name == "" || body == "" {
			return nil, fmt.Errorf("skill snapshot is incomplete")
		}
		if _, exists := backend.skills[name]; exists {
			return nil, fmt.Errorf("duplicate skill name %s", name)
		}
		mode, err := adkSkillContextMode(item.Context)
		if err != nil {
			return nil, fmt.Errorf("skill %s: %w", name, err)
		}
		matter := einoskill.FrontMatter{
			Name:        name,
			Description: description,
			Context:     mode,
			Agent:       strings.TrimSpace(item.Agent),
			Model:       strings.TrimSpace(item.Model),
		}
		runtimeSkill := einoskill.Skill{
			FrontMatter: matter,
			Content:     body,
		}
		if tokens := estimateADKTextTokens(
			formatADKSkillContent(runtimeSkill),
		); tokens > budget.SkillContentTokens {
			return nil, fmt.Errorf(
				"skill %s content exceeds token budget %d",
				name,
				budget.SkillContentTokens,
			)
		}
		backend.matters = append(backend.matters, matter)
		backend.skills[name] = runtimeSkill

		catalog.AvailableSkills = append(catalog.AvailableSkills, struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}{
			Name:        name,
			Description: description,
		})
	}
	rawCatalog, err := json.Marshal(catalog)
	if err != nil {
		return nil, fmt.Errorf("encode skill catalog: %w", err)
	}
	backend.description = string(rawCatalog)
	if tokens := estimateADKTextTokens(backend.description); tokens > budget.SkillCatalogTokens {
		return nil, fmt.Errorf(
			"skill catalog exceeds token budget %d",
			budget.SkillCatalogTokens,
		)
	}

	return backend, nil
}

func formatADKSkillContent(skill einoskill.Skill) string {
	return fmt.Sprintf(
		"Loaded skill %q instructions:\n\n%s",
		skill.Name,
		skill.Content,
	)
}

func adkSkillContextMode(value string) (einoskill.ContextMode, error) {
	switch strings.TrimSpace(value) {
	case "", "inline":
		return "", nil
	case string(einoskill.ContextModeFork):
		return einoskill.ContextModeFork, nil
	case string(einoskill.ContextModeForkWithContext):
		return einoskill.ContextModeForkWithContext, nil
	default:
		return "", fmt.Errorf("unsupported context %q", value)
	}
}

func (b *adkSkillBackend) List(context.Context) ([]einoskill.FrontMatter, error) {
	if b == nil {
		return nil, fmt.Errorf("eino skill backend is required")
	}
	return append([]einoskill.FrontMatter(nil), b.matters...), nil
}

func (b *adkSkillBackend) Get(
	ctx context.Context,
	name string,
) (einoskill.Skill, error) {
	if b == nil {
		return einoskill.Skill{}, fmt.Errorf("eino skill backend is required")
	}
	name = strings.TrimSpace(name)
	item, ok := b.skills[name]
	if name == "" && len(b.skills) == 1 {
		for fallbackName, fallback := range b.skills {
			name = fallbackName
			item = fallback
			ok = true
		}
	}
	if !ok {
		return einoskill.Skill{}, fmt.Errorf("skill %s is not available", name)
	}
	if err := b.enforceGuardrail(ctx, name); err != nil {
		return einoskill.Skill{}, err
	}
	return item, nil
}

func (b *adkSkillBackend) enforceGuardrail(
	ctx context.Context,
	name string,
) error {
	if b == nil || b.guardrailEnforcer == nil {
		return nil
	}
	request := GuardrailRequest{
		TargetType: GuardrailTargetSkill,
		TargetID:   name,
		Operation:  "load",
		Source:     "adk_skill_backend",
		FailMode:   GuardrailFailClosed,
	}
	if b.run != nil {
		request.SpaceID = b.run.SpaceID
		request.ThreadID = b.run.ThreadID
		request.RunID = b.run.RunID
		request.UserID = b.run.CreatorID
	}
	result, err := b.guardrailEnforcer.Evaluate(ctx, request)
	if err != nil {
		return fmt.Errorf("skill guardrail blocked load: %w", err)
	}
	if result.Allowed {
		return nil
	}
	if result.RequiresConfirmation {
		return fmt.Errorf("skill guardrail blocked load: %w",
			&GuardrailConfirmationRequiredError{
				ReasonCode: result.Decision.ReasonCode,
			},
		)
	}
	return fmt.Errorf("skill guardrail blocked load: %w",
		&GuardrailDeniedError{ReasonCode: result.Decision.ReasonCode},
	)
}

func (b *adkSkillBackend) toolDescription() string {
	if b == nil {
		return ""
	}
	return b.description
}

func adkSkillToolDescription(b *adkSkillBackend) string {
	catalog := ""
	if b != nil {
		catalog = b.toolDescription()
	}
	return strings.TrimSpace(`Execute a selected task skill within the main conversation.

How to invoke:
- Use a JSON object with the required "skill" string field.
- The "skill" value must exactly match one name from the available_skills catalog.
- Example: {"skill":"skill-creator"}
- Do not pass extra arguments in this tool call.
- If the user selected only one skill and you decide to load it, use that listed skill name.

Available skills catalog JSON:
` + catalog)
}

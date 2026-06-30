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
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const adkSelectedSkillPreloadHeading = "## Selected Skill Instructions"

type adkSelectedSkillMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	inner         adk.ChatModelAgentMiddleware
	backend       *adkSkillBackend
	skillName     string
	shouldPreload bool
}

func newADKSelectedSkillMiddleware(
	run *RunSummary,
	skills AgentSkillContext,
	backend *adkSkillBackend,
	inner adk.ChatModelAgentMiddleware,
) (*adkSelectedSkillMiddleware, error) {
	skillName, shouldPreload, err := adkExplicitSingleInlineSkillName(run, skills)
	if err != nil {
		return nil, err
	}
	return &adkSelectedSkillMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		inner:                        inner,
		backend:                      backend,
		skillName:                    skillName,
		shouldPreload:                shouldPreload,
	}, nil
}

func (m *adkSelectedSkillMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if m == nil || m.inner == nil || m.shouldPreload {
		return ctx, runCtx, nil
	}
	return m.inner.BeforeAgent(ctx, runCtx)
}

func (m *adkSelectedSkillMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || !m.shouldPreload {
		if m != nil && m.inner != nil {
			return m.inner.BeforeModelRewriteState(ctx, state, mc)
		}
		return ctx, state, nil
	}
	if state == nil {
		return ctx, state, nil
	}
	skill, err := m.backend.Get(ctx, m.skillName)
	if err != nil {
		return ctx, state, err
	}
	content := adkSelectedSkillPreloadContent(m.skillName, formatADKSkillContent(skill))
	state.Messages = injectADKSelectedSkillMessage(state.Messages, m.skillName, content)
	return ctx, state, nil
}

func (m *adkSelectedSkillMiddleware) WrapModel(
	ctx context.Context,
	base model.BaseModel[*schema.Message],
	mc *adk.ModelContext,
) (model.BaseModel[*schema.Message], error) {
	if m == nil || m.inner == nil || m.shouldPreload {
		return base, nil
	}
	return m.inner.WrapModel(ctx, base, mc)
}

func adkExplicitSingleInlineSkillName(
	run *RunSummary,
	skills AgentSkillContext,
) (string, bool, error) {
	if run == nil || len(skills.Items) != 1 {
		return "", false, nil
	}
	item := skills.Items[0]
	if !adkSkillShouldPreloadInline(item.Context) {
		return "", false, nil
	}
	if activation, ok := runtimeMatchedSlashSkillActivation(run.Input, skills); ok && activation.name == item.Name {
		return item.Name, true, nil
	}
	selectors, explicit, err := runtimeSkillSelectors(run.Config)
	if err != nil {
		return "", false, err
	}
	if !explicit || len(selectors) != 1 {
		return "", false, nil
	}
	selector := strings.TrimSpace(selectors[0])
	if selector == "" {
		return "", false, nil
	}
	if selector != item.Name && selector != strconv.FormatInt(item.ID, 10) {
		return "", false, nil
	}
	return item.Name, true, nil
}

func adkSkillShouldPreloadInline(contextMode string) bool {
	switch strings.TrimSpace(contextMode) {
	case "", "inline":
		return true
	default:
		return false
	}
}

func adkSelectedSkillPreloadContent(skillName string, skillContent string) string {
	return strings.TrimSpace(fmt.Sprintf(
		"%s\nThe user explicitly selected the %q skill for this turn. Follow these instructions before choosing a general workflow.\n\n%s",
		adkSelectedSkillPreloadHeading,
		skillName,
		skillContent,
	))
}

func injectADKSelectedSkillMessage(
	messages []*schema.Message,
	skillName string,
	content string,
) []*schema.Message {
	if strings.TrimSpace(content) == "" {
		return messages
	}
	marker := adkSelectedSkillPreloadHeading + "\nThe user explicitly selected the " + fmt.Sprintf("%q", skillName)
	for _, message := range messages {
		if message == nil || message.Role != schema.System {
			continue
		}
		if strings.Contains(message.Content, marker) {
			return messages
		}
	}

	insertAt := 0
	for insertAt < len(messages) {
		message := messages[insertAt]
		if message == nil || message.Role != schema.System {
			break
		}
		insertAt++
	}
	out := make([]*schema.Message, 0, len(messages)+1)
	out = append(out, messages[:insertAt]...)
	out = append(out, schema.SystemMessage(content))
	out = append(out, messages[insertAt:]...)
	return out
}

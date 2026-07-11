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
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	crossagent "github.com/coze-dev/coze-studio/backend/crossdomain/agent/model"
	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
	"github.com/stretchr/testify/require"
)

func TestADKSingleAgentLeadPromptOverlaySkipsDefaultLead(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{}
	provider := NewADKSingleAgentLeadPromptOverlayProvider(source)

	overlay, ok, err := provider.ResolveADKLeadPromptOverlay(
		context.Background(),
		&RunSummary{AssistantID: "default", SpaceID: 10},
	)

	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, overlay)
	require.Empty(t, source.draftCalls)
	require.Empty(t, source.versionCalls)
}

func TestADKSingleAgentLeadPromptOverlayLoadsDraftAndModelDefaults(t *testing.T) {
	snapshot := leadPromptSingleAgentFixture(1001, 10, "reviewer", "Reviews changes", "", "Review carefully.")
	modelID := int64(100002)
	temperature := 0.3
	maxTokens := int32(2048)
	topP := 0.8
	snapshot.ModelInfo = &bot_common.ModelInfo{
		ModelId:     &modelID,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
	}
	source := &recordingSingleAgentDefinitionService{
		drafts: map[int64]*saEntity.SingleAgent{1001: snapshot},
	}
	provider := NewADKSingleAgentLeadPromptOverlayProvider(source)

	overlay, ok, err := provider.ResolveADKLeadPromptOverlay(
		context.Background(),
		&RunSummary{
			AssistantID: "singleagent:1001",
			SpaceID:     10,
			Config:      `{"single_agent":{"agent_id":1001,"is_draft":true}}`,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []int64{1001}, source.draftCalls)
	require.Equal(t, "reviewer", overlay.AgentName)
	require.Equal(t, "Reviews changes", overlay.AgentDescription)
	require.Equal(t, "Review carefully.", overlay.Instructions)
	require.Equal(t, modelID, overlay.ModelDefaults.ModelID)
	require.NotNil(t, overlay.ModelDefaults.Temperature)
	require.InDelta(t, float32(0.3), *overlay.ModelDefaults.Temperature, 0.0001)
	require.NotNil(t, overlay.ModelDefaults.MaxTokens)
	require.Equal(t, 2048, *overlay.ModelDefaults.MaxTokens)
	require.NotNil(t, overlay.ModelDefaults.TopP)
	require.InDelta(t, float32(0.8), *overlay.ModelDefaults.TopP, 0.0001)
}

func TestADKSingleAgentLeadPromptOverlayLoadsExplicitVersion(t *testing.T) {
	snapshot := leadPromptSingleAgentFixture(1001, 10, "reviewer", "Reviews changes", "v2", "Versioned prompt.")
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{"1001:v2": snapshot},
	}
	provider := NewADKSingleAgentLeadPromptOverlayProvider(source)

	overlay, ok, err := provider.ResolveADKLeadPromptOverlay(
		context.Background(),
		&RunSummary{
			AssistantID: "singleagent:1001",
			SpaceID:     10,
			Config:      `{"single_agent":{"agent_id":1001,"version":"v2"}}`,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []string{"1001:v2"}, source.versionCalls)
	require.Equal(t, "Versioned prompt.", overlay.Instructions)
}

func TestADKSingleAgentLeadPromptOverlayRejectsInvalidReferences(t *testing.T) {
	tests := []struct {
		name      string
		run       *RunSummary
		source    *recordingSingleAgentDefinitionService
		wantError string
	}{
		{
			name:      "malformed assistant id",
			run:       &RunSummary{AssistantID: "singleagent:nope", SpaceID: 10},
			source:    &recordingSingleAgentDefinitionService{},
			wantError: "invalid single agent assistant_id",
		},
		{
			name: "config id mismatch",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
				Config:      `{"single_agent":{"agent_id":1002}}`,
			},
			source:    &recordingSingleAgentDefinitionService{},
			wantError: "does not match assistant_id",
		},
		{
			name: "missing snapshot",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
			},
			source:    &recordingSingleAgentDefinitionService{drafts: map[int64]*saEntity.SingleAgent{}},
			wantError: "single agent lead prompt snapshot not found",
		},
		{
			name: "snapshot id mismatch",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
			},
			source: &recordingSingleAgentDefinitionService{drafts: map[int64]*saEntity.SingleAgent{
				1001: leadPromptSingleAgentFixture(1002, 10, "reviewer", "", "", "prompt"),
			}},
			wantError: "single agent lead prompt snapshot id mismatch",
		},
		{
			name: "cross space",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
			},
			source: &recordingSingleAgentDefinitionService{drafts: map[int64]*saEntity.SingleAgent{
				1001: leadPromptSingleAgentFixture(1001, 99, "reviewer", "", "", "prompt"),
			}},
			wantError: "single agent lead prompt access denied",
		},
		{
			name: "snapshot version mismatch",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
				Config:      `{"single_agent":{"version":"v2"}}`,
			},
			source: &recordingSingleAgentDefinitionService{versions: map[string]*saEntity.SingleAgent{
				"1001:v2": leadPromptSingleAgentFixture(1001, 10, "reviewer", "", "v1", "prompt"),
			}},
			wantError: "single agent lead prompt version mismatch",
		},
		{
			name: "versioned reference without version",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
				Config:      `{"single_agent":{"is_draft":false}}`,
			},
			source:    &recordingSingleAgentDefinitionService{},
			wantError: "versioned single agent lead prompt requires version",
		},
		{
			name: "oversized prompt",
			run: &RunSummary{
				AssistantID: "singleagent:1001",
				SpaceID:     10,
			},
			source: &recordingSingleAgentDefinitionService{drafts: map[int64]*saEntity.SingleAgent{
				1001: leadPromptSingleAgentFixture(
					1001,
					10,
					"reviewer",
					"",
					"",
					strings.Repeat("x", adkLeadPromptOverlayMaxBytes+1),
				),
			}},
			wantError: "durable agent prompt overlay exceeds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := NewADKSingleAgentLeadPromptOverlayProvider(test.source)

			_, _, err := provider.ResolveADKLeadPromptOverlay(context.Background(), test.run)

			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestADKApplyLeadPromptModelDefaultsPreservesExplicitRunOptions(t *testing.T) {
	temperature := float32(0.2)
	defaultTemperature := float32(0.7)
	defaultMaxTokens := 2048
	defaultTopP := float32(0.9)
	cfg := modelExecutorConfig{ModelID: 2002, Temperature: &temperature}
	defaults := modelExecutorConfig{
		ModelID:     1001,
		Temperature: &defaultTemperature,
		MaxTokens:   &defaultMaxTokens,
		TopP:        &defaultTopP,
	}

	applyADKLeadPromptModelDefaults(&cfg, defaults)

	require.Equal(t, int64(2002), cfg.ModelID)
	require.Same(t, &temperature, cfg.Temperature)
	require.Equal(t, 2048, *cfg.MaxTokens)
	require.InDelta(t, float32(0.9), *cfg.TopP, 0.0001)
}

func leadPromptSingleAgentFixture(
	agentID int64,
	spaceID int64,
	name string,
	description string,
	version string,
	prompt string,
) *saEntity.SingleAgent {
	return &saEntity.SingleAgent{SingleAgent: &crossagent.SingleAgent{
		AgentID: agentID,
		SpaceID: spaceID,
		Name:    name,
		Desc:    description,
		Version: version,
		Prompt:  &bot_common.PromptInfo{Prompt: &prompt},
	}}
}

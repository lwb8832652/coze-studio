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
	"testing"

	crossagent "github.com/coze-dev/coze-studio/backend/crossdomain/agent/model"
	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
	"github.com/stretchr/testify/require"
)

func TestADKRunConfigSubagentReferenceProviderParsesRefs(t *testing.T) {
	provider := NewADKRunConfigSubagentReferenceProvider()
	run := &RunSummary{
		RunID: 20,
		Config: `{
			"subagent_refs":[{
				"name":"researcher",
				"description":"Override description.",
				"agent_id":1001,
				"version":"v1",
				"is_draft":false,
				"allowed_tools":["read_file"],
				"allowed_dynamic_tools":["search_docs"],
				"full_chat_history":true
			}]
		}`,
	}

	refs, err := provider.ResolveADKSubagentReferences(context.Background(), run)

	require.NoError(t, err)
	require.Equal(t, []ADKSubagentReference{{
		Name:                   "researcher",
		Description:            "Override description.",
		AgentID:                1001,
		Version:                "v1",
		IsDraft:                false,
		AllowedTools:           []string{"read_file"},
		AllowedDynamicTools:    []string{"search_docs"},
		FullChatHistoryAsInput: true,
	}}, refs)
}

func TestADKSingleAgentSubagentDefinitionProviderLoadsVersionAndDraft(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v1": singleAgentDefinitionFixture(
				1001,
				"Researcher UI name",
				"Research public information.",
				"v1",
			),
		},
		drafts: map[int64]*saEntity.SingleAgent{
			1002: singleAgentDefinitionFixture(
				1002,
				"Writer UI name",
				"Write concise summaries.",
				"",
			),
		},
	}
	refProvider := ADKSubagentReferenceProviderFunc(func(
		context.Context,
		*RunSummary,
	) ([]ADKSubagentReference, error) {
		return []ADKSubagentReference{
			{Name: "researcher", AgentID: 1001, Version: "v1"},
			{Name: "writer", AgentID: 1002, IsDraft: true, FullChatHistoryAsInput: true},
		}, nil
	})
	provider := NewADKSingleAgentSubagentDefinitionProvider(source, refProvider)

	definitions, err := provider.ResolveADKSubagents(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Equal(t, []ADKSubagentDefinition{
		{
			Name:                "researcher",
			Description:         "Research public information.",
			AgentID:             1001,
			Version:             "v1",
			AllowedTools:        []string{},
			AllowedDynamicTools: []string{},
		},
		{
			Name:                   "writer",
			Description:            "Write concise summaries.",
			AgentID:                1002,
			IsDraft:                true,
			FullChatHistoryAsInput: true,
			AllowedTools:           []string{},
			AllowedDynamicTools:    []string{},
		},
	}, definitions)
	require.Equal(t, []string{"1001:v1"}, source.versionCalls)
	require.Equal(t, []int64{1002}, source.draftCalls)
}

func TestADKSingleAgentSubagentDefinitionProviderAppliesDurableToolGrants(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v1": singleAgentDefinitionFixture(
				1001,
				"Researcher UI name",
				"Research public information.",
				"v1",
			),
		},
	}
	grants := &recordingSubagentToolGrantProvider{
		grants: map[int64]ADKSubagentToolGrant{
			1001: {
				AllowedTools:        []string{"read_file"},
				AllowedDynamicTools: []string{"mcp_search"},
			},
		},
	}
	provider := NewADKSingleAgentSubagentDefinitionProviderWithToolGrants(
		source,
		ADKSubagentReferenceProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentReference, error) {
			return []ADKSubagentReference{{Name: "researcher", AgentID: 1001, Version: "v1"}}, nil
		}),
		grants,
	)

	definitions, err := provider.ResolveADKSubagents(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"read_file"}, definitions[0].AllowedTools)
	require.Equal(t, []string{"mcp_search"}, definitions[0].AllowedDynamicTools)
	require.Len(t, grants.requests, 1)
	require.Equal(t, int64(1001), grants.requests[0].Reference.AgentID)
	require.Equal(t, "researcher", grants.requests[0].Definition.Name)
	require.Equal(t, "Researcher UI name", grants.requests[0].Agent.Name)
}

func TestADKSingleAgentSubagentDefinitionProviderIntersectsRequestedToolsWithDurableGrant(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v1": singleAgentDefinitionFixture(1001, "Researcher", "desc", "v1"),
		},
	}
	provider := NewADKSingleAgentSubagentDefinitionProviderWithToolGrants(
		source,
		ADKSubagentReferenceProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentReference, error) {
			return []ADKSubagentReference{{
				Name:                "researcher",
				AgentID:             1001,
				Version:             "v1",
				AllowedTools:        []string{"read_file", "delete_file"},
				AllowedDynamicTools: []string{"mcp_search", "mcp_write"},
			}}, nil
		}),
		&recordingSubagentToolGrantProvider{
			grants: map[int64]ADKSubagentToolGrant{
				1001: {
					AllowedTools:        []string{"read_file"},
					AllowedDynamicTools: []string{"mcp_search"},
				},
			},
		},
	)

	definitions, err := provider.ResolveADKSubagents(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"read_file"}, definitions[0].AllowedTools)
	require.Equal(t, []string{"mcp_search"}, definitions[0].AllowedDynamicTools)
}

func TestADKSingleAgentSubagentDefinitionProviderPropagatesToolGrantErrors(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v1": singleAgentDefinitionFixture(1001, "Researcher", "desc", "v1"),
		},
	}
	provider := NewADKSingleAgentSubagentDefinitionProviderWithToolGrants(
		source,
		ADKSubagentReferenceProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentReference, error) {
			return []ADKSubagentReference{{Name: "researcher", AgentID: 1001, Version: "v1"}}, nil
		}),
		&recordingSubagentToolGrantProvider{err: fmt.Errorf("grant store unavailable")},
	)

	_, err := provider.ResolveADKSubagents(context.Background(), &RunSummary{RunID: 20})

	require.ErrorContains(t, err, "resolve subagent tool grant")
	require.ErrorContains(t, err, "grant store unavailable")
}

func TestADKSingleAgentSubagentDefinitionProviderUsesSafeFallbackName(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v1": singleAgentDefinitionFixture(
				1001,
				"Display Name With Spaces",
				"Research public information.",
				"v1",
			),
		},
	}
	provider := NewADKSingleAgentSubagentDefinitionProvider(
		source,
		ADKSubagentReferenceProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentReference, error) {
			return []ADKSubagentReference{{AgentID: 1001, Version: "v1"}}, nil
		}),
	)

	definitions, err := provider.ResolveADKSubagents(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Equal(t, "agent_1001", definitions[0].Name)
	require.Equal(t, "Research public information.", definitions[0].Description)
}

type recordingSubagentToolGrantProvider struct {
	grants   map[int64]ADKSubagentToolGrant
	requests []ADKSubagentToolGrantRequest
	err      error
}

func (p *recordingSubagentToolGrantProvider) ResolveADKSubagentToolGrant(
	_ context.Context,
	request ADKSubagentToolGrantRequest,
) (ADKSubagentToolGrant, error) {
	p.requests = append(p.requests, request)
	if p.err != nil {
		return ADKSubagentToolGrant{}, p.err
	}
	return p.grants[request.Reference.AgentID], nil
}

func TestADKSingleAgentSubagentDefinitionProviderValidatesRefs(t *testing.T) {
	source := &recordingSingleAgentDefinitionService{
		versions: map[string]*saEntity.SingleAgent{
			"1001:v1": singleAgentDefinitionFixture(1001, "A", "desc", "v1"),
			"1002:v1": singleAgentDefinitionFixture(1002, "B", "desc", "v1"),
		},
	}
	provider := NewADKSingleAgentSubagentDefinitionProvider(
		source,
		ADKSubagentReferenceProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentReference, error) {
			return []ADKSubagentReference{
				{Name: "researcher", AgentID: 1001, Version: "v1"},
				{Name: "researcher", AgentID: 1002, Version: "v1"},
			}, nil
		}),
	)

	_, err := provider.ResolveADKSubagents(context.Background(), &RunSummary{RunID: 20})

	require.ErrorContains(t, err, "duplicate subagent tool name")
}

func TestADKSingleAgentSubagentDefinitionProviderRejectsMissingAgent(t *testing.T) {
	provider := NewADKSingleAgentSubagentDefinitionProvider(
		&recordingSingleAgentDefinitionService{},
		ADKSubagentReferenceProviderFunc(func(
			context.Context,
			*RunSummary,
		) ([]ADKSubagentReference, error) {
			return []ADKSubagentReference{{Name: "researcher", AgentID: 1001, Version: "v1"}}, nil
		}),
	)

	_, err := provider.ResolveADKSubagents(context.Background(), &RunSummary{RunID: 20})

	require.ErrorContains(t, err, "subagent not found")
}

type recordingSingleAgentDefinitionService struct {
	drafts       map[int64]*saEntity.SingleAgent
	versions     map[string]*saEntity.SingleAgent
	draftCalls   []int64
	versionCalls []string
}

func (s *recordingSingleAgentDefinitionService) GetSingleAgentDraft(
	_ context.Context,
	agentID int64,
) (*saEntity.SingleAgent, error) {
	s.draftCalls = append(s.draftCalls, agentID)
	return s.drafts[agentID], nil
}

func (s *recordingSingleAgentDefinitionService) GetSingleAgent(
	_ context.Context,
	agentID int64,
	version string,
) (*saEntity.SingleAgent, error) {
	key := fmt.Sprintf("%d:%s", agentID, version)
	s.versionCalls = append(s.versionCalls, key)
	return s.versions[key], nil
}

func singleAgentDefinitionFixture(
	agentID int64,
	name string,
	description string,
	version string,
) *saEntity.SingleAgent {
	return &saEntity.SingleAgent{
		SingleAgent: &crossagent.SingleAgent{
			AgentID: agentID,
			Name:    name,
			Desc:    description,
			Version: version,
		},
	}
}

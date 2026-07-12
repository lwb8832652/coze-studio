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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultADKToolProviderWithSingleAgentSubagentsExposesBuiltinWithoutSource(
	t *testing.T,
) {
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(nil)

	policy, ok := provider.(*ADKToolPolicyProvider)
	require.True(t, ok)
	subagents, ok := policy.base.(*ADKSubagentToolProvider)
	require.True(t, ok)
	definitions, err := subagents.definition.ResolveADKSubagents(
		context.Background(),
		&RunSummary{Config: `{"mode":"ultra"}`},
	)
	require.NoError(t, err)
	require.Len(t, definitions, 1)
	require.Equal(t, "general_purpose", definitions[0].Name)
	require.NotEmpty(t, definitions[0].Description)
}

func TestDefaultADKToolProviderBuiltinRequiresExplicitSubagentMode(t *testing.T) {
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(nil)
	policy, ok := provider.(*ADKToolPolicyProvider)
	require.True(t, ok)
	subagents, ok := policy.base.(*ADKSubagentToolProvider)
	require.True(t, ok)
	definitions, err := subagents.definition.ResolveADKSubagents(
		context.Background(),
		&RunSummary{},
	)
	require.NoError(t, err)
	require.Empty(t, definitions)
}

func TestDefaultADKToolProviderWithSingleAgentSubagentsWiresSnapshotGrants(
	t *testing.T,
) {
	source := &recordingSingleAgentDefinitionService{}
	events := &recordingRunEventSink{}
	recorder := &recordingADKSubagentRunRecorder{}
	enforcer := &recordingADKGuardrailEnforcer{}
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(
		source,
		WithDefaultADKToolProviderEventSink(events),
		WithDefaultADKToolProviderSubagentRunRecorder(recorder),
		WithDefaultADKToolProviderGuardrailEnforcer(enforcer),
	)

	policy, ok := provider.(*ADKToolPolicyProvider)
	require.True(t, ok)
	subagents, ok := policy.base.(*ADKSubagentToolProvider)
	require.True(t, ok)
	require.Same(t, events, subagents.eventSink)
	require.Same(t, recorder, subagents.recorder)
	require.Same(t, enforcer, subagents.guardrailEnforcer)
	_, ok = subagents.base.(*adkHumanInteractionToolProvider)
	require.True(t, ok)

	definitionProvider, ok := subagents.definition.(*adkBuiltinSubagentDefinitionProvider)
	require.True(t, ok)
	configuredDefinition, ok := definitionProvider.configured.(*ADKSingleAgentSubagentDefinitionProvider)
	require.True(t, ok)
	sourceProvider, ok := configuredDefinition.source.(*recordingSingleAgentDefinitionService)
	require.True(t, ok)
	require.Same(t, source, sourceProvider)
	_, ok = configuredDefinition.references.(*ADKRunConfigSubagentReferenceProvider)
	require.True(t, ok)
	_, ok = configuredDefinition.grants.(*ADKSingleAgentSnapshotToolGrantProvider)
	require.True(t, ok)

	routingFactory, ok := subagents.factory.(*adkConfiguredOrBuiltinSubagentAgentFactory)
	require.True(t, ok)
	builtinFactory, ok := routingFactory.builtin.(*adkBuiltinSubagentAgentFactory)
	require.True(t, ok)
	builtinChildFactory, ok := builtinFactory.factory.(*ApplicationADKAgentFactory)
	require.True(t, ok)
	require.IsType(t, &adkBuiltinSubagentToolProvider{}, builtinChildFactory.toolProvider)
	subagentFactory, ok := routingFactory.configured.(*ADKSingleAgentSubagentAgentFactory)
	require.True(t, ok)
	factorySource, ok := subagentFactory.source.(*recordingSingleAgentDefinitionService)
	require.True(t, ok)
	require.Same(t, source, factorySource)
	childFactory, ok := subagentFactory.factory.(*ApplicationADKAgentFactory)
	require.True(t, ok)
	childPolicy, ok := childFactory.toolProvider.(*ADKToolPolicyProvider)
	require.True(t, ok)
	_, ok = childPolicy.base.(*adkHumanInteractionToolProvider)
	require.True(t, ok)
}

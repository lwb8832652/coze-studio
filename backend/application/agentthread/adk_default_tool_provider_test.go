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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultADKToolProviderWithSingleAgentSubagentsKeepsBaseWithoutSource(
	t *testing.T,
) {
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(nil)

	policy, ok := provider.(*ADKToolPolicyProvider)
	require.True(t, ok)
	_, ok = policy.base.(*adkHumanInteractionToolProvider)
	require.True(t, ok)
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

	definitionProvider, ok := subagents.definition.(*ADKSingleAgentSubagentDefinitionProvider)
	require.True(t, ok)
	sourceProvider, ok := definitionProvider.source.(*recordingSingleAgentDefinitionService)
	require.True(t, ok)
	require.Same(t, source, sourceProvider)
	_, ok = definitionProvider.references.(*ADKRunConfigSubagentReferenceProvider)
	require.True(t, ok)
	_, ok = definitionProvider.grants.(*ADKSingleAgentSnapshotToolGrantProvider)
	require.True(t, ok)

	subagentFactory, ok := subagents.factory.(*ADKSingleAgentSubagentAgentFactory)
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

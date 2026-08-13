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
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetiredExecutionModeHasNoProductionADKConsumer(t *testing.T) {
	consumerFiles := []string{
		"adk_agent_factory.go",
		"adk_builtin_subagent.go",
		"adk_lead_prompt.go",
		"adk_middleware.go",
		"adk_provider_capability.go",
		"adk_singleagent_subagent_agent_factory.go",
		"adk_subagent_tool_provider.go",
	}
	forbiddenIdentifiers := map[string]struct{}{
		"DeerFlowModePro": {}, "DeerFlowModeUltra": {},
		"DeerFlowRequestedPolicyAuto": {}, "DeerFlowRequestedPolicyPro": {},
		"DeerFlowRequestedPolicyUltra": {}, "ParseDeerFlowRuntimeConfig": {},
	}
	forbiddenSelectors := map[string]struct{}{
		"Mode": {}, "ModeExplicit": {},
		"RequestedPolicy": {}, "RequestedPolicyExplicit": {},
	}

	for _, path := range consumerFiles {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoError(t, err)
			ast.Inspect(file, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.Ident:
					_, forbidden := forbiddenIdentifiers[value.Name]
					require.False(t, forbidden, "%s still consumes retired execution mode identifier %s", path, value.Name)
				case *ast.SelectorExpr:
					_, forbidden := forbiddenSelectors[value.Sel.Name]
					require.False(t, forbidden, "%s still branches on retired execution mode selector %s", path, value.Sel.Name)
				}
				return true
			})
		})
	}
}

func TestADKChildConfigWritersDoNotWriteRetiredExecutionMode(t *testing.T) {
	for _, path := range []string{
		"adk_builtin_subagent.go",
		"adk_singleagent_subagent_agent_factory.go",
	} {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			require.NoError(t, err)
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				require.NoError(t, err)
				require.NotContains(t, []string{"requested_policy", "mode"}, value,
					"%s still writes retired execution mode key %s", path, value)
				return true
			})
		})
	}
}

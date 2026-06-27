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

	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
)

func TestADKToolPolicyProviderFiltersStaticAndDynamicTools(t *testing.T) {
	base := &recordingADKToolSetProvider{
		set: ADKToolSet{
			StaticTools: []tool.BaseTool{
				&namedTestTool{name: "safe_static"},
				&namedTestTool{name: "blocked_static"},
			},
			DynamicTools: []tool.BaseTool{
				&namedTestTool{name: "safe_dynamic"},
				&namedTestTool{name: "blocked_dynamic"},
			},
		},
	}
	provider := NewADKToolPolicyProvider(base)

	set, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		RunID: 20,
		Config: `{
			"tool_policy":{
				"allowed_tools":["safe_static"],
				"allowed_dynamic_tools":["safe_dynamic"]
			}
		}`,
	})

	require.NoError(t, err)
	require.Equal(t, 1, base.resolveToolSetCalls)
	require.Equal(
		t,
		[]string{"safe_static"},
		adkToolNames(t, context.Background(), set.StaticTools),
	)
	require.Equal(
		t,
		[]string{"safe_dynamic"},
		adkToolNames(t, context.Background(), set.DynamicTools),
	)
}

func TestADKToolPolicyProviderExplicitEmptyAllowListDeniesAll(t *testing.T) {
	provider := NewADKToolPolicyProvider(&recordingADKToolSetProvider{
		set: ADKToolSet{
			StaticTools: []tool.BaseTool{
				&namedTestTool{name: "static_tool"},
			},
			DynamicTools: []tool.BaseTool{
				&namedTestTool{name: "dynamic_tool"},
			},
		},
	})

	set, err := provider.ResolveToolSet(context.Background(), &RunSummary{
		RunID: 20,
		Config: `{
			"tool_policy":{
				"allowed_tools":[],
				"allowed_dynamic_tools":[]
			}
		}`,
	})

	require.NoError(t, err)
	require.Empty(t, set.StaticTools)
	require.Empty(t, set.DynamicTools)
}

func TestADKToolPolicyProviderKeepsToolsWhenPolicyIsAbsent(t *testing.T) {
	provider := NewADKToolPolicyProvider(&recordingADKToolSetProvider{
		set: ADKToolSet{
			StaticTools: []tool.BaseTool{
				&namedTestTool{name: "static_tool"},
			},
			DynamicTools: []tool.BaseTool{
				&namedTestTool{name: "dynamic_tool"},
			},
		},
	})

	set, err := provider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Equal(
		t,
		[]string{"static_tool"},
		adkToolNames(t, context.Background(), set.StaticTools),
	)
	require.Equal(
		t,
		[]string{"dynamic_tool"},
		adkToolNames(t, context.Background(), set.DynamicTools),
	)
}

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

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	crossagent "github.com/coze-dev/coze-studio/backend/crossdomain/agent/model"
	saEntity "github.com/coze-dev/coze-studio/backend/domain/agent/singleagent/entity"
	"github.com/stretchr/testify/require"
)

func TestADKSingleAgentSnapshotToolGrantProviderMapsPluginsAndWorkflows(t *testing.T) {
	pluginID := int64(11)
	apiID := int64(22)
	apiName := "search_docs"
	workflowID := int64(33)
	workflowName := "summarize_report"
	provider := NewADKSingleAgentSnapshotToolGrantProvider()

	grant, err := provider.ResolveADKSubagentToolGrant(
		context.Background(),
		ADKSubagentToolGrantRequest{
			Agent: &saEntity.SingleAgent{SingleAgent: &crossagent.SingleAgent{
				Plugin: []*bot_common.PluginInfo{{
					PluginId: &pluginID,
					ApiId:    &apiID,
					ApiName:  &apiName,
				}},
				Workflow: []*bot_common.WorkflowInfo{{
					WorkflowId:   &workflowID,
					WorkflowName: &workflowName,
				}},
			}},
		},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"search_docs", "summarize_report"}, grant.AllowedTools)
	require.Empty(t, grant.AllowedDynamicTools)
}

func TestADKSingleAgentSnapshotToolGrantProviderUsesStableFallbackNames(t *testing.T) {
	pluginID := int64(11)
	apiID := int64(22)
	apiName := "Search Docs!"
	workflowID := int64(33)
	workflowName := "Summarize Report!"
	provider := NewADKSingleAgentSnapshotToolGrantProvider()

	grant, err := provider.ResolveADKSubagentToolGrant(
		context.Background(),
		ADKSubagentToolGrantRequest{
			Agent: &saEntity.SingleAgent{SingleAgent: &crossagent.SingleAgent{
				Plugin: []*bot_common.PluginInfo{{
					PluginId: &pluginID,
					ApiId:    &apiID,
					ApiName:  &apiName,
				}},
				Workflow: []*bot_common.WorkflowInfo{{
					WorkflowId:   &workflowID,
					WorkflowName: &workflowName,
				}},
			}},
		},
	)

	require.NoError(t, err)
	require.Equal(t, []string{"plugin_11_22", "workflow_33"}, grant.AllowedTools)
}

func TestADKSingleAgentSnapshotToolGrantProviderSkipsIncompleteEntries(t *testing.T) {
	apiName := "search_docs"
	provider := NewADKSingleAgentSnapshotToolGrantProvider()

	grant, err := provider.ResolveADKSubagentToolGrant(
		context.Background(),
		ADKSubagentToolGrantRequest{
			Agent: &saEntity.SingleAgent{SingleAgent: &crossagent.SingleAgent{
				Plugin: []*bot_common.PluginInfo{{
					ApiName: &apiName,
				}},
				Workflow: []*bot_common.WorkflowInfo{{}},
			}},
		},
	)

	require.NoError(t, err)
	require.Empty(t, grant.AllowedTools)
	require.Empty(t, grant.AllowedDynamicTools)
}

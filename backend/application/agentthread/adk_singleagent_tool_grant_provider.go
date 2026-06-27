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
	"strings"

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
)

type ADKSingleAgentSnapshotToolGrantProvider struct{}

func NewADKSingleAgentSnapshotToolGrantProvider() *ADKSingleAgentSnapshotToolGrantProvider {
	return &ADKSingleAgentSnapshotToolGrantProvider{}
}

func (p *ADKSingleAgentSnapshotToolGrantProvider) ResolveADKSubagentToolGrant(
	_ context.Context,
	request ADKSubagentToolGrantRequest,
) (ADKSubagentToolGrant, error) {
	if request.Agent == nil || request.Agent.SingleAgent == nil {
		return ADKSubagentToolGrant{}, nil
	}

	allowed := make([]string, 0, len(request.Agent.Plugin)+len(request.Agent.Workflow))
	for _, plugin := range request.Agent.Plugin {
		if name := adkPluginToolGrantName(plugin); name != "" {
			allowed = append(allowed, name)
		}
	}
	for _, workflow := range request.Agent.Workflow {
		if name := adkWorkflowToolGrantName(workflow); name != "" {
			allowed = append(allowed, name)
		}
	}

	return ADKSubagentToolGrant{
		AllowedTools: normalizeConfigStringSlice(allowed),
	}, nil
}

func adkPluginToolGrantName(plugin *bot_common.PluginInfo) string {
	if plugin == nil || plugin.GetPluginId() <= 0 || plugin.GetApiId() <= 0 {
		return ""
	}
	if name := strings.TrimSpace(plugin.GetApiName()); isADKToolPolicyName(name) {
		return name
	}
	return fmt.Sprintf("plugin_%d_%d", plugin.GetPluginId(), plugin.GetApiId())
}

func adkWorkflowToolGrantName(workflow *bot_common.WorkflowInfo) string {
	if workflow == nil || workflow.GetWorkflowId() <= 0 {
		return ""
	}
	if name := strings.TrimSpace(workflow.GetWorkflowName()); isADKToolPolicyName(name) {
		return name
	}
	return fmt.Sprintf("workflow_%d", workflow.GetWorkflowId())
}

func isADKToolPolicyName(name string) bool {
	return isADKSubagentToolName(name)
}

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

package workbench

import (
	"strings"

	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
)

var asyncHints = []string{
	"任务",
	"异步",
	"长时间",
	"生成报告",
	"run task",
	"async",
}

func ResolveIntent(req *chatapi.WorkbenchChatRequest) Intent {
	if req == nil {
		return Intent{Kind: IntentChat}
	}

	requiresAsync := containsAny(strings.ToLower(req.Message), asyncHints)
	if req.IsSetSelectedSkillID() {
		return Intent{Kind: IntentSkill, RequiresAsync: requiresAsync}
	}
	if requiresAsync {
		return Intent{Kind: IntentTask, RequiresAsync: true}
	}

	message := strings.ToLower(req.Message)
	if strings.Contains(message, "agent") || strings.Contains(message, "智能体") {
		return Intent{Kind: IntentAgent}
	}
	return Intent{Kind: IntentChat}
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

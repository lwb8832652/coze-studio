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
	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/skill"
	"github.com/coze-dev/coze-studio/backend/application/task"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	agentrun "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/service"
)

var SVC = new(ApplicationService)

type ServiceComponents struct {
	SkillSVC          *skill.ApplicationService
	TaskSVC           *task.ApplicationService
	AgentThreadSVC    *agentthread.ApplicationService
	KnowledgeSVC      crossknowledge.Knowledge
	AgentRunSVC       agentrun.Run
	ChatModelProvider chatModelProvider
}

func InitService(c *ServiceComponents) *ApplicationService {
	if c == nil {
		return SVC
	}
	if c.SkillSVC != nil {
		SVC.skillSVC = c.SkillSVC
	}
	if c.TaskSVC != nil {
		SVC.taskSVC = c.TaskSVC
	}
	if c.AgentThreadSVC != nil {
		SVC.agentThreadSVC = c.AgentThreadSVC
	}
	if c.KnowledgeSVC != nil {
		SVC.knowledgeSVC = c.KnowledgeSVC
	}
	if c.AgentRunSVC != nil {
		SVC.agentRunSVC = c.AgentRunSVC
	}
	if c.ChatModelProvider != nil {
		SVC.chatModelProvider = c.ChatModelProvider
	}
	return SVC
}

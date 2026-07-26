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
	"github.com/coze-dev/coze-studio/backend/application/mcptool"
	"github.com/coze-dev/coze-studio/backend/application/skill"
)

type ApplicationService struct {
	skillSVC          *skill.ApplicationService
	mcpToolSVC        *mcptool.ApplicationService
	chatModelProvider chatModelProvider
	sandboxRepository SandboxRuntimeDiagnosticRepository
}

var SVC = new(ApplicationService)

type ServiceComponents struct {
	SkillSVC          *skill.ApplicationService
	MCPToolSVC        *mcptool.ApplicationService
	ChatModelProvider chatModelProvider
	SandboxRepository SandboxRuntimeDiagnosticRepository
}

func InitService(c *ServiceComponents) *ApplicationService {
	if c == nil {
		return SVC
	}
	if c.SkillSVC != nil {
		SVC.skillSVC = c.SkillSVC
	}
	if c.MCPToolSVC != nil {
		SVC.mcpToolSVC = c.MCPToolSVC
	}
	if c.ChatModelProvider != nil {
		SVC.chatModelProvider = c.ChatModelProvider
	}
	if c.SandboxRepository != nil {
		SVC.sandboxRepository = c.SandboxRepository
	}
	return SVC
}

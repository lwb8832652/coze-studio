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

package skill

import (
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/skill/repository"
	domain "github.com/coze-dev/coze-studio/backend/domain/skill/service"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type ServiceComponents struct {
	DB         *gorm.DB
	IDGen      idgen.IDGenerator
	CodeRunner coderunner.Runner
}

func InitService(c *ServiceComponents) *ApplicationService {
	repo := repository.NewSkillRepository(c.DB, c.IDGen)
	SVC.DomainSVC = domain.NewService(&domain.Components{
		Repo:           repo,
		IDGen:          c.IDGen,
		ScriptRunner:   &domain.ScriptExecutor{Runner: c.CodeRunner},
		WorkflowRunner: domain.UnsupportedExecutor{},
	})
	return SVC
}

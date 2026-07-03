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
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type ServiceComponents struct {
	DB            *gorm.DB
	IDGen         idgen.IDGenerator
	ObjectStorage storage.Storage
}

func InitService(c *ServiceComponents) *ApplicationService {
	if c == nil {
		return SVC
	}

	repo := repository.NewThreadRepository(c.DB)
	guardrailAuditRepo := repository.NewGuardrailAuditRepository(c.DB)
	mcpRuntimeAuditRepo := repository.NewMCPRuntimeAuditRepository(c.DB)
	SVC.ThreadSVC = domainservice.NewService(&domainservice.Components{
		Repo:  repo,
		IDGen: c.IDGen,
	})
	SVC.RuntimeFileSVC = domainservice.NewRuntimeFileService(
		&domainservice.RuntimeFileComponents{
			RunReader: repo,
			FileRepo:  repository.NewRuntimeFileRepository(c.DB),
			IDGen:     c.IDGen,
		},
	)
	SVC.UploadFileSVC = domainservice.NewUploadFileService(
		&domainservice.UploadFileComponents{
			ThreadReader: repo,
			FileRepo:     repository.NewThreadUploadFileRepository(c.DB),
			IDGen:        c.IDGen,
		},
	)
	SVC.PlanSVC = domainservice.NewPlanService(
		&domainservice.PlanComponents{
			RunReader: repo,
			PlanRepo:  repository.NewPlanRepository(c.DB),
			IDGen:     c.IDGen,
		},
	)
	SVC.ArtifactSVC = domainservice.NewArtifactService(
		&domainservice.ArtifactComponents{
			FileReader:   repository.NewRuntimeFileRepository(c.DB),
			ArtifactRepo: repository.NewArtifactRepository(c.DB),
			IDGen:        c.IDGen,
		},
	)
	SVC.ArtifactObjectStorage = c.ObjectStorage
	SVC.ArtifactAuthorizer = NewThreadOwnerArtifactAuthorizer(SVC.ThreadSVC)
	SVC.MemoryAuthorizer = NewThreadOwnerMemoryAuthorizer(SVC.ThreadSVC)
	SVC.GuardrailAuditRepository = guardrailAuditRepo
	SVC.GuardrailAuditAuthorizer = NewThreadOwnerGuardrailAuditAuthorizer(SVC.ThreadSVC)
	SVC.MCPRuntimeAuditRepository = mcpRuntimeAuditRepo
	SVC.MCPRuntimeAuditAuthorizer = NewThreadOwnerMCPRuntimeAuditAuthorizer(SVC.ThreadSVC)
	SVC.ArtifactScanner, SVC.ArtifactScannerStatus = NewArtifactContentScannerFromEnvWithStatus()
	SVC.ArtifactScanReadPolicy = NewArtifactScanReadPolicyConfigFromEnv()
	SVC.MemoryExtractor = NewModelMemoryExtractorFromEnv(NewThreadUsageCollector(SVC))

	return SVC
}

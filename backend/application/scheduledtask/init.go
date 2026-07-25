// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type WorkflowService interface {
	WorkflowDomain
	WorkflowMetadataDomain
}

type ServiceComponents struct {
	DB                *gorm.DB
	IDGen             idgen.IDGenerator
	UserSpaceReader   SpaceAuthorizer
	UserProfileReader UserProfileReader
	AgentThreadClient AgentThreadClient
	WorkflowDomain    WorkflowService
	NotificationOutbox repository.NotificationOutboxAppender
	RootContext       context.Context
}

func InitService(c *ServiceComponents) (*ApplicationService, *Worker, error) {
	if c == nil || c.DB == nil || c.IDGen == nil || c.UserSpaceReader == nil || c.AgentThreadClient == nil || c.WorkflowDomain == nil || c.NotificationOutbox == nil {
		return nil, nil, fmt.Errorf("scheduled task service dependencies are incomplete")
	}
	repo := repository.NewMySQLRepository(c.DB, c.IDGen, repository.WithNotificationOutboxAppender(c.NotificationOutbox))
	targets := &CozeTargetCatalog{
		AgentTargets:    &MySQLAgentTargetReader{DB: c.DB},
		WorkflowTargets: &DomainWorkflowTargetReader{Domain: c.WorkflowDomain},
	}
	dispatcher := &ExecutionDispatcher{
		Repository: repo,
		Executors: map[entity.TargetType]TaskExecutor{
			entity.TargetTypeAgent: &AgentTaskExecutor{
				Runner:     &CozeAgentRunner{Client: c.AgentThreadClient},
				Repository: repo,
			},
			entity.TargetTypeWorkflow: &WorkflowTaskExecutor{
				Runner:     &DomainWorkflowRunner{Domain: c.WorkflowDomain},
				Repository: repo,
			},
		},
		RootContext: c.RootContext,
	}
	service := &ApplicationService{
		Repository:      repo,
		Authorizer:      c.UserSpaceReader,
		Users:           c.UserProfileReader,
		Targets:         targets,
		Dispatcher:      dispatcher,
		MaxTasksPerUser: defaultTaskLimit,
	}
	worker := &Worker{Repository: repo, Dispatcher: dispatcher, Owner: defaultWorkerOwner()}
	SVC = service
	return service, worker, nil
}

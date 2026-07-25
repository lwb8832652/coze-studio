// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gorm.io/gorm"

	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	"github.com/coze-dev/coze-studio/backend/application/scheduledtask"
	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
	infraimchannel "github.com/coze-dev/coze-studio/backend/infra/imchannel"
)

const RuntimeStableFailureThresholdEnv = "IM_CHANNEL_RUNTIME_STABLE_FAILURE_THRESHOLD"

type Components struct {
	DB           *gorm.DB
	IDGen        idgen.IDGenerator
	Roles        UserSpaceRoleReader
	AgentThreads AgentThreadClient
	RootContext  context.Context
	RuntimeStableFailureThreshold int
}

type scheduledAgentCatalog struct {
	reader *scheduledtask.MySQLAgentTargetReader
}

func (c *scheduledAgentCatalog) Resolve(
	ctx context.Context,
	spaceID, agentID int64,
) (*AgentTarget, error) {
	if c == nil || c.reader == nil {
		return nil, domain.ErrAgentUnavailable
	}
	target, err := c.reader.Resolve(ctx, spaceID, agentID)
	if err != nil || target == nil || !target.Published {
		return nil, domain.ErrAgentUnavailable
	}
	return &AgentTarget{ID: target.ID, Name: target.Name}, nil
}

func InitService(components *Components) (*Service, *RuntimeManager, error) {
	if components == nil || components.DB == nil || components.IDGen == nil ||
		components.Roles == nil || components.AgentThreads == nil {
		return nil, nil, domain.ErrRuntimeUnavailable
	}
	rootContext := components.RootContext
	if rootContext == nil {
		rootContext = context.Background()
	}
	repository := infraimchannel.NewMySQLRepository(
		components.DB,
		infraimchannel.WithNotificationOutboxAppender(imChannelNotificationOutbox{}),
	)
	codec, err := LoadCredentialCodec(os.Getenv)
	if err != nil {
		return nil, nil, fmt.Errorf("load Feishu IM credential codec: %w", err)
	}
	agents := &scheduledAgentCatalog{
		reader: &scheduledtask.MySQLAgentTargetReader{DB: components.DB},
	}
	service, err := NewService(
		repository,
		components.IDGen,
		components.Roles,
		agents,
		codec,
		OfficialConnectionTester{},
	)
	if err != nil {
		return nil, nil, err
	}
	runner := NewAgentRunner(components.AgentThreads, repository, components.IDGen)
	runtimeManager, err := NewRuntimeManager(
		repository,
		components.IDGen,
		codec,
		OfficialChannelFactory{},
		runner,
		WithRuntimeStableFailureThreshold(runtimeStableFailureThresholdForComponents(components)),
	)
	if err != nil {
		return nil, nil, err
	}
	service.SetRuntimeWake(runtimeManager.Wake)
	SVC = service
	runtimeManager.Start(rootContext)
	return service, runtimeManager, nil
}

func RuntimeStableFailureThresholdFromEnv(getenv func(string) string) int {
	if getenv == nil {
		return domain.DefaultRuntimeStableFailureThreshold
	}
	raw := strings.TrimSpace(getenv(RuntimeStableFailureThresholdEnv))
	if raw == "" {
		return domain.DefaultRuntimeStableFailureThreshold
	}
	threshold, err := strconv.Atoi(raw)
	if err != nil || threshold <= 0 {
		return domain.DefaultRuntimeStableFailureThreshold
	}
	return threshold
}

func runtimeStableFailureThresholdForComponents(components *Components) int {
	if components != nil && components.RuntimeStableFailureThreshold > 0 {
		return components.RuntimeStableFailureThreshold
	}
	return domain.DefaultRuntimeStableFailureThreshold
}

type imChannelNotificationOutbox struct{}

func (imChannelNotificationOutbox) AppendInTransaction(
	ctx context.Context,
	tx *gorm.DB,
	event domainnotification.Event,
) error {
	return appnotification.SVC.AppendInTransaction(ctx, tx, event)
}

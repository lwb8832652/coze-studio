// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const (
	spaceRoleOwner int32 = 1
	spaceRoleAdmin int32 = 2

	maxConfigNameBytes = 80
	maxAppIDBytes      = 128
	maxAppSecretBytes  = 512
)

type UserSpaceRoleReader interface {
	GetUserSpaceList(ctx context.Context, userID int64) ([]*userentity.Space, error)
}

type AgentTarget struct {
	ID   int64
	Name string
}

type AgentCatalog interface {
	Resolve(ctx context.Context, spaceID, agentID int64) (*AgentTarget, error)
}

type BotIdentity struct {
	OpenID string
	Name   string
}

type ConnectionTester interface {
	Test(ctx context.Context, appID, appSecret string) (*BotIdentity, error)
}

type ConfigView struct {
	ID               int64
	SpaceID          int64
	CreatorID        int64
	AgentID          int64
	AgentName        string
	ChannelType      string
	Name             string
	AppID            string
	SecretConfigured bool
	Enabled          bool
	ReplyMode        domain.ReplyMode
	GroupPolicy      domain.GroupPolicy
	RuntimeStatus    domain.RuntimeStatus
	RuntimeError     string
	BotOpenID        string
	BotName          string
	LastConnectedAt  *time.Time
	LastTestedAt     *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ListResult struct {
	Configs         []*ConfigView
	CanManage       bool
	CredentialReady bool
}

type CreateRequest struct {
	SpaceID     int64
	ActorID     int64
	Name        string
	AppID       string
	AppSecret   string
	AgentID     int64
	Enabled     bool
	ReplyMode   domain.ReplyMode
	GroupPolicy domain.GroupPolicy
}

type UpdateRequest struct {
	SpaceID     int64
	ConfigID    int64
	ActorID     int64
	Name        string
	AppID       string
	AppSecret   string
	AgentID     int64
	ReplyMode   domain.ReplyMode
	GroupPolicy domain.GroupPolicy
}

type Service struct {
	repository domain.Repository
	idGen      idgen.IDGenerator
	roles      UserSpaceRoleReader
	agents     AgentCatalog
	codec      CredentialCodec
	tester     ConnectionTester
	wake       func()
}

var SVC *Service

func NewService(
	repository domain.Repository,
	idGenerator idgen.IDGenerator,
	roles UserSpaceRoleReader,
	agents AgentCatalog,
	codec CredentialCodec,
	tester ConnectionTester,
) (*Service, error) {
	if repository == nil || idGenerator == nil || roles == nil || agents == nil || tester == nil {
		return nil, domain.ErrRuntimeUnavailable
	}
	return &Service{
		repository: repository,
		idGen:      idGenerator,
		roles:      roles,
		agents:     agents,
		codec:      codec,
		tester:     tester,
	}, nil
}

func (s *Service) SetRuntimeWake(wake func()) {
	if s != nil {
		s.wake = wake
	}
}

func (s *Service) List(ctx context.Context, actorID, spaceID int64) (*ListResult, error) {
	role, err := s.resolveRole(ctx, actorID, spaceID)
	if err != nil {
		return nil, err
	}
	configs, err := s.repository.ListConfigs(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	views := make([]*ConfigView, 0, len(configs))
	for _, config := range configs {
		views = append(views, s.configView(ctx, config))
	}
	return &ListResult{
		Configs:         views,
		CanManage:       role == spaceRoleOwner || role == spaceRoleAdmin,
		CredentialReady: s.codec != nil,
	}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*ConfigView, error) {
	if err := s.authorizeManage(ctx, req.ActorID, req.SpaceID); err != nil {
		return nil, err
	}
	if err := validateMutation(req.Name, req.AppID, req.AppSecret, req.AgentID, req.ReplyMode, req.GroupPolicy, true); err != nil {
		return nil, err
	}
	if s.codec == nil {
		return nil, domain.ErrCredentialCodecMissing
	}
	if _, err := s.agents.Resolve(ctx, req.SpaceID, req.AgentID); err != nil {
		return nil, domain.ErrAgentUnavailable
	}
	configID, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	secret, fingerprint, err := s.codec.Encrypt(configID, strings.TrimSpace(req.AppSecret))
	if err != nil {
		return nil, fmt.Errorf("encrypt Feishu app secret: %w", err)
	}
	now := time.Now()
	status := domain.RuntimeStatusDisabled
	if req.Enabled {
		status = domain.RuntimeStatusPending
	}
	config := &domain.Config{
		ID:                   configID,
		SpaceID:              req.SpaceID,
		CreatorID:            req.ActorID,
		UpdatedBy:            req.ActorID,
		AgentID:              req.AgentID,
		ChannelType:          domain.ChannelTypeFeishu,
		Name:                 strings.TrimSpace(req.Name),
		AppID:                strings.TrimSpace(req.AppID),
		AppSecretCiphertext:  secret,
		AppSecretFingerprint: fingerprint,
		Enabled:              req.Enabled,
		ReplyMode:            req.ReplyMode,
		GroupPolicy:          req.GroupPolicy,
		RuntimeStatus:        status,
		Version:              1,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.repository.CreateConfig(ctx, config); err != nil {
		return nil, err
	}
	s.notifyRuntime()
	return s.configView(ctx, config), nil
}

func (s *Service) Update(ctx context.Context, req UpdateRequest) (*ConfigView, error) {
	if err := s.authorizeManage(ctx, req.ActorID, req.SpaceID); err != nil {
		return nil, err
	}
	if err := validateMutation(req.Name, req.AppID, req.AppSecret, req.AgentID, req.ReplyMode, req.GroupPolicy, false); err != nil {
		return nil, err
	}
	current, err := s.repository.GetConfig(ctx, req.SpaceID, req.ConfigID)
	if err != nil {
		return nil, err
	}
	if _, err := s.agents.Resolve(ctx, req.SpaceID, req.AgentID); err != nil {
		return nil, domain.ErrAgentUnavailable
	}
	nextAppID := strings.TrimSpace(req.AppID)
	nextSecret := strings.TrimSpace(req.AppSecret)
	if nextAppID != current.AppID && nextSecret == "" {
		return nil, domain.ErrInvalidInput
	}

	current.Name = strings.TrimSpace(req.Name)
	current.AppID = nextAppID
	current.AgentID = req.AgentID
	current.ReplyMode = req.ReplyMode
	current.GroupPolicy = req.GroupPolicy
	current.UpdatedBy = req.ActorID
	current.RuntimeStatus = domain.RuntimeStatusDisabled
	if current.Enabled {
		current.RuntimeStatus = domain.RuntimeStatusPending
	}
	if nextSecret != "" {
		if s.codec == nil {
			return nil, domain.ErrCredentialCodecMissing
		}
		ciphertext, fingerprint, encryptErr := s.codec.Encrypt(current.ID, nextSecret)
		if encryptErr != nil {
			return nil, fmt.Errorf("encrypt Feishu app secret: %w", encryptErr)
		}
		current.AppSecretCiphertext = ciphertext
		current.AppSecretFingerprint = fingerprint
	} else {
		current.AppSecretCiphertext = ""
		current.AppSecretFingerprint = ""
	}
	if err := s.repository.UpdateConfig(ctx, current); err != nil {
		return nil, err
	}
	s.notifyRuntime()
	updated, err := s.repository.GetConfig(ctx, req.SpaceID, req.ConfigID)
	if err != nil {
		return nil, err
	}
	return s.configView(ctx, updated), nil
}

func (s *Service) SetEnabled(
	ctx context.Context,
	actorID, spaceID, configID int64,
	enabled bool,
) (*ConfigView, error) {
	if err := s.authorizeManage(ctx, actorID, spaceID); err != nil {
		return nil, err
	}
	config, err := s.repository.GetConfig(ctx, spaceID, configID)
	if err != nil {
		return nil, err
	}
	if enabled {
		if config.AppSecretCiphertext == "" || config.AgentID <= 0 || s.codec == nil {
			return nil, domain.ErrCredentialCodecMissing
		}
		if _, err := s.agents.Resolve(ctx, spaceID, config.AgentID); err != nil {
			return nil, domain.ErrAgentUnavailable
		}
	}
	status := domain.RuntimeStatusDisabled
	if enabled {
		status = domain.RuntimeStatusPending
	}
	if err := s.repository.SetEnabled(ctx, spaceID, configID, actorID, enabled, status); err != nil {
		return nil, err
	}
	s.notifyRuntime()
	updated, err := s.repository.GetConfig(ctx, spaceID, configID)
	if err != nil {
		return nil, err
	}
	return s.configView(ctx, updated), nil
}

func (s *Service) Delete(ctx context.Context, actorID, spaceID, configID int64) error {
	if err := s.authorizeManage(ctx, actorID, spaceID); err != nil {
		return err
	}
	if _, err := s.repository.GetConfig(ctx, spaceID, configID); err != nil {
		return err
	}
	if err := s.repository.DeleteConfig(ctx, spaceID, configID, actorID); err != nil {
		return err
	}
	s.notifyRuntime()
	return nil
}

func (s *Service) TestConnection(
	ctx context.Context,
	actorID, spaceID, configID int64,
) (*BotIdentity, error) {
	if err := s.authorizeManage(ctx, actorID, spaceID); err != nil {
		return nil, err
	}
	config, err := s.repository.GetConfig(ctx, spaceID, configID)
	if err != nil {
		return nil, err
	}
	if s.codec == nil {
		return nil, domain.ErrCredentialCodecMissing
	}
	secret, err := s.codec.Decrypt(config.ID, config.AppSecretCiphertext)
	if err != nil {
		return nil, domain.ErrCredentialCodecMissing
	}
	testCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	identity, err := s.tester.Test(testCtx, config.AppID, secret)
	if err != nil {
		return nil, domain.ErrConnectionTestFailed
	}
	if identity == nil || identity.OpenID == "" {
		return nil, domain.ErrConnectionTestFailed
	}
	_ = s.repository.MarkConnectionTest(ctx, config.ID, identity.OpenID, identity.Name, time.Now())
	return identity, nil
}

func (s *Service) authorizeManage(ctx context.Context, actorID, spaceID int64) error {
	role, err := s.resolveRole(ctx, actorID, spaceID)
	if err != nil {
		return err
	}
	if role != spaceRoleOwner && role != spaceRoleAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) resolveRole(ctx context.Context, actorID, spaceID int64) (int32, error) {
	if s == nil || s.roles == nil || actorID <= 0 {
		return 0, domain.ErrUnauthenticated
	}
	if spaceID <= 0 {
		return 0, domain.ErrInvalidInput
	}
	spaces, err := s.roles.GetUserSpaceList(ctx, actorID)
	if err != nil {
		return 0, err
	}
	for _, space := range spaces {
		if space != nil && space.ID == spaceID {
			return space.RoleType, nil
		}
	}
	return 0, domain.ErrForbidden
}

func (s *Service) configView(ctx context.Context, config *domain.Config) *ConfigView {
	if config == nil {
		return nil
	}
	agentName := ""
	if target, err := s.agents.Resolve(ctx, config.SpaceID, config.AgentID); err == nil && target != nil {
		agentName = target.Name
	}
	return &ConfigView{
		ID:               config.ID,
		SpaceID:          config.SpaceID,
		CreatorID:        config.CreatorID,
		AgentID:          config.AgentID,
		AgentName:        agentName,
		ChannelType:      config.ChannelType,
		Name:             config.Name,
		AppID:            config.AppID,
		SecretConfigured: config.AppSecretCiphertext != "",
		Enabled:          config.Enabled,
		ReplyMode:        config.ReplyMode,
		GroupPolicy:      config.GroupPolicy,
		RuntimeStatus:    config.RuntimeStatus,
		RuntimeError:     boundedMessage(config.RuntimeError, 300),
		BotOpenID:        config.BotOpenID,
		BotName:          config.BotName,
		LastConnectedAt:  config.LastConnectedAt,
		LastTestedAt:     config.LastTestedAt,
		CreatedAt:        config.CreatedAt,
		UpdatedAt:        config.UpdatedAt,
	}
}

func validateMutation(
	name, appID, appSecret string,
	agentID int64,
	replyMode domain.ReplyMode,
	groupPolicy domain.GroupPolicy,
	secretRequired bool,
) error {
	name = strings.TrimSpace(name)
	appID = strings.TrimSpace(appID)
	appSecret = strings.TrimSpace(appSecret)
	if name == "" || len(name) > maxConfigNameBytes ||
		appID == "" || len(appID) > maxAppIDBytes ||
		agentID <= 0 ||
		!domain.ValidReplyMode(replyMode) ||
		!domain.ValidGroupPolicy(groupPolicy) {
		return domain.ErrInvalidInput
	}
	if (secretRequired && appSecret == "") || len(appSecret) > maxAppSecretBytes {
		return domain.ErrInvalidInput
	}
	return nil
}

func (s *Service) notifyRuntime() {
	if s != nil && s.wake != nil {
		s.wake()
	}
}

func boundedMessage(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func IsKnownError(err error) bool {
	return errors.Is(err, domain.ErrUnauthenticated) ||
		errors.Is(err, domain.ErrForbidden) ||
		errors.Is(err, domain.ErrInvalidInput) ||
		errors.Is(err, domain.ErrNotFound) ||
		errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrCredentialCodecMissing) ||
		errors.Is(err, domain.ErrAgentUnavailable) ||
		errors.Is(err, domain.ErrConnectionTestFailed) ||
		errors.Is(err, domain.ErrRuntimeUnavailable)
}

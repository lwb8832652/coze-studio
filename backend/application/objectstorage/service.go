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

package objectstorage

import (
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	storageconfig "github.com/coze-dev/coze-studio/backend/infra/storage/config"
	storageimpl "github.com/coze-dev/coze-studio/backend/infra/storage/impl"
)

const readinessOKCode = "OK"

type Repository interface {
	Count(context.Context) (int64, error)
	List(context.Context) ([]domain.Config, error)
	Get(context.Context, uint64) (*domain.Config, error)
	GetActive(context.Context) (*domain.Config, error)
	CreateWithCredential(context.Context, domain.Config, domain.CredentialInput, storageconfig.CredentialEncryptor) (*domain.Config, error)
	Update(context.Context, storageconfig.UpdateConfigInput) (*domain.Config, error)
	UpdateHealth(context.Context, uint64, domain.Health) error
	Activate(context.Context, uint64, uint64) (*domain.Config, error)
	Delete(context.Context, uint64, uint64, domain.RuntimeDescriptor) error
}

type ServiceComponents struct {
	Repository Repository
	Codec      CredentialCodec
	Registry   ProviderRegistry
	Runtime    func() domain.RuntimeDescriptor
	AllowHTTP  bool
}

type Service struct {
	repository Repository
	codec      CredentialCodec
	registry   ProviderRegistry
	runtime    func() domain.RuntimeDescriptor
	allowHTTP  bool
}

var SVC *Service

func SetDefaultService(service *Service) {
	SVC = service
}

type CreateRequest struct {
	Name         string
	ProviderType domain.ProviderType
	PublicConfig domain.PublicConfig
	Credential   domain.CredentialInput
}

type UpdateRequest struct {
	ID              uint64
	ExpectedVersion uint64
	Name            string
	PublicConfig    domain.PublicConfig
	Credential      domain.CredentialInput
}

type TestRequest struct {
	ID              uint64
	ExpectedVersion uint64
	ProviderType    domain.ProviderType
	PublicConfig    domain.PublicConfig
	Credential      domain.CredentialInput
}

type ActivateRequest struct {
	ID                 uint64
	ExpectedVersion    uint64
	MigrationConfirmed bool
}

type DeleteRequest struct {
	ID              uint64
	ExpectedVersion uint64
}

type ConfigView struct {
	ID                   uint64
	Name                 string
	ProviderType         domain.ProviderType
	PublicConfig         domain.PublicConfig
	CredentialConfigured bool
	Health               domain.Health
	DesiredActive        bool
	RuntimeActive        bool
	RestartRequired      bool
	Version              uint64
	RuntimeRevision      uint64
	CreatedAt            string
	UpdatedAt            string
}

type ListResult struct {
	Configs         []ConfigView
	RuntimeSource   domain.RuntimeSource
	RestartRequired bool
}

type TestResult struct {
	Success bool
	Health  domain.Health
}

func NewService(c ServiceComponents) *Service {
	registry := c.Registry
	if registry == nil {
		registry = storageimpl.DefaultRegistry()
	}
	runtime := c.Runtime
	if runtime == nil {
		runtime = storageimpl.CurrentRuntimeDescriptor
	}
	return &Service{
		repository: c.Repository,
		codec:      c.Codec,
		registry:   registry,
		runtime:    runtime,
		allowHTTP:  c.AllowHTTP,
	}
}

func (s *Service) List(ctx context.Context) (*ListResult, error) {
	if s == nil || s.repository == nil {
		return nil, domain.ErrConfigInvalid
	}
	configs, err := s.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	runtime := s.currentRuntime()
	result := &ListResult{
		Configs:         make([]ConfigView, 0, len(configs)),
		RuntimeSource:   runtime.Source,
		RestartRequired: true,
	}
	for index := range configs {
		config := configs[index]
		if config.Active {
			result.RestartRequired = domain.RestartRequired(runtime, &config)
		}
		result.Configs = append(result.Configs, configView(config, runtime))
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, request CreateRequest) (*ConfigView, error) {
	if s == nil || s.repository == nil {
		return nil, domain.ErrConfigInvalid
	}
	if s.codec == nil {
		return nil, domain.ErrCredentialUnavailable
	}
	name, publicConfig, err := s.validateMutationFields(request.Name, request.ProviderType, request.PublicConfig)
	if err != nil {
		return nil, err
	}
	credential, err := requireCredentialPair(request.Credential)
	if err != nil {
		return nil, err
	}
	config, err := s.repository.CreateWithCredential(ctx, domain.Config{
		Name:         name,
		ProviderType: request.ProviderType,
		PublicConfig: publicConfig,
		Active:       false,
	}, credential, s.codec)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, domain.ErrNotFound
	}
	view := configView(*config, s.currentRuntime())
	return &view, nil
}

func (s *Service) Update(ctx context.Context, request UpdateRequest) (*ConfigView, error) {
	if s == nil || s.repository == nil {
		return nil, domain.ErrConfigInvalid
	}
	current, err := s.repository.Get(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, domain.ErrNotFound
	}
	name, publicConfig, err := s.validateMutationFields(request.Name, current.ProviderType, request.PublicConfig)
	if err != nil {
		return nil, err
	}
	credential := domain.NormalizeCredentialInput(request.Credential)
	if err := domain.ValidateCredentialInput(credential); err != nil {
		return nil, err
	}

	credentialSecret := current.CredentialSecret
	if domain.HasCredentialPair(credential) {
		if s.codec == nil {
			return nil, domain.ErrCredentialUnavailable
		}
		credentialSecret, err = s.codec.Encrypt(current.ID, current.ProviderType, storageconfig.CredentialAADVersion, credential)
		if err != nil {
			return nil, err
		}
	}

	next := *current
	next.Name = name
	next.PublicConfig = publicConfig
	next.CredentialSecret = credentialSecret
	updated, err := s.repository.Update(ctx, storageconfig.UpdateConfigInput{
		ID:               current.ID,
		ExpectedVersion:  request.ExpectedVersion,
		Name:             name,
		PublicConfig:     publicConfig,
		CredentialSecret: credentialSecret,
		RuntimeChanged:   !domain.RuntimeFieldsEqual(*current, next),
	})
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, domain.ErrNotFound
	}
	view := configView(*updated, s.currentRuntime())
	return &view, nil
}

func (s *Service) Test(ctx context.Context, request TestRequest) (*TestResult, error) {
	if s == nil || s.repository == nil {
		return nil, domain.ErrConfigInvalid
	}
	target, provider, publicConfig, credential, persistHealth, err := s.testInput(ctx, request)
	if err != nil {
		return nil, err
	}
	health, readinessErr := s.checkReadiness(ctx, provider, publicConfig, credential, 0, 0)
	if target != nil && persistHealth {
		if err := s.repository.UpdateHealth(ctx, target.ID, health); err != nil {
			return nil, err
		}
	}
	return &TestResult{
		Success: readinessErr == nil,
		Health:  health,
	}, nil
}

func (s *Service) Activate(ctx context.Context, request ActivateRequest) (*ConfigView, error) {
	if s == nil || s.repository == nil {
		return nil, domain.ErrConfigInvalid
	}
	target, err := s.repository.Get(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, domain.ErrNotFound
	}
	if request.ExpectedVersion != 0 && target.Version != request.ExpectedVersion {
		return nil, domain.ErrVersionConflict
	}
	if target.Active {
		view := configView(*target, s.currentRuntime())
		return &view, nil
	}
	if !request.MigrationConfirmed {
		return nil, domain.ErrMigrationConfirmation
	}
	credential, err := s.decryptCredential(target)
	if err != nil {
		return nil, err
	}
	health, readinessErr := s.checkReadiness(
		ctx,
		target.ProviderType,
		target.PublicConfig,
		credential,
		target.ID,
		target.RuntimeRevision,
	)
	if readinessErr != nil {
		if updateErr := s.repository.UpdateHealth(ctx, target.ID, health); updateErr != nil {
			return nil, updateErr
		}
		return nil, readinessErr
	}
	if err := s.repository.UpdateHealth(ctx, target.ID, health); err != nil {
		return nil, err
	}
	activated, err := s.repository.Activate(ctx, target.ID, request.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	if activated == nil {
		return nil, domain.ErrNotFound
	}
	view := configView(*activated, s.currentRuntime())
	return &view, nil
}

func (s *Service) Delete(ctx context.Context, request DeleteRequest) error {
	if s == nil || s.repository == nil {
		return domain.ErrConfigInvalid
	}
	return s.repository.Delete(ctx, request.ID, request.ExpectedVersion, s.currentRuntime())
}

func (s *Service) testInput(ctx context.Context, request TestRequest) (*domain.Config, domain.ProviderType, domain.PublicConfig, domain.CredentialInput, bool, error) {
	provider := request.ProviderType
	var target *domain.Config
	if request.ID != 0 {
		config, err := s.repository.Get(ctx, request.ID)
		if err != nil {
			return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false, err
		}
		if config == nil {
			return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false, domain.ErrNotFound
		}
		if request.ExpectedVersion != 0 && config.Version != request.ExpectedVersion {
			return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false, domain.ErrVersionConflict
		}
		target = config
		if provider == "" {
			provider = config.ProviderType
		}
	}
	publicConfig, err := domain.ValidatePublicConfig(provider, request.PublicConfig, domain.ValidationMode{AllowHTTP: s.allowHTTP})
	if err != nil {
		return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false, err
	}
	credential := domain.NormalizeCredentialInput(request.Credential)
	if err := domain.ValidateCredentialInput(credential); err != nil {
		return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false, err
	}
	persistHealth := false
	if !domain.HasCredentialPair(credential) {
		if target == nil {
			return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false,
				errors.Join(domain.ErrConfigInvalid, errors.New("credential pair is required"))
		}
		credential, err = s.decryptCredential(target)
		if err != nil {
			return nil, "", domain.PublicConfig{}, domain.CredentialInput{}, false, err
		}
		persistHealth = provider == target.ProviderType && publicConfig == target.PublicConfig
	}
	return target, provider, publicConfig, credential, persistHealth, nil
}

func (s *Service) validateMutationFields(name string, provider domain.ProviderType, publicConfig domain.PublicConfig) (string, domain.PublicConfig, error) {
	normalizedName, err := domain.NormalizeName(name)
	if err != nil {
		return "", domain.PublicConfig{}, err
	}
	normalizedConfig, err := domain.ValidatePublicConfig(provider, publicConfig, domain.ValidationMode{AllowHTTP: s.allowHTTP})
	if err != nil {
		return "", domain.PublicConfig{}, err
	}
	return normalizedName, normalizedConfig, nil
}

func (s *Service) decryptCredential(config *domain.Config) (domain.CredentialInput, error) {
	if config == nil {
		return domain.CredentialInput{}, domain.ErrNotFound
	}
	if s.codec == nil || config.CredentialSecret == "" {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	return s.codec.Decrypt(config.ID, config.ProviderType, storageconfig.CredentialAADVersion, config.CredentialSecret)
}

func (s *Service) checkReadiness(
	ctx context.Context,
	provider domain.ProviderType,
	publicConfig domain.PublicConfig,
	credential domain.CredentialInput,
	configID uint64,
	runtimeRevision uint64,
) (domain.Health, error) {
	start := time.Now()
	checkedAt := start.UTC()
	health := domain.Health{
		Status:    domain.HealthHealthy,
		Code:      readinessOKCode,
		Message:   readinessOKCode,
		CheckedAt: &checkedAt,
	}
	client, err := s.buildStorage(ctx, storageimpl.BuildInput{
		ProviderType:    provider,
		PublicConfig:    publicConfig,
		Credential:      credential,
		ConfigID:        configID,
		RuntimeRevision: runtimeRevision,
	})
	if err == nil {
		checker, ok := client.(storage.ReadinessChecker)
		if !ok {
			err = storage.ErrReadinessUnavailable
		} else {
			err = checker.CheckReadiness(ctx)
		}
	}
	elapsed := time.Since(start).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > int64(^uint32(0)) {
		elapsed = int64(^uint32(0))
	}
	health.LatencyMS = uint32(elapsed)
	if err == nil {
		return health, nil
	}
	stableErr := readinessError(err)
	health.Status = domain.HealthUnhealthy
	health.Code = domain.ErrorCodeOf(stableErr)
	health.Message = health.Code
	return health, stableErr
}

func (s *Service) buildStorage(ctx context.Context, input storageimpl.BuildInput) (storage.Storage, error) {
	registry := s.registry
	if registry == nil {
		registry = storageimpl.DefaultRegistry()
	}
	client, err := registry.New(ctx, input)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, domain.ErrConnectionFailed
	}
	return client, nil
}

func (s *Service) currentRuntime() domain.RuntimeDescriptor {
	if s == nil || s.runtime == nil {
		return domain.RuntimeDescriptor{}
	}
	return s.runtime()
}

func requireCredentialPair(input domain.CredentialInput) (domain.CredentialInput, error) {
	credential := domain.NormalizeCredentialInput(input)
	if err := domain.ValidateCredentialInput(credential); err != nil {
		return domain.CredentialInput{}, err
	}
	if !domain.HasCredentialPair(credential) {
		return domain.CredentialInput{}, errors.Join(domain.ErrConfigInvalid, errors.New("credential pair is required"))
	}
	return credential, nil
}

func readinessError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrConfigInvalid),
		errors.Is(err, domain.ErrProviderUnsupported),
		errors.Is(err, domain.ErrCredentialUnavailable):
		return err
	default:
		return fmt.Errorf("%w: %s", domain.ErrConnectionFailed, domain.ErrorCodeOf(domain.ErrConnectionFailed))
	}
}

func configView(config domain.Config, runtime domain.RuntimeDescriptor) ConfigView {
	runtimeActive := runtime.Source == domain.RuntimeSourceDatabase &&
		runtime.ConfigID == config.ID &&
		runtime.RuntimeRevision == config.RuntimeRevision &&
		runtime.ProviderType == config.ProviderType
	return ConfigView{
		ID:                   config.ID,
		Name:                 config.Name,
		ProviderType:         config.ProviderType,
		PublicConfig:         config.PublicConfig,
		CredentialConfigured: config.CredentialSecret != "",
		Health:               config.Health,
		DesiredActive:        config.Active,
		RuntimeActive:        runtimeActive,
		RestartRequired:      config.Active && domain.RestartRequired(runtime, &config),
		Version:              config.Version,
		RuntimeRevision:      config.RuntimeRevision,
		CreatedAt:            formatTime(config.CreatedAt),
		UpdatedAt:            formatTime(config.UpdatedAt),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

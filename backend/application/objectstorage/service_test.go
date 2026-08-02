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
	"io"
	"testing"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	storageconfig "github.com/coze-dev/coze-studio/backend/infra/storage/config"
	storageimpl "github.com/coze-dev/coze-studio/backend/infra/storage/impl"
)

func TestServiceCreateEncryptsCredentialAndDoesNotReturnSecret(t *testing.T) {
	h := newServiceHarness()

	created, err := h.service.Create(context.Background(), CreateRequest{
		Name:         "qiniu-prod",
		ProviderType: domain.ProviderQiniu,
		PublicConfig: domain.PublicConfig{Bucket: "coze", DownloadDomain: "cdn.example.com", UseHTTPS: true},
		Credential:   domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !created.CredentialConfigured {
		t.Fatalf("created view did not report configured credential: %+v", created)
	}
	if len(h.repository.created) != 1 || h.repository.created[0].CredentialSecret == "" {
		t.Fatalf("created configs = %+v", h.repository.created)
	}
	if h.codec.encryptCredential.AccessKeyID != "ak" || h.codec.encryptVersion != storageconfig.CredentialAADVersion {
		t.Fatalf("codec encrypt credential = %+v version=%d", h.codec.encryptCredential, h.codec.encryptVersion)
	}
}

func TestServiceUpdatePreservesCredentialWhenBothFieldsEmpty(t *testing.T) {
	h := newServiceHarness()
	existing := h.repository.seed(validDomainConfig(1, "minio", true))

	updated, err := h.service.Update(context.Background(), UpdateRequest{
		ID:              existing.ID,
		ExpectedVersion: existing.Version,
		Name:            "renamed",
		PublicConfig:    existing.PublicConfig,
		Credential:      domain.CredentialInput{},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Version != existing.Version+1 || h.codec.encryptCalls != 0 {
		t.Fatalf("update result = %+v encrypt calls = %d", updated, h.codec.encryptCalls)
	}
	if h.repository.lastUpdate.CredentialSecret != existing.CredentialSecret {
		t.Fatalf("updated credential secret = %q, want preserved", h.repository.lastUpdate.CredentialSecret)
	}
}

func TestServiceUpdateRejectsIncompleteCredentialPair(t *testing.T) {
	h := newServiceHarness()
	existing := h.repository.seed(validDomainConfig(1, "minio", true))

	_, err := h.service.Update(context.Background(), UpdateRequest{
		ID:              existing.ID,
		ExpectedVersion: existing.Version,
		Name:            existing.Name,
		PublicConfig:    existing.PublicConfig,
		Credential:      domain.CredentialInput{AccessKeyID: "ak-only"},
	})
	if !errors.Is(err, domain.ErrConfigInvalid) {
		t.Fatalf("Update(single credential) error = %v", err)
	}
}

func TestServiceTestDraftDoesNotPersistHealthWhenDraftDiffers(t *testing.T) {
	h := newServiceHarness()
	existing := h.repository.seed(validDomainConfig(1, "minio", true))
	h.codec.decryptCredential = domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}

	result, err := h.service.Test(context.Background(), TestRequest{
		ID:              existing.ID,
		ExpectedVersion: existing.Version,
		ProviderType:    existing.ProviderType,
		PublicConfig:    domain.PublicConfig{Bucket: "other", Endpoint: "minio:9000", UseSSL: false},
		Credential:      domain.CredentialInput{},
	})
	if err != nil || !result.Success {
		t.Fatalf("Test() result = %+v error = %v", result, err)
	}
	if h.repository.healthUpdates != 0 {
		t.Fatalf("draft test persisted health %d times", h.repository.healthUpdates)
	}
}

func TestServiceTestRejectsStaleHealthAfterConcurrentConfigUpdate(t *testing.T) {
	h := newServiceHarness()
	existing := h.repository.seed(validDomainConfig(1, "minio", true))
	h.codec.decryptCredential = domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}
	h.readiness.started = make(chan struct{})
	h.readiness.release = make(chan struct{})

	type testOutcome struct {
		result *TestResult
		err    error
	}
	done := make(chan testOutcome, 1)
	go func() {
		result, err := h.service.Test(context.Background(), TestRequest{
			ID:              existing.ID,
			ExpectedVersion: existing.Version,
			ProviderType:    existing.ProviderType,
			PublicConfig:    existing.PublicConfig,
		})
		done <- testOutcome{result: result, err: err}
	}()

	<-h.readiness.started
	updated := h.repository.configs[existing.ID]
	updated.Version++
	updated.RuntimeRevision++
	updated.PublicConfig.Bucket = "concurrently-updated"
	h.repository.configs[existing.ID] = updated
	close(h.readiness.release)

	outcome := <-done
	if !errors.Is(outcome.err, domain.ErrVersionConflict) {
		t.Fatalf("Test(concurrent update) result=%+v error=%v", outcome.result, outcome.err)
	}
	if h.repository.healthUpdates != 0 || h.repository.configs[existing.ID].Health.Status != domain.HealthUnknown {
		t.Fatalf("stale health persisted: updates=%d config=%+v", h.repository.healthUpdates, h.repository.configs[existing.ID])
	}
}

func TestServiceActivateRequiresMigrationConfirmationAndRollsBackOnReadinessFailure(t *testing.T) {
	h := newServiceHarness()
	target := h.repository.seed(validDomainConfig(2, "target", false))
	h.codec.decryptCredential = domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}

	_, err := h.service.Activate(context.Background(), ActivateRequest{ID: target.ID, ExpectedVersion: target.Version})
	if !errors.Is(err, domain.ErrMigrationConfirmation) {
		t.Fatalf("missing confirmation error = %v", err)
	}

	h.readiness.err = domain.ErrConnectionFailed
	_, err = h.service.Activate(context.Background(), ActivateRequest{ID: target.ID, ExpectedVersion: target.Version, MigrationConfirmed: true})
	if !errors.Is(err, domain.ErrConnectionFailed) || h.repository.activateCalls != 0 {
		t.Fatalf("readiness failure error = %v activate calls = %d", err, h.repository.activateCalls)
	}
}

func TestServiceDeleteUsesRuntimeDescriptor(t *testing.T) {
	h := newServiceHarness()
	existing := h.repository.seed(validDomainConfig(7, "runtime", false))
	h.runtime = domain.RuntimeDescriptor{Source: domain.RuntimeSourceDatabase, ConfigID: existing.ID, RuntimeRevision: existing.RuntimeRevision, ProviderType: existing.ProviderType}

	err := h.service.Delete(context.Background(), DeleteRequest{ID: existing.ID, ExpectedVersion: existing.Version})
	if !errors.Is(err, domain.ErrActiveDeleteForbidden) {
		t.Fatalf("Delete(runtime active) error = %v", err)
	}
}

type serviceHarness struct {
	repository *fakeServiceRepository
	codec      *fakeServiceCodec
	registry   *fakeServiceRegistry
	readiness  *fakeReadinessStorage
	runtime    domain.RuntimeDescriptor
	service    *Service
}

func newServiceHarness() *serviceHarness {
	repository := &fakeServiceRepository{configs: map[uint64]domain.Config{}}
	codec := &fakeServiceCodec{}
	readiness := &fakeReadinessStorage{}
	registry := &fakeServiceRegistry{storage: readiness}
	h := &serviceHarness{
		repository: repository,
		codec:      codec,
		registry:   registry,
		readiness:  readiness,
	}
	h.service = NewService(ServiceComponents{
		Repository: repository,
		Codec:      codec,
		Registry:   registry,
		Runtime: func() domain.RuntimeDescriptor {
			return h.runtime
		},
		AllowHTTP: true,
	})
	return h
}

type fakeServiceRepository struct {
	configs       map[uint64]domain.Config
	created       []domain.Config
	lastUpdate    storageconfig.UpdateConfigInput
	healthUpdates int
	activateCalls int
	nextID        uint64
}

func (r *fakeServiceRepository) Count(context.Context) (int64, error) {
	return int64(len(r.configs)), nil
}

func (r *fakeServiceRepository) List(context.Context) ([]domain.Config, error) {
	configs := make([]domain.Config, 0, len(r.configs))
	for _, config := range r.configs {
		configs = append(configs, config)
	}
	return configs, nil
}

func (r *fakeServiceRepository) Get(_ context.Context, id uint64) (*domain.Config, error) {
	config, ok := r.configs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &config, nil
}

func (r *fakeServiceRepository) GetActive(context.Context) (*domain.Config, error) {
	for _, config := range r.configs {
		if config.Active {
			return &config, nil
		}
	}
	return nil, domain.ErrPrimaryConfigMissing
}

func (r *fakeServiceRepository) CreateWithCredential(_ context.Context, config domain.Config, credential domain.CredentialInput, codec storageconfig.CredentialEncryptor) (*domain.Config, error) {
	if r.nextID == 0 {
		r.nextID = 1
	}
	config.ID = r.nextID
	r.nextID++
	if config.Version == 0 {
		config.Version = 1
	}
	if config.RuntimeRevision == 0 {
		config.RuntimeRevision = 1
	}
	now := time.Now().UTC()
	config.CreatedAt = now
	config.UpdatedAt = now
	secret, err := codec.Encrypt(config.ID, config.ProviderType, storageconfig.CredentialAADVersion, credential)
	if err != nil {
		return nil, err
	}
	config.CredentialSecret = secret
	r.configs[config.ID] = config
	r.created = append(r.created, config)
	return &config, nil
}

func (r *fakeServiceRepository) Update(_ context.Context, input storageconfig.UpdateConfigInput) (*domain.Config, error) {
	current, ok := r.configs[input.ID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if current.Version != input.ExpectedVersion {
		return nil, domain.ErrVersionConflict
	}
	current.Name = input.Name
	current.PublicConfig = input.PublicConfig
	current.CredentialSecret = input.CredentialSecret
	current.Version++
	if input.RuntimeChanged {
		current.RuntimeRevision++
	}
	current.UpdatedAt = time.Now().UTC()
	r.configs[current.ID] = current
	r.lastUpdate = input
	return &current, nil
}

func (r *fakeServiceRepository) UpdateHealth(_ context.Context, input storageconfig.UpdateHealthInput) error {
	current, ok := r.configs[input.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if current.Version != input.ExpectedVersion || current.RuntimeRevision != input.ExpectedRuntimeRevision {
		return domain.ErrVersionConflict
	}
	current.Health = input.Health
	r.configs[input.ID] = current
	r.healthUpdates++
	return nil
}

func (r *fakeServiceRepository) Activate(_ context.Context, id uint64, expectedVersion uint64) (*domain.Config, error) {
	target, ok := r.configs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if target.Version != expectedVersion {
		return nil, domain.ErrVersionConflict
	}
	for configID, config := range r.configs {
		config.Active = configID == id
		if configID == id {
			config.Version++
			config.UpdatedAt = time.Now().UTC()
			target = config
		}
		r.configs[configID] = config
	}
	r.activateCalls++
	return &target, nil
}

func (r *fakeServiceRepository) Delete(_ context.Context, id uint64, expectedVersion uint64, runtime domain.RuntimeDescriptor) error {
	current, ok := r.configs[id]
	if !ok {
		return domain.ErrNotFound
	}
	if current.Version != expectedVersion {
		return domain.ErrVersionConflict
	}
	if current.Active || runtime.Source == domain.RuntimeSourceDatabase && runtime.ConfigID == id {
		return domain.ErrActiveDeleteForbidden
	}
	delete(r.configs, id)
	return nil
}

func (r *fakeServiceRepository) seed(config domain.Config) domain.Config {
	r.configs[config.ID] = config
	if config.ID >= r.nextID {
		r.nextID = config.ID + 1
	}
	return config
}

type fakeServiceCodec struct {
	encryptCalls      int
	encryptCredential domain.CredentialInput
	encryptVersion    uint64
	decryptCredential domain.CredentialInput
}

func (c *fakeServiceCodec) Encrypt(_ uint64, _ domain.ProviderType, version uint64, input domain.CredentialInput) (string, error) {
	c.encryptCalls++
	c.encryptCredential = input
	c.encryptVersion = version
	return "encrypted", nil
}

func (c *fakeServiceCodec) Decrypt(uint64, domain.ProviderType, uint64, string) (domain.CredentialInput, error) {
	if c.decryptCredential.AccessKeyID == "" && c.decryptCredential.SecretAccessKey == "" {
		return domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}, nil
	}
	return c.decryptCredential, nil
}

type fakeServiceRegistry struct {
	input   storageimpl.BuildInput
	storage storage.Storage
	err     error
}

func (r *fakeServiceRegistry) New(_ context.Context, input storageimpl.BuildInput) (storage.Storage, error) {
	r.input = input
	return r.storage, r.err
}

type fakeReadinessStorage struct {
	err     error
	started chan struct{}
	release chan struct{}
}

func (s *fakeReadinessStorage) CheckReadiness(ctx context.Context) error {
	if s.started != nil {
		close(s.started)
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.err
}

func (s *fakeReadinessStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error {
	return nil
}

func (s *fakeReadinessStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}

func (s *fakeReadinessStorage) GetObject(context.Context, string) ([]byte, error) { return nil, nil }

func (s *fakeReadinessStorage) DeleteObject(context.Context, string) error { return nil }

func (s *fakeReadinessStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "", nil
}

func (s *fakeReadinessStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, nil
}

func (s *fakeReadinessStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, nil
}

func (s *fakeReadinessStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return nil, nil
}

func validDomainConfig(id uint64, name string, active bool) domain.Config {
	now := time.Now().UTC()
	return domain.Config{
		ID:               id,
		Name:             name,
		ProviderType:     domain.ProviderMinIO,
		PublicConfig:     domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000", UseSSL: false},
		CredentialSecret: "encrypted",
		Active:           active,
		Health:           domain.Health{Status: domain.HealthUnknown},
		Version:          1,
		RuntimeRevision:  1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

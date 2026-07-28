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

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	storageconfig "github.com/coze-dev/coze-studio/backend/infra/storage/config"
	storageimpl "github.com/coze-dev/coze-studio/backend/infra/storage/impl"
)

func TestBootstrapImportsLegacyEnvWhenTableEmpty(t *testing.T) {
	h := newBootstrapHarness()
	h.bootstrap.Getenv = mapGetenv(map[string]string{
		storageconfig.ObjectStorageConfigSourceEnv: storageconfig.ObjectStorageSourceDatabase,
		"STORAGE_TYPE":   "minio",
		"MINIO_ENDPOINT": "minio:9000",
		"MINIO_AK":       "ak",
		"MINIO_SK":       "sk",
		"STORAGE_BUCKET": "coze",
		"MINIO_USE_SSL":  "false",
	})
	h.bootstrap.AllowHTTP = true

	result, err := h.bootstrap.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if result.RuntimeDescriptor.Source != domain.RuntimeSourceDatabase || result.RuntimeDescriptor.ConfigID == 0 {
		t.Fatalf("runtime descriptor = %+v", result.RuntimeDescriptor)
	}
	if len(h.repository.created) != 1 || !h.repository.created[0].Active {
		t.Fatalf("created configs = %+v", h.repository.created)
	}
	if h.repository.credentials[0].AccessKeyID != "ak" || h.codec.encryptID == 0 {
		t.Fatalf("credential import failed: %+v codec id=%d", h.repository.credentials[0], h.codec.encryptID)
	}
	if h.registry.input.ConfigID != result.RuntimeDescriptor.ConfigID ||
		h.registry.input.ProviderType != domain.ProviderMinIO {
		t.Fatalf("registry input = %+v", h.registry.input)
	}
}

func TestBootstrapLoadsDatabaseActiveConfig(t *testing.T) {
	h := newBootstrapHarness()
	h.repository.count = 1
	h.repository.active = &domain.Config{
		ID:               7,
		Name:             "primary",
		ProviderType:     domain.ProviderQiniu,
		PublicConfig:     domain.PublicConfig{Bucket: "coze", DownloadDomain: "cdn.example.com", UseHTTPS: true},
		CredentialSecret: "encrypted",
		Active:           true,
		Version:          3,
		RuntimeRevision:  5,
	}
	h.codec.decryptCredential = domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}

	result, err := h.bootstrap.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap(database) error = %v", err)
	}
	if result.RuntimeDescriptor.Source != domain.RuntimeSourceDatabase ||
		result.RuntimeDescriptor.ConfigID != 7 ||
		result.RuntimeDescriptor.RuntimeRevision != 5 ||
		result.RuntimeDescriptor.ProviderType != domain.ProviderQiniu {
		t.Fatalf("runtime descriptor = %+v", result.RuntimeDescriptor)
	}
	if h.registry.input.Credential.AccessKeyID != "ak" || h.codec.decryptID != 7 {
		t.Fatalf("registry input = %+v codec decrypt id=%d", h.registry.input, h.codec.decryptID)
	}
}

func TestBootstrapFailsClosedWhenDatabaseModeHasNoActiveConfig(t *testing.T) {
	h := newBootstrapHarness()
	h.repository.count = 1
	h.repository.activeErr = domain.ErrPrimaryConfigMissing

	_, err := h.bootstrap.Bootstrap(context.Background())
	if !errors.Is(err, domain.ErrPrimaryConfigMissing) {
		t.Fatalf("Bootstrap() error = %v", err)
	}
}

func TestBootstrapEnvRescueBypassesDatabaseAndCodec(t *testing.T) {
	h := newBootstrapHarness()
	h.bootstrap.Codec = nil
	h.bootstrap.Getenv = mapGetenv(map[string]string{
		storageconfig.ObjectStorageConfigSourceEnv: storageconfig.ObjectStorageSourceEnv,
		"STORAGE_TYPE":   "minio",
		"MINIO_ENDPOINT": "minio:9000",
		"MINIO_AK":       "ak",
		"MINIO_SK":       "sk",
		"STORAGE_BUCKET": "coze",
		"MINIO_USE_SSL":  "false",
	})
	h.bootstrap.AllowHTTP = true

	result, err := h.bootstrap.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap(env) error = %v", err)
	}
	if result.RuntimeDescriptor.Source != domain.RuntimeSourceEnvRescue {
		t.Fatalf("runtime source = %s", result.RuntimeDescriptor.Source)
	}
	if h.repository.getActiveCalls != 0 || len(h.repository.created) != 0 {
		t.Fatalf("rescue touched repository: getActive=%d created=%d", h.repository.getActiveCalls, len(h.repository.created))
	}
}

func TestBootstrapNilReceiverReturnsConfigError(t *testing.T) {
	_, err := ((*Bootstrapper)(nil)).Bootstrap(context.Background())
	if !errors.Is(err, domain.ErrConfigInvalid) {
		t.Fatalf("Bootstrap(nil) error = %v", err)
	}
}

func TestStorageBackedImageXDelegatesToRuntimeStorage(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "not-used")
	runtimeStorage := &fakeBootstrapStorage{signedURL: "https://storage.example.test/object?token=redacted"}
	client := storageimpl.NewStorageBackedImageX(runtimeStorage)

	resourceURL, err := client.GetResourceURL(context.Background(), "object")
	if err != nil {
		t.Fatalf("GetResourceURL() error = %v", err)
	}
	if resourceURL.URL != runtimeStorage.signedURL || runtimeStorage.signedKey != "object" {
		t.Fatalf("resource url = %+v signed key = %s", resourceURL, runtimeStorage.signedKey)
	}
	token, err := client.GetUploadAuth(context.Background())
	if err != nil {
		t.Fatalf("GetUploadAuth() error = %v", err)
	}
	if token.AccessKeyID != "" || token.SecretAccessKey != "" {
		t.Fatal("storage-backed ImageX exposed credentials")
	}
}

type bootstrapHarness struct {
	repository *fakeBootstrapRepository
	codec      *fakeBootstrapCodec
	registry   *fakeBootstrapRegistry
	bootstrap  *Bootstrapper
}

func newBootstrapHarness() *bootstrapHarness {
	repository := &fakeBootstrapRepository{}
	codec := &fakeBootstrapCodec{}
	registry := &fakeBootstrapRegistry{storage: &fakeBootstrapStorage{}}
	return &bootstrapHarness{
		repository: repository,
		codec:      codec,
		registry:   registry,
		bootstrap: &Bootstrapper{
			Repository: repository,
			Codec:      codec,
			Registry:   registry,
			Getenv:     mapGetenv(map[string]string{}),
		},
	}
}

type fakeBootstrapRepository struct {
	count          int64
	countErr       error
	active         *domain.Config
	activeErr      error
	getActiveCalls int
	created        []domain.Config
	credentials    []domain.CredentialInput
}

func (r *fakeBootstrapRepository) Count(context.Context) (int64, error) {
	return r.count, r.countErr
}

func (r *fakeBootstrapRepository) GetActive(context.Context) (*domain.Config, error) {
	r.getActiveCalls++
	if r.activeErr != nil {
		return nil, r.activeErr
	}
	return r.active, nil
}

func (r *fakeBootstrapRepository) CreateWithCredential(_ context.Context, config domain.Config, credential domain.CredentialInput, codec storageconfig.CredentialEncryptor) (*domain.Config, error) {
	if config.ID == 0 {
		config.ID = uint64(len(r.created) + 1)
	}
	if config.Version == 0 {
		config.Version = 1
	}
	if config.RuntimeRevision == 0 {
		config.RuntimeRevision = 1
	}
	secret, err := codec.Encrypt(config.ID, config.ProviderType, config.Version, credential)
	if err != nil {
		return nil, err
	}
	config.CredentialSecret = secret
	r.created = append(r.created, config)
	r.credentials = append(r.credentials, credential)
	return &config, nil
}

type fakeBootstrapCodec struct {
	encryptID         uint64
	decryptID         uint64
	decryptCredential domain.CredentialInput
}

func (c *fakeBootstrapCodec) Encrypt(id uint64, _ domain.ProviderType, _ uint64, _ domain.CredentialInput) (string, error) {
	c.encryptID = id
	return "encrypted", nil
}

func (c *fakeBootstrapCodec) Decrypt(id uint64, _ domain.ProviderType, _ uint64, _ string) (domain.CredentialInput, error) {
	c.decryptID = id
	return c.decryptCredential, nil
}

type fakeBootstrapRegistry struct {
	input   storageimpl.BuildInput
	storage storage.Storage
	err     error
}

func (r *fakeBootstrapRegistry) New(_ context.Context, input storageimpl.BuildInput) (storage.Storage, error) {
	r.input = input
	return r.storage, r.err
}

type fakeBootstrapStorage struct {
	signedURL string
	signedKey string
}

func (s *fakeBootstrapStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error {
	return nil
}

func (s *fakeBootstrapStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}

func (s *fakeBootstrapStorage) GetObject(context.Context, string) ([]byte, error) { return nil, nil }

func (s *fakeBootstrapStorage) DeleteObject(context.Context, string) error { return nil }

func (s *fakeBootstrapStorage) GetObjectUrl(_ context.Context, key string, _ ...storage.GetOptFn) (string, error) {
	s.signedKey = key
	if s.signedURL == "" {
		return "https://storage.example.test/" + key, nil
	}
	return s.signedURL, nil
}

func (s *fakeBootstrapStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, nil
}

func (s *fakeBootstrapStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, nil
}

func (s *fakeBootstrapStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return nil, nil
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

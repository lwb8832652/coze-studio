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

package impl

import (
	"context"
	"errors"
	"io"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	miniopkg "github.com/coze-dev/coze-studio/backend/infra/storage/impl/minio"
	s3pkg "github.com/coze-dev/coze-studio/backend/infra/storage/impl/s3"
	tospkg "github.com/coze-dev/coze-studio/backend/infra/storage/impl/tos"
)

type fakeProviderStorage struct{ readiness error }

func (f fakeProviderStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error {
	return nil
}

func (f fakeProviderStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}

func (f fakeProviderStorage) GetObject(context.Context, string) ([]byte, error) { return nil, nil }

func (f fakeProviderStorage) DeleteObject(context.Context, string) error { return nil }

func (f fakeProviderStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "", nil
}

func (f fakeProviderStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, nil
}

func (f fakeProviderStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, nil
}

func (f fakeProviderStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return nil, nil
}

func (f fakeProviderStorage) CheckReadiness(context.Context) error { return f.readiness }

func TestRegistryBuildsSupportedProviderAndRejectsUnknown(t *testing.T) {
	registry := NewRegistry()
	registry.Register(domain.ProviderMinIO, func(context.Context, BuildInput) (storage.Storage, error) {
		return fakeProviderStorage{}, nil
	})
	_, err := registry.New(context.Background(), BuildInput{
		ProviderType:    domain.ProviderMinIO,
		PublicConfig:    domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000", UseSSL: true},
		Credential:      domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		ConfigID:        9,
		RuntimeRevision: 2,
	})
	if err != nil {
		t.Fatalf("registry.New(minio) error = %v", err)
	}
	_, err = registry.New(context.Background(), BuildInput{ProviderType: domain.ProviderType("unknown")})
	if !errors.Is(err, domain.ErrProviderUnsupported) {
		t.Fatalf("registry.New(unknown) error = %v", err)
	}
}

func TestDefaultRegistryBuildsKnownProvidersAndRejectsDeferredProviders(t *testing.T) {
	registry := DefaultRegistry()
	supported := []BuildInput{
		{
			ProviderType: domain.ProviderMinIO,
			PublicConfig: domain.PublicConfig{
				Bucket: "coze", Endpoint: "127.0.0.1:1", UseSSL: false,
			},
			Credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
		{
			ProviderType: domain.ProviderAWSS3,
			PublicConfig: domain.PublicConfig{
				Bucket: "coze", Region: "us-east-1", Endpoint: "http://127.0.0.1:1", ForcePathStyle: true,
			},
			Credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
		{
			ProviderType: domain.ProviderTOS,
			PublicConfig: domain.PublicConfig{
				Bucket: "coze", Region: "cn-beijing", Endpoint: "http://127.0.0.1:1",
			},
			Credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
	}
	for _, input := range supported {
		if _, err := registry.New(context.Background(), input); err != nil {
			t.Fatalf("DefaultRegistry().New(%s) error = %v", input.ProviderType, err)
		}
	}

	deferred := []domain.ProviderType{
		domain.ProviderQiniu,
		domain.ProviderAliyunOSS,
		domain.ProviderTencentCOS,
		domain.ProviderHuaweiOBS,
	}
	for _, provider := range deferred {
		_, err := registry.New(context.Background(), BuildInput{ProviderType: provider})
		if !errors.Is(err, domain.ErrProviderUnsupported) {
			t.Fatalf("DefaultRegistry().New(%s) error = %v", provider, err)
		}
	}
}

func TestRegistryNewStableErrors(t *testing.T) {
	if _, err := ((*Registry)(nil)).New(context.Background(), BuildInput{ProviderType: domain.ProviderMinIO}); !errors.Is(err, domain.ErrProviderUnsupported) {
		t.Fatalf("nil registry.New() error = %v", err)
	}

	registry := NewRegistry()
	if _, err := registry.New(context.Background(), BuildInput{ProviderType: domain.ProviderMinIO}); !errors.Is(err, domain.ErrProviderUnsupported) {
		t.Fatalf("missing builder error = %v", err)
	}

	wantErr := errors.New("builder failed")
	registry.Register(domain.ProviderMinIO, func(context.Context, BuildInput) (storage.Storage, error) {
		return nil, wantErr
	})
	if _, err := registry.New(context.Background(), BuildInput{ProviderType: domain.ProviderMinIO}); !errors.Is(err, wantErr) {
		t.Fatalf("builder error = %v, want %v", err, wantErr)
	}
}

func TestSetAndGetRuntimeDescriptor(t *testing.T) {
	SetRuntimeDescriptor(domain.RuntimeDescriptor{Source: domain.RuntimeSourceDatabase, ConfigID: 7, RuntimeRevision: 3, ProviderType: domain.ProviderMinIO})
	got := CurrentRuntimeDescriptor()
	if got.ConfigID != 7 || got.RuntimeRevision != 3 || got.Source != domain.RuntimeSourceDatabase || got.ProviderType != domain.ProviderMinIO {
		t.Fatalf("runtime descriptor = %+v", got)
	}
}

func TestNewFromConfigDoesNotCreateBucketsAndValidatesConfig(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		build      func(context.Context, domain.PublicConfig, domain.CredentialInput) (storage.Storage, error)
		cfg        domain.PublicConfig
		credential domain.CredentialInput
	}{
		{
			name:  "minio",
			build: miniopkg.NewFromConfig,
			cfg: domain.PublicConfig{
				Bucket: "coze", Endpoint: "127.0.0.1:1", UseSSL: false,
			},
			credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
		{
			name:  "s3",
			build: s3pkg.NewFromConfig,
			cfg: domain.PublicConfig{
				Bucket: "coze", Region: "us-east-1", Endpoint: "http://127.0.0.1:1", ForcePathStyle: true,
			},
			credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
		{
			name:  "tos",
			build: tospkg.NewFromConfig,
			cfg: domain.PublicConfig{
				Bucket: "coze", Region: "cn-beijing", Endpoint: "http://127.0.0.1:1",
			},
			credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := tc.build(ctx, tc.cfg, tc.credential)
			if err != nil {
				t.Fatalf("NewFromConfig() error = %v", err)
			}
			if client == nil {
				t.Fatal("NewFromConfig() returned nil storage")
			}

			_, err = tc.build(ctx, domain.PublicConfig{}, tc.credential)
			if !errors.Is(err, domain.ErrConfigInvalid) {
				t.Fatalf("NewFromConfig(invalid config) error = %v", err)
			}

			_, err = tc.build(ctx, tc.cfg, domain.CredentialInput{AccessKeyID: "ak"})
			if !errors.Is(err, domain.ErrConfigInvalid) {
				t.Fatalf("NewFromConfig(invalid credential) error = %v", err)
			}

			_, err = tc.build(ctx, tc.cfg, domain.CredentialInput{})
			if !errors.Is(err, domain.ErrConfigInvalid) {
				t.Fatalf("NewFromConfig(empty credential) error = %v", err)
			}
		})
	}
}

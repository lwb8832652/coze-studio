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
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/imagex"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	storageconfig "github.com/coze-dev/coze-studio/backend/infra/storage/config"
	storageimpl "github.com/coze-dev/coze-studio/backend/infra/storage/impl"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type BootstrapRepository interface {
	Count(context.Context) (int64, error)
	GetActive(context.Context) (*domain.Config, error)
	CreateWithCredential(context.Context, domain.Config, domain.CredentialInput, storageconfig.CredentialEncryptor) (*domain.Config, error)
}

type CredentialCodec interface {
	storageconfig.CredentialEncryptor
	Decrypt(uint64, domain.ProviderType, uint64, string) (domain.CredentialInput, error)
}

type ProviderRegistry interface {
	New(context.Context, storageimpl.BuildInput) (storage.Storage, error)
}

type Bootstrapper struct {
	Repository BootstrapRepository
	Codec      CredentialCodec
	Registry   ProviderRegistry
	Getenv     func(string) string
	AllowHTTP  bool
}

type BootstrapResult struct {
	Storage           storage.Storage
	ImageX            imagex.ImageX
	RuntimeDescriptor domain.RuntimeDescriptor
}

func NewBootstrapper(db *gorm.DB) *Bootstrapper {
	return &Bootstrapper{
		Repository: storageconfig.NewMySQLRepository(db),
		Registry:   storageimpl.DefaultRegistry(),
		Getenv:     os.Getenv,
	}
}

func AllowHTTPFromEnv(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return strings.EqualFold(getenv(consts.RunMode), "debug")
}

func (b *Bootstrapper) Bootstrap(ctx context.Context) (*BootstrapResult, error) {
	if b == nil {
		return nil, fmt.Errorf("%w: object storage bootstrapper is unavailable", domain.ErrConfigInvalid)
	}
	getenv := b.getenv()
	source, err := storageconfig.LoadConfigSource(getenv)
	if err != nil {
		return nil, err
	}
	if source == domain.RuntimeSourceEnvRescue {
		envConfig, err := storageconfig.LoadEnvConfig(getenv, domain.ValidationMode{AllowHTTP: b.AllowHTTP})
		if err != nil {
			return nil, err
		}
		return b.bootstrapEnvRescue(ctx, envConfig)
	}
	return b.bootstrapDatabase(ctx)
}

func (b *Bootstrapper) bootstrapEnvRescue(ctx context.Context, envConfig storageconfig.EnvConfig) (*BootstrapResult, error) {
	client, err := b.buildStorage(ctx, storageimpl.BuildInput{
		ProviderType: envConfig.ProviderType,
		PublicConfig: envConfig.PublicConfig,
		Credential:   envConfig.Credential,
	})
	if err != nil {
		return nil, err
	}
	descriptor := domain.RuntimeDescriptor{
		Source:       domain.RuntimeSourceEnvRescue,
		ProviderType: envConfig.ProviderType,
	}
	storageimpl.SetRuntimeDescriptor(descriptor)
	return &BootstrapResult{
		Storage:           client,
		ImageX:            storageimpl.NewStorageBackedImageX(client),
		RuntimeDescriptor: descriptor,
	}, nil
}

func (b *Bootstrapper) bootstrapDatabase(ctx context.Context) (*BootstrapResult, error) {
	if b.Repository == nil {
		return nil, fmt.Errorf("%w: object storage repository is unavailable", domain.ErrConfigInvalid)
	}
	codec, err := b.codec()
	if err != nil {
		return nil, err
	}
	count, err := b.Repository.Count(ctx)
	if err != nil {
		return nil, err
	}
	var active *domain.Config
	var credential domain.CredentialInput
	if count == 0 {
		envConfig, err := storageconfig.LoadEnvConfig(b.getenv(), domain.ValidationMode{AllowHTTP: b.AllowHTTP})
		if err != nil {
			return nil, err
		}
		active, err = b.Repository.CreateWithCredential(ctx, domain.Config{
			Name:         "Environment import",
			ProviderType: envConfig.ProviderType,
			PublicConfig: envConfig.PublicConfig,
			Active:       true,
		}, envConfig.Credential, codec)
		if err != nil {
			return nil, err
		}
		credential = envConfig.Credential
	} else {
		active, err = b.Repository.GetActive(ctx)
		if err != nil {
			return nil, err
		}
		if active == nil {
			return nil, domain.ErrPrimaryConfigMissing
		}
		credential, err = codec.Decrypt(active.ID, active.ProviderType, storageconfig.CredentialAADVersion, active.CredentialSecret)
		if err != nil {
			return nil, err
		}
	}
	client, err := b.buildStorage(ctx, storageimpl.BuildInput{
		ProviderType:    active.ProviderType,
		PublicConfig:    active.PublicConfig,
		Credential:      credential,
		ConfigID:        active.ID,
		RuntimeRevision: active.RuntimeRevision,
	})
	if err != nil {
		return nil, err
	}
	descriptor := domain.RuntimeDescriptor{
		Source:          domain.RuntimeSourceDatabase,
		ConfigID:        active.ID,
		RuntimeRevision: active.RuntimeRevision,
		ProviderType:    active.ProviderType,
	}
	storageimpl.SetRuntimeDescriptor(descriptor)
	return &BootstrapResult{
		Storage:           client,
		ImageX:            storageimpl.NewStorageBackedImageX(client),
		RuntimeDescriptor: descriptor,
	}, nil
}

func (b *Bootstrapper) buildStorage(ctx context.Context, input storageimpl.BuildInput) (storage.Storage, error) {
	registry := b.Registry
	if registry == nil {
		registry = storageimpl.DefaultRegistry()
	}
	client, err := registry.New(ctx, input)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("%w: object storage client is unavailable", domain.ErrConfigInvalid)
	}
	return client, nil
}

func (b *Bootstrapper) codec() (CredentialCodec, error) {
	if b.Codec != nil {
		return b.Codec, nil
	}
	codec, err := storageconfig.LoadCredentialCodec(b.getenv())
	if err != nil {
		return nil, err
	}
	b.Codec = codec
	return codec, nil
}

func (b *Bootstrapper) getenv() func(string) string {
	if b != nil && b.Getenv != nil {
		return b.Getenv
	}
	return os.Getenv
}

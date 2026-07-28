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

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/minio"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/s3"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/tos"
)

type BuildInput struct {
	ProviderType    domain.ProviderType
	PublicConfig    domain.PublicConfig
	Credential      domain.CredentialInput
	ConfigID        uint64
	RuntimeRevision uint64
}

type Builder func(context.Context, BuildInput) (storage.Storage, error)

type Registry struct {
	builders map[domain.ProviderType]Builder
}

func NewRegistry() *Registry {
	return &Registry{builders: make(map[domain.ProviderType]Builder)}
}

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.Register(domain.ProviderMinIO, func(ctx context.Context, input BuildInput) (storage.Storage, error) {
		return minio.NewFromConfig(ctx, input.PublicConfig, input.Credential)
	})
	registry.Register(domain.ProviderAWSS3, func(ctx context.Context, input BuildInput) (storage.Storage, error) {
		return s3.NewFromConfig(ctx, input.PublicConfig, input.Credential)
	})
	registry.Register(domain.ProviderTOS, func(ctx context.Context, input BuildInput) (storage.Storage, error) {
		return tos.NewFromConfig(ctx, input.PublicConfig, input.Credential)
	})
	for _, provider := range []domain.ProviderType{
		domain.ProviderQiniu,
		domain.ProviderAliyunOSS,
		domain.ProviderTencentCOS,
		domain.ProviderHuaweiOBS,
	} {
		registry.Register(provider, unsupportedProviderBuilder)
	}
	return registry
}

func (r *Registry) Register(provider domain.ProviderType, builder Builder) {
	if r == nil {
		return
	}
	if r.builders == nil {
		r.builders = make(map[domain.ProviderType]Builder)
	}
	r.builders[provider] = builder
}

func (r *Registry) New(ctx context.Context, input BuildInput) (storage.Storage, error) {
	if err := domain.ValidateProvider(input.ProviderType); err != nil {
		return nil, err
	}
	if r == nil || r.builders == nil {
		return nil, domain.ErrProviderUnsupported
	}
	builder := r.builders[input.ProviderType]
	if builder == nil {
		return nil, domain.ErrProviderUnsupported
	}
	return builder(ctx, input)
}

func unsupportedProviderBuilder(context.Context, BuildInput) (storage.Storage, error) {
	return nil, domain.ErrProviderUnsupported
}

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
	"os"
	"strings"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/imagex"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	storageconfig "github.com/coze-dev/coze-studio/backend/infra/storage/config"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type Storage = storage.Storage

func New(ctx context.Context) (Storage, error) {
	envConfig, err := storageconfig.LoadEnvConfig(os.Getenv, domain.ValidationMode{AllowHTTP: envStorageAllowHTTP(os.Getenv)})
	if err != nil {
		return nil, err
	}
	client, err := DefaultRegistry().New(ctx, BuildInput{
		ProviderType: envConfig.ProviderType,
		PublicConfig: envConfig.PublicConfig,
		Credential:   envConfig.Credential,
	})
	if err != nil {
		return nil, err
	}
	SetRuntimeDescriptor(domain.RuntimeDescriptor{
		Source:       domain.RuntimeSourceEnvRescue,
		ProviderType: envConfig.ProviderType,
	})
	return client, nil
}

func NewImagex(ctx context.Context) (imagex.ImageX, error) {
	client, err := New(ctx)
	if err != nil {
		return nil, err
	}
	return NewStorageBackedImageX(client), nil
}

func envStorageAllowHTTP(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return strings.EqualFold(getenv(consts.RunMode), "debug")
}

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
	"fmt"
	"path"
	"time"

	"github.com/coze-dev/coze-studio/backend/infra/imagex"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type storageBackedImageX struct {
	storage storage.Storage
}

var _ imagex.ImageX = (*storageBackedImageX)(nil)

func NewStorageBackedImageX(storageClient storage.Storage) imagex.ImageX {
	return &storageBackedImageX{storage: storageClient}
}

func (s *storageBackedImageX) GetUploadAuth(ctx context.Context, opt ...imagex.UploadAuthOpt) (*imagex.SecurityToken, error) {
	return s.GetUploadAuthWithExpire(ctx, time.Hour, opt...)
}

func (s *storageBackedImageX) GetUploadAuthWithExpire(ctx context.Context, expire time.Duration, opt ...imagex.UploadAuthOpt) (*imagex.SecurityToken, error) {
	scheme, ok := ctxcache.Get[string](ctx, consts.RequestSchemeKeyInCtx)
	if !ok || scheme == "" {
		scheme = "http"
	}
	now := time.Now()
	return &imagex.SecurityToken{
		AccessKeyID:     "",
		SecretAccessKey: "",
		SessionToken:    "",
		ExpiredTime:     now.Add(expire).Format("2006-01-02 15:04:05"),
		CurrentTime:     now.Format("2006-01-02 15:04:05"),
		HostScheme:      scheme,
	}, nil
}

func (s *storageBackedImageX) GetResourceURL(ctx context.Context, uri string, opts ...imagex.GetResourceOpt) (*imagex.ResourceURL, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("storage-backed imagex is not configured")
	}
	option := &imagex.GetResourceOption{}
	for _, opt := range opts {
		opt(option)
	}
	getOpts := make([]storage.GetOptFn, 0, 1)
	if option.Expire > 0 {
		getOpts = append(getOpts, storage.WithExpire(int64(option.Expire)))
	}
	url, err := s.storage.GetObjectUrl(ctx, uri, getOpts...)
	if err != nil {
		return nil, err
	}
	return &imagex.ResourceURL{URL: url}, nil
}

func (s *storageBackedImageX) Upload(ctx context.Context, data []byte, opts ...imagex.UploadAuthOpt) (*imagex.UploadResult, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("storage-backed imagex is not configured")
	}
	option := &imagex.UploadAuthOption{}
	for _, opt := range opts {
		opt(option)
	}
	if option.StoreKey == nil || *option.StoreKey == "" {
		return nil, fmt.Errorf("storage-backed imagex upload requires store key")
	}
	key := *option.StoreKey
	if err := s.storage.PutObject(ctx, key, data, storage.WithObjectSize(int64(len(data)))); err != nil {
		return nil, err
	}
	return &imagex.UploadResult{
		Result: &imagex.Result{
			Uri:       key,
			UriStatus: 2000,
		},
		FileInfo: &imagex.FileInfo{
			Name:      path.Base(key),
			Uri:       key,
			ImageSize: len(data),
		},
	}, nil
}

func (s *storageBackedImageX) GetServerID() string {
	return ""
}

func (s *storageBackedImageX) GetUploadHost(ctx context.Context) string {
	currentHost, ok := ctxcache.Get[string](ctx, consts.HostKeyInCtx)
	if !ok {
		return ""
	}
	return currentHost + consts.ApplyUploadActionURI
}

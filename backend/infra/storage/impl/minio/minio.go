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

package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type minioClient struct {
	client          *minio.Client
	streamOpener    minioObjectStreamOpener
	readinessCheck  func(context.Context, string) (bool, error)
	accessKeyID     string
	secretAccessKey string
	bucketName      string
	endpoint        string
}

type minioObjectStreamOpener interface {
	OpenObjectStream(context.Context, string, string) (io.ReadCloser, error)
}

type minioSDKObjectStreamOpener struct{ client *minio.Client }

var _ minioObjectStreamOpener = (*minioSDKObjectStreamOpener)(nil)

func (opener *minioSDKObjectStreamOpener) OpenObjectStream(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if opener == nil || opener.client == nil {
		return nil, fmt.Errorf("GetObject stream client is unavailable")
	}
	object, err := opener.client.GetObject(ctx, bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("GetObject failed: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	return object, nil
}

var (
	_ storage.StreamingStorage = (*minioClient)(nil)
	_ storage.ReadinessChecker = (*minioClient)(nil)
)

func New(ctx context.Context, endpoint, accessKeyID, secretAccessKey, bucketName string, useSSL bool) (storage.Storage, error) {
	m, err := getMinioClient(ctx, endpoint, accessKeyID, secretAccessKey, bucketName, useSSL)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	normalized, err := domain.ValidatePublicConfig(domain.ProviderMinIO, cfg, domain.ValidationMode{AllowHTTP: true})
	if err != nil {
		return nil, err
	}
	credential = domain.NormalizeCredentialInput(credential)
	if err = domain.ValidateCredentialInput(credential); err != nil {
		return nil, err
	}
	if !domain.HasCredentialPair(credential) {
		return nil, domain.ErrConfigInvalid
	}
	return getMinioClientWithOptions(ctx, normalized.Endpoint, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, normalized.UseSSL, false)
}

func getMinioClient(ctx context.Context, endpoint, accessKeyID, secretAccessKey, bucketName string, useSSL bool) (*minioClient, error) {
	return getMinioClientWithOptions(ctx, endpoint, accessKeyID, secretAccessKey, bucketName, useSSL, true)
}

func getMinioClientWithOptions(ctx context.Context, endpoint, accessKeyID, secretAccessKey, bucketName string, useSSL bool, createBucket bool) (*minioClient, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init minio client failed %v", err)
	}

	m := &minioClient{
		client:          client,
		streamOpener:    &minioSDKObjectStreamOpener{client: client},
		readinessCheck:  client.BucketExists,
		accessKeyID:     accessKeyID,
		secretAccessKey: secretAccessKey,
		bucketName:      bucketName,
		endpoint:        endpoint,
	}

	if createBucket {
		err = m.createBucketIfNeed(ctx, client, bucketName, "cn-north-1")
		if err != nil {
			return nil, fmt.Errorf("init minio client failed %v", err)
		}
	}

	return m, nil
}

func (m *minioClient) CheckReadiness(ctx context.Context) error {
	if ctx == nil || m == nil || m.readinessCheck == nil || strings.TrimSpace(m.bucketName) == "" {
		return storage.ErrReadinessUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	exists, err := m.readinessCheck(ctx, m.bucketName)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return storage.ErrReadinessUnavailable
	}
	if !exists {
		return storage.ErrReadinessUnavailable
	}
	return nil
}

func (m *minioClient) createBucketIfNeed(ctx context.Context, client *minio.Client, bucketName, region string) error {
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("check bucket %s exist failed %v", bucketName, err)
	}

	if exists {
		return nil
	}

	err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{Region: region})
	if err != nil {
		return fmt.Errorf("create bucket %s failed %v", bucketName, err)
	}

	return nil
}

func (m *minioClient) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return m.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (m *minioClient) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}

	minioOpts := minio.PutObjectOptions{}
	if option.ContentType != nil {
		minioOpts.ContentType = *option.ContentType
	}

	if option.ContentEncoding != nil {
		minioOpts.ContentEncoding = *option.ContentEncoding
	}

	if option.ContentDisposition != nil {
		minioOpts.ContentDisposition = *option.ContentDisposition
	}

	if option.ContentLanguage != nil {
		minioOpts.ContentLanguage = *option.ContentLanguage
	}

	if option.Expires != nil {
		minioOpts.Expires = *option.Expires
	}

	if option.Tagging != nil {
		minioOpts.UserTags = option.Tagging
	}

	_, err := m.client.PutObject(ctx, m.bucketName, objectKey,
		content, option.ObjectSize, minioOpts)
	if err != nil {
		return fmt.Errorf("PutObject failed: %v", err)
	}
	return nil
}

func (m *minioClient) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	obj, err := m.OpenObjectStream(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("ReadObject failed: %v", err)
	}
	return data, nil
}

func (m *minioClient) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("GetObject context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil || m.streamOpener == nil {
		return nil, fmt.Errorf("GetObject stream opener is unavailable")
	}
	obj, err := m.streamOpener.OpenObjectStream(ctx, m.bucketName, objectKey)
	if err != nil {
		return nil, fmt.Errorf("GetObject failed: %w", err)
	}
	if obj == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	if err := ctx.Err(); err != nil {
		_ = obj.Close()
		return nil, err
	}
	return obj, nil
}

func (m *minioClient) DeleteObject(ctx context.Context, objectKey string) error {
	err := m.client.RemoveObject(ctx, m.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("DeleteObject failed: %v", err)
	}
	return nil
}

func (m *minioClient) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}

	if option.Expire == 0 {
		option.Expire = 3600 * 24 * 7
	}

	reqParams := make(url.Values)
	if option.ResponseContentDisposition != "" {
		reqParams.Set(
			"response-content-disposition",
			option.ResponseContentDisposition,
		)
	}
	if option.ResponseContentType != "" {
		reqParams.Set("response-content-type", option.ResponseContentType)
	}
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, objectKey, time.Duration(option.Expire)*time.Second, reqParams)
	if err != nil {
		return "", fmt.Errorf("GetObjectUrl failed: %v", err)
	}

	return presignedURL.String(), nil
}

func (m *minioClient) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}

	files, err := m.ListAllObjects(ctx, input.Prefix, opts...)
	if err != nil {
		return nil, err
	}

	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}

	if option.WithURL {
		files, err = fileutil.AssembleFileUrl(ctx, &option.Expire, files, m)
		if err != nil {
			return nil, err
		}
	}

	return &storage.ListObjectsPaginatedOutput{
		Files:       files,
		IsTruncated: false,
		Cursor:      "",
	}, nil
}

func (m *minioClient) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}

	minioOpts := minio.ListObjectsOptions{
		Prefix:       prefix,
		Recursive:    true,
		WithMetadata: option.WithTagging,
	}

	objectCh := m.client.ListObjects(ctx, m.bucketName, minioOpts)

	var files []*storage.FileInfo
	for object := range objectCh {
		if object.Err != nil {
			return nil, object.Err
		}

		files = append(files, &storage.FileInfo{
			Key:          object.Key,
			LastModified: object.LastModified,
			ETag:         object.ETag,
			Size:         object.Size,
			Tagging:      object.UserTags,
		})

		logs.CtxDebugf(ctx, "key = %s, lastModified = %s, eTag = %s, size = %d, tagging = %v",
			object.Key, object.LastModified, object.ETag, object.Size, object.UserTags)
	}

	return files, nil
}

func (m *minioClient) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}

	stat, err := m.client.StatObject(ctx, m.bucketName, objectKey, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, storage.ErrObjectNotFound
		}

		return nil, fmt.Errorf("HeadObject failed for key %s: %w", objectKey, err)
	}

	f := &storage.FileInfo{
		Key:          objectKey,
		LastModified: stat.LastModified,
		ETag:         stat.ETag,
		Size:         stat.Size,
	}

	if option.WithTagging {
		tags, err := m.client.GetObjectTagging(ctx, m.bucketName, objectKey, minio.GetObjectTaggingOptions{})
		if err != nil {
			return nil, err
		}

		f.Tagging = tags.ToMap()
	}

	if option.WithURL {
		f.URL, err = m.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}

	return f, nil
}

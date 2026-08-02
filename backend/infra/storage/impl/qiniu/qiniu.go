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

package qiniu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	qiniuauth "github.com/qiniu/go-sdk/v7/auth"
	qiniugo "github.com/qiniu/go-sdk/v7/client"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type qiniuClient struct {
	uploader       *qiniustorage.FormUploader
	bucketManager  *qiniustorage.BucketManager
	streamOpener   qiniuObjectStreamOpener
	readinessCheck func(context.Context, string) error
	mac            *qiniuauth.Credentials
	bucketName     string
	downloadDomain string
}

type qiniuObjectStreamOpener interface {
	OpenObjectStream(context.Context, string, string) (io.ReadCloser, error)
}

type qiniuSDKObjectStreamOpener struct {
	bucketManager  *qiniustorage.BucketManager
	downloadDomain string
}

var _ qiniuObjectStreamOpener = (*qiniuSDKObjectStreamOpener)(nil)

func (opener *qiniuSDKObjectStreamOpener) OpenObjectStream(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if opener == nil || opener.bucketManager == nil {
		return nil, fmt.Errorf("GetObject stream client is unavailable")
	}
	output, err := opener.bucketManager.Get(bucket, objectKey, &qiniustorage.GetObjectInput{
		Context:         ctx,
		DownloadDomains: []string{opener.downloadDomain},
		PresignUrl:      true,
	})
	if err != nil {
		if output != nil {
			_ = output.Close()
		}
		return nil, err
	}
	if output == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	return output, nil
}

var (
	_ storage.Storage          = (*qiniuClient)(nil)
	_ storage.StreamingStorage = (*qiniuClient)(nil)
	_ storage.ReadinessChecker = (*qiniuClient)(nil)
)

func New(ctx context.Context, ak, sk, bucketName, downloadDomain string, useHTTPS bool) (storage.Storage, error) {
	cfg := domain.PublicConfig{Bucket: bucketName, DownloadDomain: downloadDomain, UseHTTPS: useHTTPS}
	credential := domain.CredentialInput{AccessKeyID: ak, SecretAccessKey: sk}
	return NewFromConfig(ctx, cfg, credential)
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	return NewFromConfigWithMode(ctx, cfg, credential, domain.ValidationMode{})
}

func NewFromConfigWithMode(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput, mode domain.ValidationMode) (storage.Storage, error) {
	normalized, err := domain.ValidatePublicConfig(domain.ProviderQiniu, cfg, mode)
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
	return getQiniuClient(ctx, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, normalized.DownloadDomain, normalized.UseHTTPS)
}

func getQiniuClient(ctx context.Context, ak, sk, bucketName, downloadDomain string, useHTTPS bool) (*qiniuClient, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	domainWithScheme := assembleDownloadDomain(downloadDomain, useHTTPS)
	mac := qiniuauth.New(ak, sk)
	cfg := &qiniustorage.Config{UseHTTPS: useHTTPS}
	uploader := qiniustorage.NewFormUploader(cfg)
	bucketManager := qiniustorage.NewBucketManager(mac, cfg)
	q := &qiniuClient{
		uploader:      uploader,
		bucketManager: bucketManager,
		streamOpener:  &qiniuSDKObjectStreamOpener{bucketManager: bucketManager, downloadDomain: domainWithScheme},
		readinessCheck: func(ctx context.Context, bucket string) error {
			_, _, err := bucketManager.ListFilesWithContext(ctx, bucket, qiniustorage.ListInputOptionsLimit(1))
			return err
		},
		mac:            mac,
		bucketName:     bucketName,
		downloadDomain: domainWithScheme,
	}
	return q, nil
}

func (q *qiniuClient) CheckReadiness(ctx context.Context) error {
	if ctx == nil || q == nil || q.readinessCheck == nil || strings.TrimSpace(q.bucketName) == "" {
		return storage.ErrReadinessUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := q.readinessCheck(ctx, q.bucketName); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return storage.ErrReadinessUnavailable
	}
	return nil
}

func (q *qiniuClient) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return q.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (q *qiniuClient) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if q == nil || q.uploader == nil || q.mac == nil {
		return fmt.Errorf("PutObject client is unavailable")
	}
	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}
	if option.ObjectSize < 0 {
		return fmt.Errorf("object size must be non-negative")
	}

	extra := &qiniustorage.PutExtra{}
	if option.ContentType != nil {
		extra.MimeType = *option.ContentType
	}
	policy := qiniustorage.PutPolicy{Scope: q.bucketName}
	upToken := policy.UploadToken(q.mac)
	if err := q.uploader.Put(ctx, &qiniustorage.PutRet{}, upToken, objectKey, content, option.ObjectSize, extra); err != nil {
		return fmt.Errorf("PutObject failed: %w", err)
	}
	return nil
}

func (q *qiniuClient) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	body, err := q.OpenObjectStream(ctx, objectKey)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (q *qiniuClient) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("GetObject context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q == nil || q.streamOpener == nil {
		return nil, fmt.Errorf("GetObject stream opener is unavailable")
	}
	body, err := q.streamOpener.OpenObjectStream(ctx, q.bucketName, objectKey)
	if err != nil {
		return nil, fmt.Errorf("GetObject failed: %w", err)
	}
	if body == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	if err := ctx.Err(); err != nil {
		_ = body.Close()
		return nil, err
	}
	return body, nil
}

func (q *qiniuClient) DeleteObject(ctx context.Context, objectKey string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if q == nil || q.bucketManager == nil {
		return fmt.Errorf("DeleteObject client is unavailable")
	}
	if err := q.bucketManager.Delete(q.bucketName, objectKey); err != nil {
		return fmt.Errorf("DeleteObject failed: %w", err)
	}
	return ctx.Err()
}

func (q *qiniuClient) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	if err := ctxErr(ctx); err != nil {
		return "", err
	}
	if q == nil || q.mac == nil {
		return "", fmt.Errorf("GetObjectUrl client is unavailable")
	}
	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	expire := int64(7 * 24 * 60 * 60)
	if option.Expire > 0 {
		expire = option.Expire
	}
	query := make(url.Values)
	if option.ResponseContentDisposition != "" {
		query.Set("response-content-disposition", option.ResponseContentDisposition)
	}
	if option.ResponseContentType != "" {
		query.Set("response-content-type", option.ResponseContentType)
	}
	return qiniustorage.MakePrivateURLv2WithQuery(q.mac, q.downloadDomain, objectKey, query, time.Now().Unix()+expire), nil
}

func (q *qiniuClient) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if q == nil || q.bucketManager == nil {
		return nil, fmt.Errorf("HeadObject client is unavailable")
	}
	info, err := q.bucketManager.Stat(q.bucketName, objectKey)
	if err != nil {
		if isQiniuNotFound(err) {
			return nil, storage.ErrObjectNotFound
		}
		return nil, fmt.Errorf("HeadObject failed for key %s: %w", objectKey, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fileInfo := &storage.FileInfo{
		Key:          objectKey,
		LastModified: qiniuPutTime(info.PutTime),
		ETag:         info.Hash,
		Size:         info.Fsize,
	}
	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	if option.WithURL {
		fileInfo.URL, err = q.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}
	return fileInfo, nil
}

func (q *qiniuClient) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if q == nil || q.bucketManager == nil {
		return nil, fmt.Errorf("ListObjects client is unavailable")
	}
	limit := input.PageSize
	if limit > 1000 {
		limit = 1000
	}
	ret, hasNext, err := q.bucketManager.ListFilesWithContext(ctx, q.bucketName,
		qiniustorage.ListInputOptionsPrefix(input.Prefix),
		qiniustorage.ListInputOptionsMarker(input.Cursor),
		qiniustorage.ListInputOptionsLimit(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("list objects failed, err: %w", err)
	}
	files := make([]*storage.FileInfo, 0, len(ret.Items))
	for _, item := range ret.Items {
		if item.IsEmpty() {
			continue
		}
		files = append(files, &storage.FileInfo{
			Key:          item.Key,
			LastModified: qiniuPutTime(item.PutTime),
			ETag:         item.Hash,
			Size:         item.Fsize,
		})
	}

	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	if option.WithURL {
		files, err = fileutil.AssembleFileUrl(ctx, &option.Expire, files, q)
		if err != nil {
			return nil, err
		}
	}

	return &storage.ListObjectsPaginatedOutput{
		Files:       files,
		Cursor:      ret.Marker,
		IsTruncated: hasNext,
	}, nil
}

func (q *qiniuClient) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	const (
		defaultPageSize = 100
		maxListObjects  = 10000
	)
	files := make([]*storage.FileInfo, 0, defaultPageSize)
	cursor := ""
	for {
		output, err := q.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{
			Prefix:   prefix,
			PageSize: defaultPageSize,
			Cursor:   cursor,
		}, opts...)
		if err != nil {
			return nil, fmt.Errorf("list objects failed, prefix = %v, err: %w", prefix, err)
		}
		for _, object := range output.Files {
			logs.CtxDebugf(ctx, "key = %s, lastModified = %s, eTag = %s, size = %d, tagging = %v",
				object.Key, object.LastModified, object.ETag, object.Size, object.Tagging)
			files = append(files, object)
		}
		cursor = output.Cursor
		if len(files) >= maxListObjects {
			logs.CtxErrorf(ctx, "[ListObjects] max list objects reached, total: %d", len(files))
			break
		}
		if !output.IsTruncated || output.Cursor == "" {
			break
		}
	}
	return files, nil
}

func assembleDownloadDomain(downloadDomain string, useHTTPS bool) string {
	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimRight(downloadDomain, "/")
}

func qiniuPutTime(putTime int64) time.Time {
	if putTime <= 0 {
		return time.Time{}
	}
	return time.Unix(0, putTime*100)
}

func isQiniuNotFound(err error) bool {
	var info *qiniugo.ErrorInfo
	if errors.As(err, &info) {
		return info.Code == http.StatusNotFound ||
			info.Code == 612 ||
			info.Code == http.StatusBadRequest && strings.Contains(strings.ToLower(info.Err), "no such")
	}
	return false
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is unavailable")
	}
	return ctx.Err()
}

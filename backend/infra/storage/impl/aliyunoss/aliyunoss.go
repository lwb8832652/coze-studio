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

package aliyunoss

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	oss "github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/goutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/taskgroup"
)

type aliyunOSSClient struct {
	client         *oss.Client
	streamOpener   aliyunOSSObjectStreamOpener
	readinessCheck func(context.Context, string) error
	bucketName     string
}

type aliyunOSSObjectStreamOpener interface {
	OpenObjectStream(context.Context, string, string) (io.ReadCloser, error)
}

type aliyunOSSSDKObjectStreamOpener struct{ client *oss.Client }

var _ aliyunOSSObjectStreamOpener = (*aliyunOSSSDKObjectStreamOpener)(nil)

func (opener *aliyunOSSSDKObjectStreamOpener) OpenObjectStream(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if opener == nil || opener.client == nil {
		return nil, fmt.Errorf("GetObject stream client is unavailable")
	}
	output, err := opener.client.GetObject(ctx, &oss.GetObjectRequest{
		Bucket: oss.Ptr(bucket),
		Key:    oss.Ptr(objectKey),
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Body == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	return output.Body, nil
}

var (
	_ storage.Storage          = (*aliyunOSSClient)(nil)
	_ storage.StreamingStorage = (*aliyunOSSClient)(nil)
	_ storage.ReadinessChecker = (*aliyunOSSClient)(nil)
)

func New(ctx context.Context, ak, sk, bucketName, endpoint, region string) (storage.Storage, error) {
	return getAliyunOSSClient(ctx, ak, sk, bucketName, endpoint, region, false)
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	return NewFromConfigWithMode(ctx, cfg, credential, domain.ValidationMode{})
}

func NewFromConfigWithMode(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput, mode domain.ValidationMode) (storage.Storage, error) {
	normalized, err := domain.ValidatePublicConfig(domain.ProviderAliyunOSS, cfg, mode)
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
	endpoint := normalized.EndpointOverride
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://oss-%s.aliyuncs.com", normalized.Region)
	} else if !mode.AllowHTTP && !strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return nil, fmt.Errorf("%w: endpoint override must use https", domain.ErrConfigInvalid)
	}
	return getAliyunOSSClient(ctx, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, endpoint, normalized.Region, normalized.ForcePathStyle)
}

func getAliyunOSSClient(ctx context.Context, ak, sk, bucketName, endpoint, region string, forcePathStyle bool) (*aliyunOSSClient, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider(ak, sk)).
		WithRegion(region).
		WithEndpoint(endpoint).
		WithUsePathStyle(forcePathStyle)
	client := oss.NewClient(cfg)
	c := &aliyunOSSClient{
		client:       client,
		streamOpener: &aliyunOSSSDKObjectStreamOpener{client: client},
		readinessCheck: func(ctx context.Context, bucket string) error {
			_, err := client.GetBucketInfo(ctx, &oss.GetBucketInfoRequest{Bucket: oss.Ptr(bucket)})
			return err
		},
		bucketName: bucketName,
	}
	return c, nil
}

func (c *aliyunOSSClient) CheckReadiness(ctx context.Context) error {
	if ctx == nil || c == nil || c.readinessCheck == nil || strings.TrimSpace(c.bucketName) == "" {
		return storage.ErrReadinessUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.readinessCheck(ctx, c.bucketName); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return storage.ErrReadinessUnavailable
	}
	return nil
}

func (c *aliyunOSSClient) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return c.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (c *aliyunOSSClient) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}
	input := &oss.PutObjectRequest{
		Bucket: oss.Ptr(c.bucketName),
		Key:    oss.Ptr(objectKey),
		Body:   content,
	}
	if option.ContentType != nil {
		input.ContentType = option.ContentType
	}
	if option.ContentEncoding != nil {
		input.ContentEncoding = option.ContentEncoding
	}
	if option.ContentDisposition != nil {
		input.ContentDisposition = option.ContentDisposition
	}
	if option.Expires != nil {
		input.Expires = oss.Ptr(option.Expires.UTC().Format(http.TimeFormat))
	}
	if option.ObjectSize > 0 {
		input.ContentLength = oss.Ptr(option.ObjectSize)
	}
	if len(option.Tagging) > 0 {
		input.Tagging = oss.Ptr(goutil.MapToQuery(option.Tagging))
	}
	_, err := c.client.PutObject(ctx, input)
	return err
}

func (c *aliyunOSSClient) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	body, err := c.OpenObjectStream(ctx, objectKey)
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

func (c *aliyunOSSClient) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("GetObject context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.streamOpener == nil {
		return nil, fmt.Errorf("GetObject stream opener is unavailable")
	}
	body, err := c.streamOpener.OpenObjectStream(ctx, c.bucketName, objectKey)
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

func (c *aliyunOSSClient) DeleteObject(ctx context.Context, objectKey string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	_, err := c.client.DeleteObject(ctx, &oss.DeleteObjectRequest{
		Bucket: oss.Ptr(c.bucketName),
		Key:    oss.Ptr(objectKey),
	})
	return err
}

func (c *aliyunOSSClient) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	if err := ctxErr(ctx); err != nil {
		return "", err
	}
	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	expire := int64(24 * 60 * 60)
	if option.Expire > 0 {
		expire = option.Expire
	}
	input := &oss.GetObjectRequest{
		Bucket: oss.Ptr(c.bucketName),
		Key:    oss.Ptr(objectKey),
	}
	if option.ResponseContentDisposition != "" {
		input.ResponseContentDisposition = oss.Ptr(option.ResponseContentDisposition)
	}
	if option.ResponseContentType != "" {
		input.ResponseContentType = oss.Ptr(option.ResponseContentType)
	}
	result, err := c.client.Presign(ctx, input, oss.PresignExpires(time.Duration(expire)*time.Second))
	if err != nil {
		return "", fmt.Errorf("GetObjectUrl failed: %w", err)
	}
	return result.URL, nil
}

func (c *aliyunOSSClient) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}
	output, err := c.client.ListObjectsV2(ctx, &oss.ListObjectsV2Request{
		Bucket:            oss.Ptr(c.bucketName),
		Prefix:            oss.Ptr(input.Prefix),
		MaxKeys:           int32(input.PageSize),
		ContinuationToken: oss.Ptr(input.Cursor),
	})
	if err != nil {
		return nil, fmt.Errorf("list objects failed, err: %w", err)
	}

	files := make([]*storage.FileInfo, 0, len(output.Contents))
	for _, obj := range output.Contents {
		file := &storage.FileInfo{Size: obj.Size}
		if obj.Key != nil {
			file.Key = *obj.Key
		}
		if obj.LastModified != nil {
			file.LastModified = *obj.LastModified
		}
		if obj.ETag != nil {
			file.ETag = *obj.ETag
		}
		files = append(files, file)
	}

	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	if option.WithTagging {
		taskGroup := taskgroup.NewTaskGroup(ctx, 5)
		for idx := range files {
			f := files[idx]
			taskGroup.Go(func() error {
				tagging, err := c.client.GetObjectTagging(ctx, &oss.GetObjectTaggingRequest{
					Bucket: oss.Ptr(c.bucketName),
					Key:    oss.Ptr(f.Key),
				})
				if err != nil {
					return err
				}
				f.Tagging = aliyunTagsToMap(tagging.Tags)
				return nil
			})
		}
		if err := taskGroup.Wait(); err != nil {
			return nil, err
		}
	}
	if option.WithURL {
		files, err = fileutil.AssembleFileUrl(ctx, &option.Expire, files, c)
		if err != nil {
			return nil, err
		}
	}

	result := &storage.ListObjectsPaginatedOutput{
		Files:       files,
		IsTruncated: output.IsTruncated,
	}
	if output.NextContinuationToken != nil {
		result.Cursor = *output.NextContinuationToken
	}
	return result, nil
}

func (c *aliyunOSSClient) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	const (
		defaultPageSize = 100
		maxListObjects  = 10000
	)
	files := make([]*storage.FileInfo, 0, defaultPageSize)
	cursor := ""
	for {
		output, err := c.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{
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

func (c *aliyunOSSClient) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	output, err := c.client.HeadObject(ctx, &oss.HeadObjectRequest{
		Bucket: oss.Ptr(c.bucketName),
		Key:    oss.Ptr(objectKey),
	})
	if err != nil {
		if isAliyunNotFound(err) {
			return nil, storage.ErrObjectNotFound
		}
		return nil, err
	}
	fileInfo := &storage.FileInfo{
		Key:  objectKey,
		Size: output.ContentLength,
	}
	if output.LastModified != nil {
		fileInfo.LastModified = *output.LastModified
	}
	if output.ETag != nil {
		fileInfo.ETag = *output.ETag
	}

	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	if option.WithTagging {
		tagging, err := c.client.GetObjectTagging(ctx, &oss.GetObjectTaggingRequest{
			Bucket: oss.Ptr(c.bucketName),
			Key:    oss.Ptr(objectKey),
		})
		if err != nil {
			return nil, err
		}
		fileInfo.Tagging = aliyunTagsToMap(tagging.Tags)
	}
	if option.WithURL {
		fileInfo.URL, err = c.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}
	return fileInfo, nil
}

func aliyunTagsToMap(tags []oss.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			m[*tag.Key] = *tag.Value
		}
	}
	return m
}

func isAliyunNotFound(err error) bool {
	var serviceErr *oss.ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.StatusCode == http.StatusNotFound || serviceErr.Code == "NoSuchKey"
	}
	return false
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is unavailable")
	}
	return ctx.Err()
}

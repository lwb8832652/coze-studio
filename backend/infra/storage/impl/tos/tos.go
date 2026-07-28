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

package tos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/goutil"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/taskgroup"
)

type tosClient struct {
	client         *tos.ClientV2
	streamOpener   tosObjectStreamOpener
	readinessCheck func(context.Context, string) error
	bucketName     string
}

type tosObjectStreamOpener interface {
	OpenObjectStream(context.Context, string, string) (io.ReadCloser, error)
}

type tosSDKObjectStreamOpener struct{ client *tos.ClientV2 }

var _ tosObjectStreamOpener = (*tosSDKObjectStreamOpener)(nil)

func (opener *tosSDKObjectStreamOpener) OpenObjectStream(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if opener == nil || opener.client == nil {
		return nil, fmt.Errorf("GetObject stream client is unavailable")
	}
	output, err := opener.client.GetObjectV2(ctx, &tos.GetObjectV2Input{
		Bucket: bucket, Key: objectKey,
		ResponseContentType: "application/json", ResponseContentEncoding: "deflate",
	})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Content == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	return output.Content, nil
}

var (
	_ storage.StreamingStorage = (*tosClient)(nil)
	_ storage.ReadinessChecker = (*tosClient)(nil)
)

func New(ctx context.Context, ak, sk, bucketName, endpoint, region string) (storage.Storage, error) {
	t, err := getTosClient(ctx, ak, sk, bucketName, endpoint, region)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	normalized, err := domain.ValidatePublicConfig(domain.ProviderTOS, cfg, domain.ValidationMode{})
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
	return getTosClientWithOptions(ctx, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, normalized.Endpoint, normalized.Region, false)
}

func getTosClient(ctx context.Context, ak, sk, bucketName, endpoint, region string) (*tosClient, error) {
	return getTosClientWithOptions(ctx, ak, sk, bucketName, endpoint, region, true)
}

func getTosClientWithOptions(ctx context.Context, ak, sk, bucketName, endpoint, region string, createBucket bool) (*tosClient, error) {
	credential := tos.NewStaticCredentials(ak, sk)
	client, err := tos.NewClientV2(endpoint,
		tos.WithCredentials(credential), tos.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("new tos client failed, bucketName: %s, endpoint: %s, region: %s, err: %v", bucketName, endpoint, region, err)
	}

	t := &tosClient{
		client:       client,
		streamOpener: &tosSDKObjectStreamOpener{client: client},
		readinessCheck: func(ctx context.Context, bucket string) error {
			_, err := client.HeadBucket(ctx, &tos.HeadBucketInput{Bucket: bucket})
			return err
		},
		bucketName: bucketName,
	}

	if createBucket {
		err = t.CheckAndCreateBucket(ctx)
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

func (t *tosClient) CheckReadiness(ctx context.Context) error {
	if ctx == nil || t == nil || t.readinessCheck == nil || strings.TrimSpace(t.bucketName) == "" {
		return storage.ErrReadinessUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.readinessCheck(ctx, t.bucketName); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return storage.ErrReadinessUnavailable
	}
	return nil
}

func (t *tosClient) CheckAndCreateBucket(ctx context.Context) error {
	client := t.client
	bucketName := t.bucketName

	_, err := client.HeadBucket(ctx, &tos.HeadBucketInput{Bucket: bucketName})
	if err == nil {
		return nil // already exist
	}

	serverErr, ok := err.(*tos.TosServerError)
	if !ok {
		return err
	}

	if serverErr.StatusCode == http.StatusNotFound {
		// Bucket does not exist
		logs.CtxInfof(ctx, "Bucket not found.")
		resp, err := client.CreateBucketV2(ctx, &tos.CreateBucketV2Input{
			Bucket: bucketName,
			ACL:    enum.ACLPrivate,
		})

		logs.CtxInfof(ctx, "Bucket Create resp: %v, err: %v", conv.DebugJsonToStr(resp), err)
		return err
	}

	return err
}
func (t *tosClient) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return t.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (t *tosClient) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	client := t.client
	bucketName := t.bucketName

	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}

	input := &tos.PutObjectV2Input{
		PutObjectBasicInput: tos.PutObjectBasicInput{
			Bucket: bucketName,
			Key:    objectKey,
		},
		Content: content,
	}

	if option.ContentType != nil {
		input.ContentType = *option.ContentType
	}
	if option.ContentEncoding != nil {
		input.ContentEncoding = *option.ContentEncoding
	}
	if option.ContentDisposition != nil {
		input.ContentDisposition = *option.ContentDisposition
	}
	if option.ContentLanguage != nil {
		input.ContentLanguage = *option.ContentLanguage
	}
	if option.Expires != nil {
		input.Expires = *option.Expires
	}

	if option.ObjectSize > 0 {
		input.ContentLength = option.ObjectSize
	}

	if len(option.Tagging) > 0 {
		input.Tagging = goutil.MapToQuery(option.Tagging)
	}

	_, err := client.PutObjectV2(ctx, input)

	return err
}

func (t *tosClient) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	body, err := t.OpenObjectStream(ctx, objectKey)
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

func (t *tosClient) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("GetObject context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t == nil || t.streamOpener == nil {
		return nil, fmt.Errorf("GetObject stream opener is unavailable")
	}
	content, err := t.streamOpener.OpenObjectStream(ctx, t.bucketName, objectKey)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	if err := ctx.Err(); err != nil {
		_ = content.Close()
		return nil, err
	}
	return content, nil
}

func (t *tosClient) DeleteObject(ctx context.Context, objectKey string) error {
	client := t.client
	bucketName := t.bucketName

	// Delete the specified object in the bucket
	_, err := client.DeleteObjectV2(ctx, &tos.DeleteObjectV2Input{
		Bucket: bucketName,
		Key:    objectKey,
	})

	return err
}

func (t *tosClient) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	client := t.client
	bucketName := t.bucketName

	opt := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&opt)
	}

	expire := int64(7 * 24 * 60 * 60)
	if opt.Expire > 0 {
		expire = opt.Expire
	}

	query := make(map[string]string)
	if opt.ResponseContentDisposition != "" {
		query["response-content-disposition"] = opt.ResponseContentDisposition
	}
	if opt.ResponseContentType != "" {
		query["response-content-type"] = opt.ResponseContentType
	}
	output, err := client.PreSignedURL(&tos.PreSignedURLInput{
		HTTPMethod: enum.HttpMethodGet,
		Expires:    expire,
		Bucket:     bucketName,
		Key:        objectKey,
		Query:      query,
	})
	if err != nil {
		return "", err
	}

	return output.SignedUrl, nil
}

func (t *tosClient) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}

	output, err := t.client.ListObjectsV2(ctx, &tos.ListObjectsV2Input{
		Bucket: t.bucketName,
		ListObjectsInput: tos.ListObjectsInput{
			MaxKeys: int(input.PageSize),
			Marker:  input.Cursor,
			Prefix:  input.Prefix,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("list objects failed, err: %w", err)
	}

	files := make([]*storage.FileInfo, 0, len(output.Contents))
	for _, obj := range output.Contents {
		if obj.Size == 0 && strings.HasSuffix(obj.Key, "/") {
			logs.CtxDebugf(ctx, "[ListObjectsPaginated] skip dir: %s", obj.Key)
			continue
		}

		files = append(files, &storage.FileInfo{
			Key:          obj.Key,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			Size:         obj.Size,
		})
	}

	opt := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&opt)
	}

	if opt.WithTagging {
		client := t.client
		taskGroup := taskgroup.NewTaskGroup(ctx, 5)
		for idx := range files {
			f := files[idx]
			taskGroup.Go(func() error {
				tagging, err := client.GetObjectTagging(ctx, &tos.GetObjectTaggingInput{
					Bucket: t.bucketName,
					Key:    f.Key,
				})
				if err != nil {
					return err
				}

				f.Tagging = tagsToMap(tagging.TagSet.Tags)
				return nil
			})
		}

		if err := taskGroup.Wait(); err != nil {
			return nil, err
		}
	}

	if opt.WithURL {
		files, err = fileutil.AssembleFileUrl(ctx, &opt.Expire, files, t)
		if err != nil {
			return nil, err
		}
	}

	return &storage.ListObjectsPaginatedOutput{
		Files:       files,
		Cursor:      output.NextMarker,
		IsTruncated: output.IsTruncated,
	}, nil
}

func (t *tosClient) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	const (
		DefaultPageSize = 100
		MaxListObjects  = 10000
	)

	files := make([]*storage.FileInfo, 0, DefaultPageSize)
	cursor := ""

	for {
		output, err := t.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{
			Prefix:   prefix,
			PageSize: DefaultPageSize,
			Cursor:   cursor,
		}, opts...)
		if err != nil {
			return nil, fmt.Errorf("list objects failed, prefix = %v, err: %v", prefix, err)
		}

		for _, object := range output.Files {
			logs.CtxDebugf(ctx, "key = %s, lastModified = %s, eTag = %s, size = %d, tagging = %v",
				object.Key, object.LastModified, object.ETag, object.Size, object.Tagging)
			files = append(files, object)
		}

		cursor = output.Cursor
		logs.CtxDebugf(ctx, "IsTruncated = %v, Cursor = %s", output.IsTruncated, output.Cursor)

		if len(files) >= MaxListObjects {
			logs.CtxErrorf(ctx, "[ListObjects] max list objects reached, total: %d", len(files))
			break
		}

		if !output.IsTruncated || output.Cursor == "" {
			break
		}
	}

	return files, nil
}

func (t *tosClient) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	output, err := t.client.HeadObjectV2(ctx, &tos.HeadObjectV2Input{Bucket: t.bucketName, Key: objectKey})
	if err != nil {
		if serverErr, ok := err.(*tos.TosServerError); ok {
			if serverErr.StatusCode == http.StatusNotFound {
				return nil, storage.ErrObjectNotFound
			}
		}
		return nil, err
	}

	fileInfo := &storage.FileInfo{
		Key:          objectKey,
		LastModified: output.LastModified,
		ETag:         output.ETag,
		Size:         output.ContentLength,
	}

	opt := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&opt)
	}

	if opt.WithTagging {
		tagging, err := t.client.GetObjectTagging(ctx, &tos.GetObjectTaggingInput{
			Bucket: t.bucketName,
			Key:    objectKey,
		})
		if err != nil {
			return nil, err
		}

		fileInfo.Tagging = tagsToMap(tagging.TagSet.Tags)
	}

	if opt.WithURL {
		fileInfo.URL, err = t.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}

	return fileInfo, nil
}

func tagsToMap(tags []tos.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}

	m := make(map[string]string, len(tags))
	for _, tag := range tags {
		m[tag.Key] = tag.Value
	}

	return m
}

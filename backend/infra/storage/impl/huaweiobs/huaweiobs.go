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

package huaweiobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	obs "github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/taskgroup"
)

type huaweiOBSClient struct {
	client         *obs.ObsClient
	streamOpener   huaweiOBSObjectStreamOpener
	readinessCheck func(context.Context) error
	bucketName     string
}

type huaweiOBSObjectStreamOpener interface {
	OpenObjectStream(context.Context, string, string) (io.ReadCloser, error)
}

type huaweiOBSSDKObjectStreamOpener struct{ client *obs.ObsClient }

var _ huaweiOBSObjectStreamOpener = (*huaweiOBSSDKObjectStreamOpener)(nil)

func (opener *huaweiOBSSDKObjectStreamOpener) OpenObjectStream(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if opener == nil || opener.client == nil {
		return nil, fmt.Errorf("GetObject stream client is unavailable")
	}
	output, err := opener.client.GetObject(&obs.GetObjectInput{
		GetObjectMetadataInput: obs.GetObjectMetadataInput{
			Bucket: bucket,
			Key:    objectKey,
		},
	})
	if err != nil {
		if output != nil && output.Body != nil {
			_ = output.Body.Close()
		}
		return nil, err
	}
	if output == nil || output.Body == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	if err := ctx.Err(); err != nil {
		_ = output.Body.Close()
		return nil, err
	}
	return output.Body, nil
}

var (
	_ storage.Storage          = (*huaweiOBSClient)(nil)
	_ storage.StreamingStorage = (*huaweiOBSClient)(nil)
	_ storage.ReadinessChecker = (*huaweiOBSClient)(nil)
)

func New(ctx context.Context, ak, sk, bucketName, endpoint, region string) (storage.Storage, error) {
	cfg := domain.PublicConfig{Bucket: bucketName, Endpoint: endpoint, Region: region}
	credential := domain.CredentialInput{AccessKeyID: ak, SecretAccessKey: sk}
	return NewFromConfig(ctx, cfg, credential)
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	return NewFromConfigWithMode(ctx, cfg, credential, domain.ValidationMode{})
}

func NewFromConfigWithMode(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput, mode domain.ValidationMode) (storage.Storage, error) {
	normalized, err := domain.ValidatePublicConfig(domain.ProviderHuaweiOBS, cfg, mode)
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
	return getHuaweiOBSClient(ctx, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, normalized.Endpoint, normalized.Region)
}

func getHuaweiOBSClient(ctx context.Context, ak, sk, bucketName, endpoint, region string) (*huaweiOBSClient, error) {
	return getHuaweiOBSClientWithReadinessHTTPClient(ctx, ak, sk, bucketName, endpoint, region, nil)
}

func getHuaweiOBSClientWithReadinessHTTPClient(ctx context.Context, ak, sk, bucketName, endpoint, region string, readinessHTTPClient *http.Client) (*huaweiOBSClient, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	client, err := obs.New(ak, sk, endpoint, obs.WithRegion(region))
	if err != nil {
		return nil, err
	}
	c := &huaweiOBSClient{
		client:       client,
		streamOpener: &huaweiOBSSDKObjectStreamOpener{client: client},
		readinessCheck: func(checkCtx context.Context) error {
			readinessClient, err := newHuaweiOBSReadinessClient(checkCtx, ak, sk, endpoint, region, readinessHTTPClient)
			if err != nil {
				return err
			}
			defer readinessClient.Close()
			_, err = readinessClient.HeadBucket(bucketName)
			return err
		},
		bucketName: bucketName,
	}
	return c, nil
}

func newHuaweiOBSReadinessClient(ctx context.Context, ak, sk, endpoint, region string, httpClient *http.Client) (*obs.ObsClient, error) {
	if httpClient != nil {
		return obs.New(
			ak,
			sk,
			endpoint,
			obs.WithRegion(region),
			obs.WithRequestContext(ctx),
			obs.WithMaxRetryCount(0),
			obs.WithHttpClient(httpClient),
		)
	}
	return obs.New(
		ak,
		sk,
		endpoint,
		obs.WithRegion(region),
		obs.WithRequestContext(ctx),
		obs.WithMaxRetryCount(0),
	)
}

func (c *huaweiOBSClient) CheckReadiness(ctx context.Context) error {
	if ctx == nil || c == nil || c.readinessCheck == nil || strings.TrimSpace(c.bucketName) == "" {
		return storage.ErrReadinessUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.readinessCheck(ctx); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return storage.ErrReadinessUnavailable
	}
	return ctx.Err()
}

func (c *huaweiOBSClient) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return c.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (c *huaweiOBSClient) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if c == nil || c.client == nil {
		return fmt.Errorf("PutObject client is unavailable")
	}
	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}
	if option.ObjectSize < 0 {
		return fmt.Errorf("object size must be non-negative")
	}
	input := &obs.PutObjectInput{
		PutObjectBasicInput: obs.PutObjectBasicInput{
			ObjectOperationInput: obs.ObjectOperationInput{
				Bucket: c.bucketName,
				Key:    objectKey,
			},
			ContentLength: option.ObjectSize,
		},
		Body: content,
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
		input.HttpExpires = option.Expires.UTC().Format(http.TimeFormat)
	}
	_, err := c.client.PutObject(input)
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (c *huaweiOBSClient) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
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

func (c *huaweiOBSClient) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if err := ctxErr(ctx); err != nil {
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

func (c *huaweiOBSClient) DeleteObject(ctx context.Context, objectKey string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if c == nil || c.client == nil {
		return fmt.Errorf("DeleteObject client is unavailable")
	}
	_, err := c.client.DeleteObject(&obs.DeleteObjectInput{
		Bucket: c.bucketName,
		Key:    objectKey,
	})
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (c *huaweiOBSClient) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	if err := ctxErr(ctx); err != nil {
		return "", err
	}
	if c == nil || c.client == nil {
		return "", fmt.Errorf("GetObjectUrl client is unavailable")
	}
	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	expire := int64(24 * 60 * 60)
	if option.Expire > 0 {
		expire = option.Expire
	}
	query := make(map[string]string)
	if option.ResponseContentDisposition != "" {
		query["response-content-disposition"] = option.ResponseContentDisposition
	}
	if option.ResponseContentType != "" {
		query["response-content-type"] = option.ResponseContentType
	}
	output, err := c.client.CreateSignedUrl(&obs.CreateSignedUrlInput{
		Method:      obs.HttpMethodGet,
		Bucket:      c.bucketName,
		Key:         objectKey,
		Expires:     int(expire),
		QueryParams: query,
	})
	if err != nil {
		return "", fmt.Errorf("GetObjectUrl failed: %w", err)
	}
	if output == nil {
		return "", fmt.Errorf("GetObjectUrl returned an empty result")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return output.SignedUrl, nil
}

func (c *huaweiOBSClient) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("ListObjects client is unavailable")
	}
	listInput := &obs.ListObjectsInput{
		Bucket:       c.bucketName,
		Marker:       input.Cursor,
		EncodingType: "url",
	}
	listInput.Prefix = input.Prefix
	listInput.MaxKeys = input.PageSize
	output, err := c.client.ListObjects(listInput)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("list objects failed, err: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files := make([]*storage.FileInfo, 0, len(output.Contents))
	for _, obj := range output.Contents {
		files = append(files, &storage.FileInfo{
			Key:          obj.Key,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			Size:         obj.Size,
		})
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
				if err := ctxErr(ctx); err != nil {
					return err
				}
				tagging, err := c.client.GetObjectTagging(&obs.GetObjectTaggingInput{
					ObjectTaggingInput: obs.ObjectTaggingInput{
						Bucket: c.bucketName,
						Key:    f.Key,
					},
				})
				if err != nil {
					if contextErr := ctx.Err(); contextErr != nil {
						return contextErr
					}
					return err
				}
				f.Tagging = huaweiTagsToMap(tagging.Tags)
				return ctx.Err()
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
	return &storage.ListObjectsPaginatedOutput{
		Files:       files,
		Cursor:      output.NextMarker,
		IsTruncated: output.IsTruncated,
	}, nil
}

func (c *huaweiOBSClient) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
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

func (c *huaweiOBSClient) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("HeadObject client is unavailable")
	}
	output, err := c.client.GetObjectMetadata(&obs.GetObjectMetadataInput{
		Bucket: c.bucketName,
		Key:    objectKey,
	})
	if err != nil {
		if isHuaweiNotFound(err) {
			return nil, storage.ErrObjectNotFound
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fileInfo := &storage.FileInfo{
		Key:          objectKey,
		LastModified: output.LastModified,
		ETag:         output.ETag,
		Size:         output.ContentLength,
	}

	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	if option.WithTagging {
		tagging, err := c.client.GetObjectTagging(&obs.GetObjectTaggingInput{
			ObjectTaggingInput: obs.ObjectTaggingInput{
				Bucket: c.bucketName,
				Key:    objectKey,
			},
		})
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			return nil, err
		}
		fileInfo.Tagging = huaweiTagsToMap(tagging.Tags)
	}
	if option.WithURL {
		fileInfo.URL, err = c.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}
	return fileInfo, nil
}

func huaweiTagsToMap(tags []obs.Tag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, tag := range tags {
		m[tag.Key] = tag.Value
	}
	return m
}

func isHuaweiNotFound(err error) bool {
	var obsErr obs.ObsError
	if errors.As(err, &obsErr) {
		return obsErr.StatusCode == http.StatusNotFound || obsErr.Code == "NoSuchKey" || obsErr.Code == "NoSuchBucket"
	}
	return false
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is unavailable")
	}
	return ctx.Err()
}

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

package tencentcos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	cos "github.com/tencentyun/cos-go-sdk-v5"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/taskgroup"
)

type tencentCOSClient struct {
	client         *cos.Client
	streamOpener   tencentCOSObjectStreamOpener
	readinessCheck func(context.Context) error
	accessKeyID    string
	secretKey      string
	bucketName     string
}

type tencentCOSObjectStreamOpener interface {
	OpenObjectStream(context.Context, string) (io.ReadCloser, error)
}

type tencentCOSSDKObjectStreamOpener struct{ client *cos.Client }

var _ tencentCOSObjectStreamOpener = (*tencentCOSSDKObjectStreamOpener)(nil)

func (opener *tencentCOSSDKObjectStreamOpener) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if opener == nil || opener.client == nil {
		return nil, fmt.Errorf("GetObject stream client is unavailable")
	}
	resp, err := opener.client.Object.Get(ctx, objectKey, nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("GetObject returned an empty stream")
	}
	return resp.Body, nil
}

var (
	_ storage.Storage          = (*tencentCOSClient)(nil)
	_ storage.StreamingStorage = (*tencentCOSClient)(nil)
	_ storage.ReadinessChecker = (*tencentCOSClient)(nil)
)

func New(ctx context.Context, ak, sk, bucketName, endpointOverride, region string) (storage.Storage, error) {
	return getTencentCOSClient(ctx, ak, sk, bucketName, endpointOverride, region)
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	return NewFromConfigWithMode(ctx, cfg, credential, domain.ValidationMode{})
}

func NewFromConfigWithMode(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput, mode domain.ValidationMode) (storage.Storage, error) {
	normalized, err := domain.ValidatePublicConfig(domain.ProviderTencentCOS, cfg, mode)
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
	if normalized.EndpointOverride != "" && !mode.AllowHTTP && !strings.HasPrefix(strings.ToLower(normalized.EndpointOverride), "https://") {
		return nil, fmt.Errorf("%w: endpoint override must use https", domain.ErrConfigInvalid)
	}
	return getTencentCOSClient(ctx, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, normalized.EndpointOverride, normalized.Region)
}

func getTencentCOSClient(ctx context.Context, ak, sk, bucketName, endpointOverride, region string) (*tencentCOSClient, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	bucketURL, err := tencentBucketURL(bucketName, endpointOverride, region)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  ak,
			SecretKey: sk,
		},
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, httpClient)
	c := &tencentCOSClient{
		client:       client,
		streamOpener: &tencentCOSSDKObjectStreamOpener{client: client},
		readinessCheck: func(ctx context.Context) error {
			_, err := client.Bucket.Head(ctx)
			return err
		},
		accessKeyID: ak,
		secretKey:   sk,
		bucketName:  bucketName,
	}
	return c, nil
}

func (c *tencentCOSClient) CheckReadiness(ctx context.Context) error {
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
	return nil
}

func (c *tencentCOSClient) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return c.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (c *tencentCOSClient) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}
	cosOpts := &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{}}
	if option.ContentType != nil {
		cosOpts.ContentType = *option.ContentType
	}
	if option.ContentEncoding != nil {
		cosOpts.ContentEncoding = *option.ContentEncoding
	}
	if option.ContentDisposition != nil {
		cosOpts.ContentDisposition = *option.ContentDisposition
	}
	if option.ContentLanguage != nil {
		cosOpts.ContentLanguage = *option.ContentLanguage
	}
	if option.Expires != nil {
		cosOpts.Expires = option.Expires.UTC().Format(http.TimeFormat)
	}
	if option.ObjectSize > 0 {
		cosOpts.ContentLength = option.ObjectSize
	}
	_, err := c.client.Object.Put(ctx, objectKey, content, cosOpts)
	return err
}

func (c *tencentCOSClient) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
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

func (c *tencentCOSClient) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("GetObject context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.streamOpener == nil {
		return nil, fmt.Errorf("GetObject stream opener is unavailable")
	}
	body, err := c.streamOpener.OpenObjectStream(ctx, objectKey)
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

func (c *tencentCOSClient) DeleteObject(ctx context.Context, objectKey string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	_, err := c.client.Object.Delete(ctx, objectKey)
	return err
}

func (c *tencentCOSClient) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
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
	getOpts := &cos.ObjectGetOptions{
		ResponseContentDisposition: option.ResponseContentDisposition,
		ResponseContentType:        option.ResponseContentType,
	}
	signed, err := c.client.Object.GetPresignedURL(ctx, http.MethodGet, objectKey, c.accessKeyID, c.secretKey, time.Duration(expire)*time.Second, getOpts)
	if err != nil {
		return "", fmt.Errorf("GetObjectUrl failed: %w", err)
	}
	return signed.String(), nil
}

func (c *tencentCOSClient) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}
	output, _, err := c.client.Bucket.Get(ctx, &cos.BucketGetOptions{
		Prefix:  input.Prefix,
		Marker:  input.Cursor,
		MaxKeys: input.PageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("list objects failed, err: %w", err)
	}
	files := make([]*storage.FileInfo, 0, len(output.Contents))
	for _, obj := range output.Contents {
		files = append(files, &storage.FileInfo{
			Key:          obj.Key,
			LastModified: parseCOSLastModified(obj.LastModified),
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
				tagging, _, err := c.client.Object.GetTagging(ctx, f.Key)
				if err != nil {
					return err
				}
				f.Tagging = tencentTagsToMap(tagging.TagSet)
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
	return &storage.ListObjectsPaginatedOutput{
		Files:       files,
		Cursor:      output.NextMarker,
		IsTruncated: output.IsTruncated,
	}, nil
}

func (c *tencentCOSClient) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
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

func (c *tencentCOSClient) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	resp, err := c.client.Object.Head(ctx, objectKey, nil)
	if err != nil {
		if cos.IsNotFoundError(err) {
			return nil, storage.ErrObjectNotFound
		}
		return nil, err
	}
	fileInfo := &storage.FileInfo{Key: objectKey}
	if resp != nil {
		fileInfo.LastModified = parseHTTPTime(resp.Header.Get("Last-Modified"))
		fileInfo.ETag = resp.Header.Get("ETag")
		if size, parseErr := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64); parseErr == nil {
			fileInfo.Size = size
		}
	}

	option := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&option)
	}
	if option.WithTagging {
		tagging, _, err := c.client.Object.GetTagging(ctx, objectKey)
		if err != nil {
			return nil, err
		}
		fileInfo.Tagging = tencentTagsToMap(tagging.TagSet)
	}
	if option.WithURL {
		fileInfo.URL, err = c.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}
	return fileInfo, nil
}

func tencentBucketURL(bucketName, endpointOverride, region string) (*url.URL, error) {
	if endpointOverride != "" {
		parsed, err := url.Parse(endpointOverride)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("%w: endpoint override is invalid", domain.ErrConfigInvalid)
		}
		return parsed, nil
	}
	return cos.NewBucketURL(bucketName, region, true)
}

func parseCOSLastModified(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t
	}
	return parseHTTPTime(value)
}

func parseHTTPTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := http.ParseTime(value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func tencentTagsToMap(tags []cos.ObjectTaggingTag) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, tag := range tags {
		m[tag.Key] = tag.Value
	}
	return m
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("context is unavailable")
	}
	return ctx.Err()
}

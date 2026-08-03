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

package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/fileutil"
	"github.com/coze-dev/coze-studio/backend/pkg/goutil"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/taskgroup"
)

type s3Client struct {
	client         *s3.Client
	streamOpener   s3ObjectStreamOpener
	readinessCheck func(context.Context, string) error
	bucketName     string
}

type s3ObjectStreamOpener interface {
	OpenObjectStream(context.Context, string, string) (io.ReadCloser, error)
}

type s3SDKObjectStreamOpener struct{ client *s3.Client }

var _ s3ObjectStreamOpener = (*s3SDKObjectStreamOpener)(nil)

func (opener *s3SDKObjectStreamOpener) OpenObjectStream(ctx context.Context, bucket, objectKey string) (io.ReadCloser, error) {
	if opener == nil || opener.client == nil {
		return nil, fmt.Errorf("get object stream client is unavailable")
	}
	result, err := opener.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(objectKey)})
	if err != nil {
		return nil, fmt.Errorf("get object failed: %w", err)
	}
	if result == nil || result.Body == nil {
		return nil, fmt.Errorf("get object returned an empty stream")
	}
	return result.Body, nil
}

var (
	_ storage.StreamingStorage = (*s3Client)(nil)
	_ storage.ReadinessChecker = (*s3Client)(nil)
)

func New(ctx context.Context, ak, sk, bucketName, endpoint, region string) (storage.Storage, error) {
	t, err := getS3Client(ctx, ak, sk, bucketName, endpoint, region)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error) {
	return NewFromConfigWithMode(ctx, cfg, credential, domain.ValidationMode{})
}

func NewFromConfigWithMode(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput, mode domain.ValidationMode) (storage.Storage, error) {
	runtimeConfig := cfg
	runtimeConfig.Endpoint = ""
	normalized, err := domain.ValidatePublicConfig(domain.ProviderAWSS3, runtimeConfig, mode)
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
	return getS3ClientWithOptions(ctx, credential.AccessKeyID, credential.SecretAccessKey, normalized.Bucket, endpoint, normalized.Region, normalized.ForcePathStyle, false, normalized.Region)
}

func getS3Client(ctx context.Context, ak, sk, bucketName, endpoint, region string) (*s3Client, error) {
	return getS3ClientWithOptions(ctx, ak, sk, bucketName, endpoint, region, false, true, "auto")
}

func getS3ClientWithOptions(ctx context.Context, ak, sk, bucketName, endpoint, region string, forcePathStyle bool, createBucket bool, configRegion string) (*s3Client, error) {
	creds := credentials.NewStaticCredentialsProvider(ak, sk, "")
	configOptions := []func(*config.LoadOptions) error{
		config.WithCredentialsProvider(creds),
		config.WithRegion(configRegion),
	}
	if endpoint != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               endpoint,
				SigningRegion:     region,
				HostnameImmutable: false,
				Source:            aws.EndpointSourceCustom,
			}, nil
		})
		configOptions = append(configOptions, config.WithEndpointResolverWithOptions(customResolver))
	}
	cfg, err := config.LoadDefaultConfig(
		ctx,
		configOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("init config failed, bucketName: %s, endpoint: %s, region: %s, err: %v", bucketName, endpoint, region, err)
	}

	c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = forcePathStyle
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})

	t := &s3Client{
		client:       c,
		streamOpener: &s3SDKObjectStreamOpener{client: c},
		readinessCheck: func(ctx context.Context, bucket string) error {
			_, err := c.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
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

func (t *s3Client) CheckReadiness(ctx context.Context) error {
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

func (t *s3Client) CheckAndCreateBucket(ctx context.Context) error {
	client := t.client
	bucket := t.bucketName

	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return nil // already exist
	}

	awsErr, ok := err.(interface{ ErrorCode() string })
	if !ok || awsErr.ErrorCode() != "404" {
		return err
	}

	// bucket not exist
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}
	_, err = client.CreateBucket(ctx, input)
	return err
}

func (t *s3Client) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	opts = append(opts, storage.WithObjectSize(int64(len(content))))
	return t.PutObjectWithReader(ctx, objectKey, bytes.NewReader(content), opts...)
}

func (t *s3Client) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	client := t.client
	bucket := t.bucketName

	option := storage.PutOption{}
	for _, opt := range opts {
		opt(&option)
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
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
	if option.ContentLanguage != nil {
		input.ContentLanguage = option.ContentLanguage
	}
	if option.Expires != nil {
		input.Expires = option.Expires
	}

	if option.ObjectSize > 0 {
		input.ContentLength = aws.Int64(option.ObjectSize)
	}

	if option.Tagging != nil {
		input.Tagging = aws.String(goutil.MapToQuery(option.Tagging))
	}

	// upload object
	_, err := client.PutObject(ctx, input)
	return err
}

func (t *s3Client) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
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

func (t *s3Client) OpenObjectStream(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	if ctx == nil {
		return nil, fmt.Errorf("get object context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t == nil || t.streamOpener == nil {
		return nil, fmt.Errorf("get object stream opener is unavailable")
	}
	result, err := t.streamOpener.OpenObjectStream(ctx, t.bucketName, objectKey)
	if err != nil {
		return nil, fmt.Errorf("get object failed: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("get object returned an empty stream")
	}
	if err := ctx.Err(); err != nil {
		_ = result.Close()
		return nil, err
	}
	return result, nil
}

func (t *s3Client) DeleteObject(ctx context.Context, objectKey string) error {
	client := t.client
	bucket := t.bucketName

	_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})

	return err
}

func (t *s3Client) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	client := t.client
	bucket := t.bucketName
	presignClient := s3.NewPresignClient(client)

	opt := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&opt)
	}

	expire := int64(60 * 60 * 24)
	if opt.Expire > 0 {
		expire = opt.Expire
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	}
	if opt.ResponseContentDisposition != "" {
		input.ResponseContentDisposition = aws.String(
			opt.ResponseContentDisposition,
		)
	}
	if opt.ResponseContentType != "" {
		input.ResponseContentType = aws.String(opt.ResponseContentType)
	}
	if opt.ResponseCacheControl != "" {
		input.ResponseCacheControl = aws.String(opt.ResponseCacheControl)
	}

	req, err := presignClient.PresignGetObject(ctx, input, func(options *s3.PresignOptions) {
		options.Expires = time.Duration(expire) * time.Second
	})
	if err != nil {
		return "", fmt.Errorf("get object presigned url failed: %v", err)
	}

	return req.URL, nil
}

func (t *s3Client) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	const (
		DefaultPageSize = 100
		MaxListObjects  = 10000
	)

	var files []*storage.FileInfo
	var cursor string
	for {
		output, err := t.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{
			Prefix:   prefix,
			PageSize: DefaultPageSize,
			Cursor:   cursor,
		}, opts...)

		if err != nil {
			return nil, err
		}

		cursor = output.Cursor

		files = append(files, output.Files...)

		if len(files) >= MaxListObjects {
			logs.CtxErrorf(ctx, "list objects failed, max list objects: %d", MaxListObjects)
			break
		}

		if !output.IsTruncated {
			break
		}
	}

	return files, nil
}

func (t *s3Client) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input cannot be nil")
	}
	if input.PageSize <= 0 {
		return nil, fmt.Errorf("page size must be positive")
	}

	client := t.client
	bucket := t.bucketName

	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket:            aws.String(bucket),
		Prefix:            aws.String(input.Prefix),
		MaxKeys:           aws.Int32(int32(input.PageSize)),
		ContinuationToken: aws.String(input.Cursor),
	}

	p, err := client.ListObjectsV2(ctx, listObjectsInput)
	if err != nil {
		return nil, err
	}

	var files []*storage.FileInfo
	for _, obj := range p.Contents {
		f := &storage.FileInfo{}
		if obj.Key != nil {
			f.Key = *obj.Key
		}
		if obj.LastModified != nil {
			f.LastModified = *obj.LastModified
		}
		if obj.ETag != nil {
			f.ETag = *obj.ETag
		}
		if obj.Size != nil {
			f.Size = *obj.Size
		}
		files = append(files, f)
	}

	output := &storage.ListObjectsPaginatedOutput{
		Files: files,
	}
	if p.IsTruncated != nil {
		output.IsTruncated = *p.IsTruncated
	}
	if p.NextContinuationToken != nil {
		output.Cursor = *p.NextContinuationToken
	}

	opt := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&opt)
	}

	if opt.WithTagging {
		taskGroup := taskgroup.NewTaskGroup(ctx, 5)
		for idx := range files {
			f := files[idx]
			taskGroup.Go(func() error {
				tagging, err := client.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{
					Bucket: aws.String(bucket),
					Key:    aws.String(f.Key),
				})
				if err != nil {
					return err
				}

				f.Tagging = tagsToMap(tagging.TagSet)
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

	return output, nil
}

func (t *s3Client) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	obj, err := t.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(t.bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		var nsk *types.NotFound
		if errors.As(err, &nsk) {
			return nil, storage.ErrObjectNotFound
		}
		return nil, err
	}

	f := &storage.FileInfo{
		Key: objectKey,
	}
	if obj.LastModified != nil {
		f.LastModified = *obj.LastModified
	}

	if obj.ETag != nil {
		f.ETag = *obj.ETag
	}

	if obj.ContentLength != nil {
		f.Size = *obj.ContentLength
	}

	opt := storage.GetOption{}
	for _, optFn := range opts {
		optFn(&opt)
	}

	if opt.WithTagging {
		tagging, err := t.client.GetObjectTagging(ctx, &s3.GetObjectTaggingInput{
			Bucket: aws.String(t.bucketName),
			Key:    aws.String(objectKey),
		})
		if err != nil {
			return nil, err
		}

		f.Tagging = tagsToMap(tagging.TagSet)
	}

	if opt.WithURL {
		f.URL, err = t.GetObjectUrl(ctx, objectKey, opts...)
		if err != nil {
			return nil, err
		}
	}

	return f, nil
}

func tagsToMap(tags []types.Tag) map[string]string {
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

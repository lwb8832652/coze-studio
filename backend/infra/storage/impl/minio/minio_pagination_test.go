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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	miniov7 "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestListObjectsPaginatedBoundsSDKIteration(t *testing.T) {
	t.Parallel()

	sourceDone := make(chan struct{})
	var listedOptions miniov7.ListObjectsOptions
	client := &minioClient{
		bucketName: "bucket",
		listObjects: func(ctx context.Context, bucket string, opts miniov7.ListObjectsOptions) <-chan miniov7.ObjectInfo {
			listedOptions = opts
			objects := make(chan miniov7.ObjectInfo)
			go func() {
				defer close(sourceDone)
				defer close(objects)
				for _, key := range []string{"journal/002", "journal/003", "journal/004", "journal/005"} {
					select {
					case objects <- miniov7.ObjectInfo{Key: key}:
					case <-ctx.Done():
						objects <- miniov7.ObjectInfo{Err: ctx.Err()}
						return
					}
				}
			}()
			return objects
		},
	}

	page, err := client.ListObjectsPaginated(context.Background(), &storage.ListObjectsPaginatedInput{
		Prefix:   "journal/",
		PageSize: 2,
		Cursor:   "journal/001",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"journal/002", "journal/003"}, []string{
		page.Files[0].Key,
		page.Files[1].Key,
	})
	require.True(t, page.IsTruncated)
	require.Equal(t, "journal/003", page.Cursor)
	require.Equal(t, "journal/", listedOptions.Prefix)
	require.Equal(t, "journal/001", listedOptions.StartAfter)
	require.Equal(t, 3, listedOptions.MaxKeys)
	require.True(t, listedOptions.Recursive)

	select {
	case <-sourceDone:
	case <-time.After(time.Second):
		t.Fatal("list source did not stop after the bounded page was collected")
	}
}

func TestListObjectsPaginatedLeavesCursorEmptyForFinalPage(t *testing.T) {
	t.Parallel()

	client := &minioClient{
		bucketName: "bucket",
		listObjects: func(_ context.Context, _ string, _ miniov7.ListObjectsOptions) <-chan miniov7.ObjectInfo {
			objects := make(chan miniov7.ObjectInfo, 2)
			objects <- miniov7.ObjectInfo{Key: "journal/001"}
			objects <- miniov7.ObjectInfo{Key: "journal/002"}
			close(objects)
			return objects
		},
	}

	page, err := client.ListObjectsPaginated(context.Background(), &storage.ListObjectsPaginatedInput{
		Prefix:   "journal/",
		PageSize: 2,
	})
	require.NoError(t, err)
	require.Len(t, page.Files, 2)
	require.False(t, page.IsTruncated)
	require.Empty(t, page.Cursor)
}

func TestListObjectsPaginatedRejectsUnboundedPageSizesBeforeListing(t *testing.T) {
	t.Parallel()

	listed := false
	client := &minioClient{
		bucketName: "bucket",
		listObjects: func(context.Context, string, miniov7.ListObjectsOptions) <-chan miniov7.ObjectInfo {
			listed = true
			return nil
		},
	}
	for _, pageSize := range []int{0, -1, maxMinIOListPageSize + 1, int(^uint(0) >> 1)} {
		_, err := client.ListObjectsPaginated(
			context.Background(),
			&storage.ListObjectsPaginatedInput{PageSize: pageSize},
		)
		require.Error(t, err)
	}
	require.False(t, listed)
}

func TestListObjectsPaginatedDrainsAfterProviderError(t *testing.T) {
	t.Parallel()

	providerErr := errors.New("provider failed")
	producerDone := make(chan struct{})
	client := &minioClient{
		bucketName: "bucket",
		listObjects: func(context.Context, string, miniov7.ListObjectsOptions) <-chan miniov7.ObjectInfo {
			objects := make(chan miniov7.ObjectInfo)
			go func() {
				defer close(producerDone)
				defer close(objects)
				objects <- miniov7.ObjectInfo{Err: providerErr}
				objects <- miniov7.ObjectInfo{Key: "tail-write-must-be-drained"}
			}()
			return objects
		},
	}

	_, err := client.ListObjectsPaginated(
		context.Background(),
		&storage.ListObjectsPaginatedInput{PageSize: 2},
	)
	require.ErrorIs(t, err, providerErr)
	select {
	case <-producerDone:
	case <-time.After(time.Second):
		t.Fatal("list source remained blocked after provider error")
	}
}

func TestListObjectsPaginatedPreservesCallerCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	client := &minioClient{
		bucketName: "bucket",
		listObjects: func(ctx context.Context, _ string, _ miniov7.ListObjectsOptions) <-chan miniov7.ObjectInfo {
			objects := make(chan miniov7.ObjectInfo, 1)
			go func() {
				defer close(objects)
				<-ctx.Done()
				objects <- miniov7.ObjectInfo{Err: ctx.Err()}
			}()
			return objects
		},
	}
	cancel()

	_, err := client.ListObjectsPaginated(
		ctx,
		&storage.ListObjectsPaginatedInput{PageSize: 2},
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestListObjectsPaginatedBoundsRealMinIOSDKAutoPagination(t *testing.T) {
	var requestCount atomic.Int32
	transport := minIORoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requestCount.Add(1)
		if r.URL.Query().Get("list-type") != "2" ||
			r.URL.Query().Get("max-keys") != "3" {
			return nil, fmt.Errorf("unexpected list query: %s", r.URL.RawQuery)
		}
		continuation := r.URL.Query().Get("continuation-token")
		var body string
		if continuation == "" {
			if r.URL.Query().Get("start-after") != "journal/000" {
				return nil, fmt.Errorf("unexpected start-after cursor")
			}
			body = minIOListObjectsV2Response(
				[]string{"journal/001", "journal/002"},
				true,
				"next-token",
			)
		} else {
			if continuation != "next-token" {
				return nil, fmt.Errorf("unexpected continuation token: %s", continuation)
			}
			body = minIOListObjectsV2Response(
				[]string{"journal/003", "journal/004"},
				false,
				"",
			)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/xml"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})

	sdk, err := miniov7.New("minio.example.test", &miniov7.Options{
		Creds:     credentials.NewStaticV4("access", "secret", ""),
		Secure:    false,
		Region:    "us-east-1",
		Transport: transport,
	})
	require.NoError(t, err)
	client := &minioClient{
		client: sdk, bucketName: "bucket", listObjects: sdk.ListObjects,
	}

	page, err := client.ListObjectsPaginated(
		context.Background(),
		&storage.ListObjectsPaginatedInput{
			Prefix: "journal/", Cursor: "journal/000", PageSize: 2,
		},
	)
	require.NoError(t, err)
	require.Equal(t, []string{"journal/001", "journal/002"}, []string{
		page.Files[0].Key,
		page.Files[1].Key,
	})
	require.True(t, page.IsTruncated)
	require.Equal(t, "journal/002", page.Cursor)
	require.Equal(t, int32(2), requestCount.Load(), "the SDK must auto-page once for lookahead")
}

type minIORoundTripperFunc func(*http.Request) (*http.Response, error)

func (f minIORoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func minIOListObjectsV2Response(keys []string, truncated bool, nextToken string) string {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	body.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	body.WriteString(`<Name>bucket</Name><Prefix>journal/</Prefix><MaxKeys>3</MaxKeys>`)
	body.WriteString(fmt.Sprintf("<KeyCount>%d</KeyCount><IsTruncated>%t</IsTruncated>", len(keys), truncated))
	if nextToken != "" {
		body.WriteString("<NextContinuationToken>" + nextToken + "</NextContinuationToken>")
	}
	for index, key := range keys {
		body.WriteString("<Contents><Key>" + key + "</Key>")
		body.WriteString(`<LastModified>2026-07-30T00:00:00.000Z</LastModified>`)
		body.WriteString(fmt.Sprintf("<ETag>&quot;etag-%d&quot;</ETag><Size>1</Size>", index))
		body.WriteString(`<StorageClass>STANDARD</StorageClass></Contents>`)
	}
	body.WriteString(`</ListBucketResult>`)
	return body.String()
}

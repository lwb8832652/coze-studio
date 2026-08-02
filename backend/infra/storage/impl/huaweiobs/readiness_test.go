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
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/contract"
)

type huaweiOBSReadinessRecorder struct {
	contract.ReadinessRecorder
	err error
}

func (r *huaweiOBSReadinessRecorder) HeadBucket(context.Context) error {
	r.HeadBucketCalls++
	return r.err
}

func TestCheckReadinessCanceledContextDoesNotCallHuaweiSDK(t *testing.T) {
	recorder := &huaweiOBSReadinessRecorder{}
	client := &huaweiOBSClient{bucketName: "bucket", readinessCheck: recorder.HeadBucket}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.CheckReadiness(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckReadiness(canceled) error = %v", err)
	}
	if recorder.HeadBucketCalls != 0 {
		t.Fatalf("HeadBucketCalls = %d, want 0", recorder.HeadBucketCalls)
	}
}

func TestHuaweiOBSReadinessCancelsInFlightSDKRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-request.Context().Done():
			return nil, request.Context().Err()
		case <-release:
			return nil, errors.New("test request released")
		}
	})}

	ctx, cancel := context.WithCancel(context.Background())
	client, err := getHuaweiOBSClientWithReadinessHTTPClient(ctx, "ak", "sk", "bucket", "http://obs.example.test", "", httpClient)
	if err != nil {
		t.Fatalf("getHuaweiOBSClient() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- client.CheckReadiness(ctx) }()

	<-started
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("CheckReadiness(canceled in flight) error = %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		unblock()
		<-done
		t.Fatal("CheckReadiness did not cancel the in-flight Huawei OBS request")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCheckReadinessMapsHuaweiSDKError(t *testing.T) {
	recorder := &huaweiOBSReadinessRecorder{err: errors.New("sdk unavailable")}
	client := &huaweiOBSClient{bucketName: "bucket", readinessCheck: recorder.HeadBucket}

	err := client.CheckReadiness(context.Background())

	if !errors.Is(err, storage.ErrReadinessUnavailable) {
		t.Fatalf("CheckReadiness(sdk error) error = %v", err)
	}
	contract.AssertReadinessIsReadOnly(t, recorder.ReadinessRecorder)
}

func TestHuaweiOBSReadinessIsReadOnly(t *testing.T) {
	recorder := &huaweiOBSReadinessRecorder{}
	client := &huaweiOBSClient{bucketName: "bucket", readinessCheck: recorder.HeadBucket}

	if err := client.CheckReadiness(context.Background()); err != nil {
		t.Fatalf("CheckReadiness() error = %v", err)
	}
	contract.AssertReadinessIsReadOnly(t, recorder.ReadinessRecorder)
}

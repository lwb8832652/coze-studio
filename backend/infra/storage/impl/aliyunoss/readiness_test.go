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
	"context"
	"errors"
	"testing"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/contract"
)

type aliyunOSSReadinessRecorder struct {
	contract.ReadinessRecorder
	err error
}

func (r *aliyunOSSReadinessRecorder) GetBucketInfo(context.Context, string) error {
	r.HeadBucketCalls++
	return r.err
}

func TestCheckReadinessCanceledContextDoesNotCallAliyunSDK(t *testing.T) {
	recorder := &aliyunOSSReadinessRecorder{}
	client := &aliyunOSSClient{bucketName: "bucket", readinessCheck: recorder.GetBucketInfo}
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

func TestCheckReadinessMapsAliyunSDKError(t *testing.T) {
	recorder := &aliyunOSSReadinessRecorder{err: errors.New("sdk unavailable")}
	client := &aliyunOSSClient{bucketName: "bucket", readinessCheck: recorder.GetBucketInfo}

	err := client.CheckReadiness(context.Background())

	if !errors.Is(err, storage.ErrReadinessUnavailable) {
		t.Fatalf("CheckReadiness(sdk error) error = %v", err)
	}
	contract.AssertReadinessIsReadOnly(t, recorder.ReadinessRecorder)
}

func TestAliyunOSSReadinessIsReadOnly(t *testing.T) {
	recorder := &aliyunOSSReadinessRecorder{}
	client := &aliyunOSSClient{bucketName: "bucket", readinessCheck: recorder.GetBucketInfo}

	if err := client.CheckReadiness(context.Background()); err != nil {
		t.Fatalf("CheckReadiness() error = %v", err)
	}
	contract.AssertReadinessIsReadOnly(t, recorder.ReadinessRecorder)
}

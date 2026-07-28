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
	"testing"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/contract"
)

type huaweiOBSReadinessRecorder struct {
	contract.ReadinessRecorder
	err error
}

func (r *huaweiOBSReadinessRecorder) HeadBucket() error {
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

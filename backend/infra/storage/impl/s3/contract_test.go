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
	"context"
	"os"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/contract"
)

func TestS3ContractWithExternalEndpoint(t *testing.T) {
	if os.Getenv("COZE_STORAGE_CONTRACT_AWS_S3") != "1" {
		t.Skip("set COZE_STORAGE_CONTRACT_AWS_S3=1 to run")
	}

	contract.RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
		t.Helper()
		client, err := NewFromConfig(context.Background(), domain.PublicConfig{
			Bucket:           os.Getenv("AWS_S3_CONTRACT_BUCKET"),
			Region:           os.Getenv("AWS_S3_CONTRACT_REGION"),
			EndpointOverride: os.Getenv("AWS_S3_CONTRACT_ENDPOINT_OVERRIDE"),
			ForcePathStyle:   os.Getenv("AWS_S3_CONTRACT_FORCE_PATH_STYLE") == "true",
		}, domain.CredentialInput{
			AccessKeyID:     os.Getenv("AWS_S3_CONTRACT_AK"),
			SecretAccessKey: os.Getenv("AWS_S3_CONTRACT_SK"),
		})
		if err != nil {
			t.Fatalf("NewFromConfig() error = %v", err)
		}
		return client
	})
}

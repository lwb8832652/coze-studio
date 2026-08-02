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
	"os"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/infra/storage/impl/internal/contract"
)

func TestMinIOContractWithExternalEndpoint(t *testing.T) {
	if os.Getenv("COZE_STORAGE_CONTRACT_MINIO") != "1" {
		t.Skip("set COZE_STORAGE_CONTRACT_MINIO=1 to run")
	}

	contract.RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
		t.Helper()
		client, err := NewFromConfig(context.Background(), domain.PublicConfig{
			Bucket:   os.Getenv("MINIO_CONTRACT_BUCKET"),
			Endpoint: os.Getenv("MINIO_CONTRACT_ENDPOINT"),
			UseSSL:   os.Getenv("MINIO_CONTRACT_USE_SSL") == "true",
		}, domain.CredentialInput{
			AccessKeyID:     os.Getenv("MINIO_CONTRACT_AK"),
			SecretAccessKey: os.Getenv("MINIO_CONTRACT_SK"),
		})
		if err != nil {
			t.Fatalf("NewFromConfig() error = %v", err)
		}
		return client
	})
}

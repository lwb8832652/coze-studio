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

package contract

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestRunStorageLifecycleAllowsEmptyHeadObjectURL(t *testing.T) {
	fake := newContractFakeStorage()
	fake.headURL = ""

	RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
		t.Helper()
		return fake
	})

	if !fake.headWithURL {
		t.Fatal("RunStorageLifecycle did not call HeadObject with URL")
	}
}

func TestRunStorageLifecycleDoesNotPrintSignedURLSecret(t *testing.T) {
	if os.Getenv("COZE_STORAGE_CONTRACT_LEAK_PROBE") == "1" {
		fake := newContractFakeStorage()
		fake.signedURL = "https://signed.example.com/?secret=leak-marker"
		RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
			t.Helper()
			return fake
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestRunStorageLifecycleDoesNotPrintSignedURLSecret$")
	cmd.Env = append(os.Environ(), "COZE_STORAGE_CONTRACT_LEAK_PROBE=1")

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("leak probe passed, want signed URL validation failure")
	}
	if strings.Contains(string(output), "leak-marker") {
		t.Fatal("RunStorageLifecycle printed signed URL secret marker")
	}
}

func TestRunStorageLifecycleUsesUniqueObjectKey(t *testing.T) {
	fake := newContractFakeStorage()

	RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
		t.Helper()
		return fake
	})
	RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
		t.Helper()
		return fake
	})

	if len(fake.putKeys) != 2 {
		t.Fatalf("PutObject calls = %d, want 2", len(fake.putKeys))
	}
	if fake.putKeys[0] == fake.putKeys[1] {
		t.Fatal("RunStorageLifecycle reused object key")
	}
}

type contractFakeStorage struct {
	key         string
	putKeys     []string
	body        []byte
	headURL     string
	signedURL   string
	headWithURL bool
	deleted     bool
}

func newContractFakeStorage() *contractFakeStorage {
	return &contractFakeStorage{
		headURL: "https://signed.example.com/contract/object-storage-lifecycle.txt",
	}
}

func (f *contractFakeStorage) PutObject(ctx context.Context, objectKey string, content []byte, opts ...storage.PutOptFn) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.key = objectKey
	f.putKeys = append(f.putKeys, objectKey)
	f.body = bytes.Clone(content)
	f.deleted = false
	return nil
}

func (f *contractFakeStorage) PutObjectWithReader(ctx context.Context, objectKey string, content io.Reader, opts ...storage.PutOptFn) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	return f.PutObject(ctx, objectKey, body, opts...)
}

func (f *contractFakeStorage) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return bytes.Clone(f.body), nil
}

func (f *contractFakeStorage) DeleteObject(ctx context.Context, objectKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.deleted = true
	return nil
}

func (f *contractFakeStorage) GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f.signedURL == "" {
		return "https://signed.example.com/" + objectKey, nil
	}
	return f.signedURL, nil
}

func (f *contractFakeStorage) HeadObject(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (*storage.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}
	f.headWithURL = option.WithURL
	return &storage.FileInfo{Key: objectKey, LastModified: time.Now(), Size: int64(len(f.body)), URL: f.headURL}, nil
}

func (f *contractFakeStorage) ListAllObjects(ctx context.Context, prefix string, opts ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []*storage.FileInfo{{Key: f.key, Size: int64(len(f.body))}}, nil
}

func (f *contractFakeStorage) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &storage.ListObjectsPaginatedOutput{
		Files: []*storage.FileInfo{{Key: f.key, Size: int64(len(f.body))}},
	}, nil
}

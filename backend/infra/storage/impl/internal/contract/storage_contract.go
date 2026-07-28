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
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type StorageFactory func(t *testing.T) storage.Storage

var storageLifecycleSequence atomic.Uint64

func RunStorageLifecycle(t *testing.T, factory StorageFactory) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := factory(t)
	key := fmt.Sprintf("contract/object-storage-lifecycle-%d-%d.txt", time.Now().UnixNano(), storageLifecycleSequence.Add(1))
	body := []byte("object storage contract body")
	uploaded := false
	defer func() {
		if uploaded {
			_ = client.DeleteObject(context.Background(), key)
		}
	}()

	if err := client.PutObject(ctx, key, body, storage.WithContentType("text/plain")); err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}
	uploaded = true

	got, err := client.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("GetObject() = %q", string(got))
	}

	info, err := client.HeadObject(ctx, key, storage.WithURL(true))
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if info == nil || info.Key == "" {
		t.Fatal("HeadObject() returned empty metadata")
	}

	list, err := client.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{Prefix: "contract/", PageSize: 20})
	if err != nil {
		t.Fatalf("ListObjectsPaginated() error = %v", err)
	}
	if list == nil {
		t.Fatal("ListObjectsPaginated() returned nil")
	}

	signed, err := client.GetObjectUrl(ctx, key, storage.WithExpire(int64(time.Minute/time.Second)))
	if err != nil {
		t.Fatalf("GetObjectUrl() error = %v", err)
	}
	if !strings.Contains(signed, key) {
		t.Fatal("signed URL does not reference object key")
	}

	if err = client.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject() error = %v", err)
	}
	uploaded = false
}

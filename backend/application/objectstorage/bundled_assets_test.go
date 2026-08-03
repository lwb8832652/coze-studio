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

package objectstorage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestEnsureBundledAssetsUploadsOnlyMissingFiles(t *testing.T) {
	root := t.TempDir()
	writeBundledAsset(t, root, "default_icon/workflow_icon/icon-llm.jpg", "llm")
	writeBundledAsset(t, root, "official_plugin_icon/plugin-search.png", "search")

	client := &bundledAssetStorage{
		existing: map[string]bool{
			"default_icon/workflow_icon/icon-llm.jpg": true,
		},
	}

	result, err := EnsureBundledAssets(context.Background(), client, root)
	if err != nil {
		t.Fatalf("EnsureBundledAssets() error = %v", err)
	}
	if result.Checked != 2 || result.Uploaded != 1 {
		t.Fatalf("EnsureBundledAssets() result = %+v", result)
	}
	if got := string(client.uploaded["official_plugin_icon/plugin-search.png"]); got != "search" {
		t.Fatalf("uploaded plugin content = %q", got)
	}
	if _, overwritten := client.uploaded["default_icon/workflow_icon/icon-llm.jpg"]; overwritten {
		t.Fatal("existing default icon was overwritten")
	}
}

func TestEnsureBundledAssetsFailsClosedOnStorageInspectionError(t *testing.T) {
	root := t.TempDir()
	writeBundledAsset(t, root, "default_icon/workflow_icon/icon-start.jpg", "start")
	client := &bundledAssetStorage{headErr: errors.New("storage unavailable")}

	_, err := EnsureBundledAssets(context.Background(), client, root)
	if err == nil || !errors.Is(err, client.headErr) {
		t.Fatalf("EnsureBundledAssets() error = %v", err)
	}
	if len(client.uploaded) != 0 {
		t.Fatalf("uploaded after inspection failure = %v", client.uploaded)
	}
}

func TestEnsureBundledAssetsRejectsMissingRoot(t *testing.T) {
	client := &bundledAssetStorage{}

	_, err := EnsureBundledAssets(context.Background(), client, filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("EnsureBundledAssets() unexpectedly accepted a missing asset root")
	}
}

func writeBundledAsset(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

type bundledAssetStorage struct {
	existing map[string]bool
	headErr  error
	uploaded map[string][]byte
}

func (s *bundledAssetStorage) PutObject(_ context.Context, key string, content []byte, _ ...storage.PutOptFn) error {
	if s.uploaded == nil {
		s.uploaded = make(map[string][]byte)
	}
	s.uploaded[key] = append([]byte(nil), content...)
	return nil
}

func (*bundledAssetStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return errors.New("unexpected PutObjectWithReader call")
}

func (*bundledAssetStorage) GetObject(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected GetObject call")
}

func (*bundledAssetStorage) DeleteObject(context.Context, string) error {
	return errors.New("unexpected DeleteObject call")
}

func (*bundledAssetStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "", errors.New("unexpected GetObjectUrl call")
}

func (s *bundledAssetStorage) HeadObject(_ context.Context, key string, _ ...storage.GetOptFn) (*storage.FileInfo, error) {
	if s.headErr != nil {
		return nil, s.headErr
	}
	if s.existing[key] {
		return &storage.FileInfo{Key: key}, nil
	}
	return nil, storage.ErrObjectNotFound
}

func (*bundledAssetStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, errors.New("unexpected ListAllObjects call")
}

func (*bundledAssetStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return nil, errors.New("unexpected ListObjectsPaginated call")
}

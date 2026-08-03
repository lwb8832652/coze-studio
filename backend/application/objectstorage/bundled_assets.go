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
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const (
	BundledStorageAssetDirEnv = "COZE_BUNDLED_STORAGE_ASSET_DIR"
	bundledAssetSyncLimit     = 8
)

type BundledAssetSyncResult struct {
	Checked  int
	Uploaded int
}

type bundledAsset struct {
	key  string
	path string
}

// EnsureBundledAssets fills missing built-in assets without replacing objects
// that were already provisioned by an operator.
func EnsureBundledAssets(ctx context.Context, client storage.Storage, root string) (BundledAssetSyncResult, error) {
	if ctx == nil {
		return BundledAssetSyncResult{}, fmt.Errorf("bundled asset context is unavailable")
	}
	if client == nil {
		return BundledAssetSyncResult{}, fmt.Errorf("bundled asset storage is unavailable")
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return BundledAssetSyncResult{}, fmt.Errorf("bundled asset root is required")
	}

	assets, err := loadBundledAssetManifest(root)
	if err != nil {
		return BundledAssetSyncResult{}, err
	}

	var checked atomic.Int64
	var uploaded atomic.Int64
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(bundledAssetSyncLimit)
	for _, asset := range assets {
		asset := asset
		group.Go(func() error {
			checked.Add(1)
			if _, headErr := client.HeadObject(groupCtx, asset.key); headErr == nil {
				return nil
			} else if !errors.Is(headErr, storage.ErrObjectNotFound) {
				return fmt.Errorf("inspect bundled asset %q: %w", asset.key, headErr)
			}

			content, readErr := os.ReadFile(asset.path)
			if readErr != nil {
				return fmt.Errorf("read bundled asset %q: %w", asset.key, readErr)
			}
			contentType := mime.TypeByExtension(filepath.Ext(asset.path))
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			if putErr := client.PutObject(
				groupCtx,
				asset.key,
				content,
				storage.WithContentType(contentType),
			); putErr != nil {
				return fmt.Errorf("upload bundled asset %q: %w", asset.key, putErr)
			}
			uploaded.Add(1)
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return BundledAssetSyncResult{
			Checked:  int(checked.Load()),
			Uploaded: int(uploaded.Load()),
		}, err
	}
	return BundledAssetSyncResult{
		Checked:  int(checked.Load()),
		Uploaded: int(uploaded.Load()),
	}, nil
}

func loadBundledAssetManifest(root string) ([]bundledAsset, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect bundled asset root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bundled asset root %q is not a directory", root)
	}

	assets := make([]bundledAsset, 0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundled asset %q must not be a symbolic link", path)
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("bundled asset %q is not a regular file", path)
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		key := filepath.ToSlash(relative)
		if key == "." || strings.HasPrefix(key, "../") {
			return fmt.Errorf("bundled asset %q resolves outside its root", path)
		}
		assets = append(assets, bundledAsset{key: key, path: path})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load bundled asset manifest: %w", err)
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("bundled asset root %q is empty", root)
	}
	return assets, nil
}

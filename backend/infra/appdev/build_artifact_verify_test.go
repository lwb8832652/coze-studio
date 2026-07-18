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

package appdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type buildArtifactVerifyBackend struct {
	content []byte
	headErr error
	closed  bool
}

func (*buildArtifactVerifyBackend) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}
func (backend *buildArtifactVerifyBackend) HeadObject(_ context.Context, key string, _ ...storage.GetOptFn) (*storage.FileInfo, error) {
	if backend.headErr != nil {
		return nil, backend.headErr
	}
	return &storage.FileInfo{Key: key, Size: int64(len(backend.content))}, nil
}
func (backend *buildArtifactVerifyBackend) OpenObjectStream(context.Context, string) (io.ReadCloser, error) {
	return &buildArtifactVerifyReader{Reader: bytes.NewReader(backend.content), closed: &backend.closed}, nil
}

type buildArtifactVerifyReader struct {
	*bytes.Reader
	closed *bool
}

func (reader *buildArtifactVerifyReader) Close() error { *reader.closed = true; return nil }

func TestArtifactStoreVerifiesActualBuildArtifactStreamAndMissingObject(t *testing.T) {
	content := []byte("verified artifact stream")
	sum := sha256.Sum256(content)
	digest, err := domainappdev.ParseArtifactGrantDigest("sha256:" + fmt.Sprintf("%x", sum[:]))
	require.NoError(t, err)
	backend := &buildArtifactVerifyBackend{content: content}
	store, err := NewArtifactStore(backend)
	require.NoError(t, err)

	verified, err := store.VerifyBuildArtifact(context.Background(), "appdev/build/object.zip", digest, int64(len(content)))
	require.NoError(t, err)
	require.True(t, verified)
	require.True(t, backend.closed)

	backend.headErr = storage.ErrObjectNotFound
	verified, err = store.VerifyBuildArtifact(context.Background(), "appdev/build/object.zip", digest, int64(len(content)))
	require.NoError(t, err)
	require.False(t, verified)
}

func TestArtifactStoreRejectsBuildArtifactDigestAndSizeMismatch(t *testing.T) {
	content := []byte("actual")
	sum := sha256.Sum256([]byte("expected"))
	digest, err := domainappdev.ParseArtifactGrantDigest("sha256:" + fmt.Sprintf("%x", sum[:]))
	require.NoError(t, err)
	backend := &buildArtifactVerifyBackend{content: content}
	store, err := NewArtifactStore(backend)
	require.NoError(t, err)

	verified, err := store.VerifyBuildArtifact(context.Background(), "appdev/build/object.zip", digest, int64(len(content)))
	require.False(t, verified)
	require.ErrorIs(t, err, domainappdev.ErrArtifactGrantStorage)
	require.True(t, backend.closed)

	backend.closed = false
	verified, err = store.VerifyBuildArtifact(context.Background(), "appdev/build/object.zip", digest, int64(len(content)+1))
	require.False(t, verified)
	require.ErrorIs(t, err, domainappdev.ErrArtifactGrantStorage)
	require.False(t, backend.closed)

	backend.headErr = errors.New("backend secret uri=s3://private")
	_, err = store.VerifyBuildArtifact(context.Background(), "appdev/build/object.zip", digest, int64(len(content)))
	require.ErrorIs(t, err, domainappdev.ErrArtifactGrantStorage)
	require.NotContains(t, err.Error(), "s3://")
}

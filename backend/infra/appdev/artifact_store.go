// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"strings"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type artifactStorageBackend interface {
	PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error
	HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error)
}

type artifactStreamingStorageBackend interface {
	artifactStorageBackend
	storage.StreamingStorage
}

type ArtifactStore struct {
	backend artifactStreamingStorageBackend
}

func NewArtifactStore(backend artifactStorageBackend) (*ArtifactStore, error) {
	if backend == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	streaming, ok := backend.(artifactStreamingStorageBackend)
	if !ok {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	return &ArtifactStore{backend: streaming}, nil
}

func (store *ArtifactStore) PutArtifact(ctx context.Context, objectKey string, reader io.Reader, size int64) error {
	if err := validateArtifactStoreInput(ctx, objectKey, size); err != nil || reader == nil {
		if err != nil {
			return err
		}
		return domainappdev.ErrArtifactGrantInvalid
	}
	limited := &artifactStoreCountingReader{reader: io.LimitReader(reader, size)}
	if err := store.backend.PutObjectWithReader(ctx, objectKey, limited, storage.WithObjectSize(size)); err != nil {
		return normalizeArtifactStoreError(ctx, err)
	}
	if limited.read != size {
		return domainappdev.ErrArtifactGrantStorage
	}
	return nil
}

func (store *ArtifactStore) OpenArtifact(ctx context.Context, objectKey string, maxSize int64) (*applicationappdev.ArtifactObjectRead, error) {
	if err := validateArtifactStoreInput(ctx, objectKey, maxSize); err != nil {
		return nil, err
	}
	info, err := store.backend.HeadObject(ctx, objectKey)
	if err != nil {
		return nil, normalizeArtifactStoreError(ctx, err)
	}
	if info == nil || info.Key != objectKey || info.Size < 0 || info.Size > maxSize {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	var trustedDigest *domainappdev.ArtifactGrantDigest
	if strings.HasPrefix(info.ETag, "sha256:") {
		digest, parseErr := domainappdev.ParseArtifactGrantDigest(info.ETag)
		if parseErr != nil {
			return nil, domainappdev.ErrArtifactGrantStorage
		}
		trustedDigest = &digest
	}
	content, err := store.backend.OpenObjectStream(ctx, objectKey)
	if err != nil {
		return nil, normalizeArtifactStoreError(ctx, err)
	}
	if content == nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	if err := ctx.Err(); err != nil {
		_ = content.Close()
		return nil, err
	}
	return &applicationappdev.ArtifactObjectRead{
		Body: content, Size: info.Size, Digest: trustedDigest,
	}, nil
}

// VerifyBuildArtifact verifies the actual bounded object stream. Head metadata
// is only an early bound and is never treated as proof of content integrity.
func (store *ArtifactStore) VerifyBuildArtifact(
	ctx context.Context,
	objectKey string,
	digest domainappdev.ArtifactGrantDigest,
	size int64,
) (bool, error) {
	if err := validateArtifactStoreInput(ctx, objectKey, size); err != nil || digest.IsZero() || size <= 0 {
		if err != nil {
			return false, err
		}
		return false, domainappdev.ErrArtifactGrantInvalid
	}
	info, err := store.backend.HeadObject(ctx, objectKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return false, nil
	}
	if err != nil {
		return false, normalizeArtifactStoreError(ctx, err)
	}
	if info == nil || info.Key != objectKey || info.Size != size {
		return false, domainappdev.ErrArtifactGrantStorage
	}
	content, err := store.backend.OpenObjectStream(ctx, objectKey)
	if errors.Is(err, storage.ErrObjectNotFound) {
		return false, nil
	}
	if err != nil || content == nil {
		return false, normalizeArtifactStoreError(ctx, err)
	}
	hasher := sha256.New()
	read, readErr := io.Copy(hasher, io.LimitReader(content, size+1))
	closeErr := content.Close()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if readErr != nil || closeErr != nil || read != size || subtle.ConstantTimeCompare(hasher.Sum(nil), digest[:]) != 1 {
		return false, domainappdev.ErrArtifactGrantStorage
	}
	return true, nil
}

type artifactStoreCountingReader struct {
	reader io.Reader
	read   int64
}

func (reader *artifactStoreCountingReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.read += int64(count)
	return count, err
}

func validateArtifactStoreInput(ctx context.Context, objectKey string, size int64) error {
	if ctx == nil || !domainappdev.ValidArtifactGrantObjectKey(objectKey) || size < 0 || size > domainappdev.MaxArtifactGrantObjectBytes {
		return domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func normalizeArtifactStoreError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return domainappdev.ErrArtifactGrantStorage
}

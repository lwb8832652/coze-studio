// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type artifactStorageBackendStub struct {
	putCalls  int
	putKey    string
	putData   []byte
	putSize   int64
	putErr    error
	headCalls int
	headInfo  *storage.FileInfo
	headErr   error
	openCalls int
	openBody  io.ReadCloser
	openErr   error
}

func (s *artifactStorageBackendStub) PutObjectWithReader(_ context.Context, key string, content io.Reader, options ...storage.PutOptFn) error {
	s.putCalls++
	s.putKey = key
	s.putData, _ = io.ReadAll(content)
	option := &storage.PutOption{}
	for _, apply := range options {
		apply(option)
	}
	s.putSize = option.ObjectSize
	return s.putErr
}

func (s *artifactStorageBackendStub) HeadObject(_ context.Context, _ string, _ ...storage.GetOptFn) (*storage.FileInfo, error) {
	s.headCalls++
	return s.headInfo, s.headErr
}

func (s *artifactStorageBackendStub) OpenObjectStream(_ context.Context, _ string) (io.ReadCloser, error) {
	s.openCalls++
	return s.openBody, s.openErr
}

var _ storage.StreamingStorage = (*artifactStorageBackendStub)(nil)

type artifactStorageLegacyBackendStub struct{}

func (*artifactStorageLegacyBackendStub) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}

func (*artifactStorageLegacyBackendStub) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, nil
}

type artifactStoreTrackingReader struct {
	reader io.Reader
	reads  int
	closed bool
}

func (reader *artifactStoreTrackingReader) Read(buffer []byte) (int, error) {
	reader.reads++
	return reader.reader.Read(buffer)
}

func (reader *artifactStoreTrackingReader) Close() error {
	reader.closed = true
	return nil
}

func TestArtifactStorePublishesValidatedReaderWithBoundedSize(t *testing.T) {
	backend := &artifactStorageBackendStub{}
	store, err := NewArtifactStore(backend)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("payload")
	if err := store.PutArtifact(context.Background(), "appdev/1001/project-a/object", bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	if backend.putCalls != 1 || backend.putSize != int64(len(payload)) || !bytes.Equal(backend.putData, payload) {
		t.Fatalf("put calls=%d size=%d data=%q", backend.putCalls, backend.putSize, backend.putData)
	}
	var _ applicationappdev.ArtifactObjectStore = store
}

func TestArtifactStoreRejectsTraversalAndOversizedMetadataBeforeGet(t *testing.T) {
	backend := &artifactStorageBackendStub{headInfo: &storage.FileInfo{Key: "appdev/1001/project-a/object", Size: 2048}}
	store, _ := NewArtifactStore(backend)
	if err := store.PutArtifact(context.Background(), "appdev/../secret", strings.NewReader("payload"), 7); !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) || backend.putCalls != 0 {
		t.Fatalf("traversal put=%v calls=%d", err, backend.putCalls)
	}
	object, err := store.OpenArtifact(context.Background(), "appdev/1001/project-a/object", 1024)
	if object != nil || !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) || backend.openCalls != 0 {
		t.Fatalf("oversized open=%#v,%v stream calls=%d", object, err, backend.openCalls)
	}
}

func TestArtifactStoreRequiresStreamingBackendAndReturnsRawReader(t *testing.T) {
	if store, err := NewArtifactStore(&artifactStorageLegacyBackendStub{}); store != nil || !errors.Is(err, domainappdev.ErrArtifactGrantUnavailable) {
		t.Fatalf("legacy backend = %#v, %v", store, err)
	}
	stream := &artifactStoreTrackingReader{reader: strings.NewReader("payload")}
	backend := &artifactStorageBackendStub{
		headInfo: &storage.FileInfo{Key: "appdev/1001/project-a/object", Size: 7}, openBody: stream,
	}
	store, _ := NewArtifactStore(backend)
	object, err := store.OpenArtifact(context.Background(), "appdev/1001/project-a/object", 1024)
	if err != nil || object == nil || object.Body != stream || object.Size != 7 || backend.openCalls != 1 || stream.reads != 0 {
		t.Fatalf("stream object=%#v err=%v opens=%d reads=%d", object, err, backend.openCalls, stream.reads)
	}
	if err := object.Body.Close(); err != nil || !stream.closed {
		t.Fatalf("stream close=%v closed=%v", err, stream.closed)
	}
}

func TestArtifactStoreDoesNotLeakBackendErrors(t *testing.T) {
	backend := &artifactStorageBackendStub{headInfo: &storage.FileInfo{Key: "appdev/1001/project-a/object", Size: 7}}
	store, _ := NewArtifactStore(backend)
	backend.headErr = errors.New("s3://secret-bucket/internal/object")
	if _, err := store.OpenArtifact(context.Background(), "appdev/1001/project-a/object", 1024); !errors.Is(err, domainappdev.ErrArtifactGrantStorage) || strings.Contains(err.Error(), "secret-bucket") {
		t.Fatalf("backend error leaked: %v", err)
	}
	backend.headErr = nil
	backend.openErr = errors.New("https://storage.invalid/private/object")
	if _, err := store.OpenArtifact(context.Background(), "appdev/1001/project-a/object", 1024); !errors.Is(err, domainappdev.ErrArtifactGrantStorage) || strings.Contains(err.Error(), "storage.invalid") {
		t.Fatalf("stream error leaked: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.OpenArtifact(ctx, "appdev/1001/project-a/object", 1024); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled open=%v", err)
	}
}

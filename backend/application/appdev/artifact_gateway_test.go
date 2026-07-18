// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type artifactGatewayConsumerStub struct {
	record    *domainappdev.ArtifactGrantRecord
	calls     int
	consumed  bool
	err       error
	onConsume func()
	request   ConsumeArtifactGrantRequest
}

func (s *artifactGatewayConsumerStub) Consume(_ context.Context, request ConsumeArtifactGrantRequest) (*domainappdev.ArtifactGrantRecord, error) {
	s.calls++
	s.request = request
	if s.onConsume != nil {
		s.onConsume()
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.consumed {
		return nil, domainappdev.ErrArtifactGrantConsumed
	}
	s.consumed = true
	result := *s.record
	return &result, nil
}

type artifactObjectStoreStub struct {
	putCalls  int
	putKey    string
	putData   []byte
	putMode   os.FileMode
	putErr    error
	openCalls int
	openKey   string
	object    *ArtifactObjectRead
	openErr   error
	onPut     func()
}

func (s *artifactObjectStoreStub) PutArtifact(_ context.Context, key string, reader io.Reader, _ int64) error {
	s.putCalls++
	s.putKey = key
	if s.onPut != nil {
		s.onPut()
	}
	if file, ok := reader.(*os.File); ok {
		if info, err := file.Stat(); err == nil {
			s.putMode = info.Mode().Perm()
		}
	}
	s.putData, _ = io.ReadAll(reader)
	return s.putErr
}

func (s *artifactObjectStoreStub) OpenArtifact(_ context.Context, key string, _ int64) (*ArtifactObjectRead, error) {
	s.openCalls++
	s.openKey = key
	return s.object, s.openErr
}

type trackingArtifactBody struct {
	reader *bytes.Reader
	closed bool
	reads  int
}

type maliciousArtifactStream struct {
	readBytes int64
	closed    bool
}

type artifactGatewayUnlinkFailureOps struct {
	file         *os.File
	path         string
	sizeAtUnlink int64
	unlinkCalls  int
	err          error
}

func (ops *artifactGatewayUnlinkFailureOps) CreateTemp(directory, pattern string) (*os.File, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err == nil {
		ops.file = file
		ops.path = file.Name()
	}
	return file, err
}

func (ops *artifactGatewayUnlinkFailureOps) Unlink(path string) error {
	ops.unlinkCalls++
	if info, err := os.Stat(path); err == nil {
		ops.sizeAtUnlink = info.Size()
	}
	return ops.err
}

type artifactDownloadCloseReader struct {
	closeCalls int
	closeErr   error
}

func (*artifactDownloadCloseReader) Read([]byte) (int, error) { return 0, io.EOF }
func (reader *artifactDownloadCloseReader) Close() error {
	reader.closeCalls++
	return reader.closeErr
}

func (stream *maliciousArtifactStream) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	stream.readBytes += int64(len(buffer))
	return len(buffer), nil
}

func (stream *maliciousArtifactStream) Close() error {
	stream.closed = true
	return nil
}

func newTrackingArtifactBody(content []byte) *trackingArtifactBody {
	return &trackingArtifactBody{reader: bytes.NewReader(content)}
}

func (b *trackingArtifactBody) Read(buffer []byte) (int, error) {
	b.reads++
	return b.reader.Read(buffer)
}

func (b *trackingArtifactBody) Close() error {
	b.closed = true
	return nil
}

func artifactGatewayFixture(t *testing.T, payload []byte, direction domainappdev.ArtifactGrantDirection) (*ArtifactGrantCapability, *domainappdev.ArtifactGrantRecord) {
	t.Helper()
	grantID, err := domainappdev.NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{0x61}, domainappdev.ArtifactGrantIDBytes)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := domainappdev.NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{0x62}, domainappdev.ArtifactGrantTokenBytes)))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	audience := domainappdev.ArtifactGrantAudience{
		SpaceID: "1001", ProjectID: "project-a", ProviderKey: "provider-a",
		ProviderScope: domainsandbox.ScopeAppDev, Operation: "snapshot.transfer",
	}
	issuedAt := time.Date(2026, 7, 18, 11, 0, 0, 0, time.UTC)
	record := &domainappdev.ArtifactGrantRecord{
		GrantID: grantID,
		Spec: domainappdev.ArtifactGrantSpec{
			Audience: audience, Direction: direction, ObjectKey: "appdev/1001/project-a/snapshot.zip",
			Digest: domainappdev.ArtifactGrantDigest(digest), Size: int64(len(payload)), MaxSize: int64(len(payload) + 4),
		},
		State: domainappdev.ArtifactGrantStateConsumed, IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(time.Minute),
	}
	capability := &ArtifactGrantCapability{
		grantID: grantID, token: token, audience: audience, direction: direction,
		digest: record.Spec.Digest, size: record.Spec.Size, maxSize: record.Spec.MaxSize,
		issuedAt: issuedAt, expiresAt: record.ExpiresAt,
	}
	return capability, record
}

func assertArtifactTempDirEmpty(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging files remain: %#v", entries)
	}
}

func TestArtifactGatewayUploadStagesValidatesAndPublishesOnce(t *testing.T) {
	payload := []byte("artifact-payload")
	capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionUpload)
	consumer := &artifactGatewayConsumerStub{record: record}
	directory := t.TempDir()
	store := &artifactObjectStoreStub{onPut: func() { assertArtifactTempDirEmpty(t, directory) }}
	gateway, err := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(directory))
	if err != nil {
		t.Fatal(err)
	}
	body := newTrackingArtifactBody(payload)
	result, err := gateway.Upload(context.Background(), UploadArtifactRequest{
		Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
		Body: body, ContentLength: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Size != int64(len(payload)) || result.Digest != record.Spec.Digest || consumer.calls != 1 ||
		store.putCalls != 1 || !bytes.Equal(store.putData, payload) || store.putMode != 0o600 || !body.closed {
		t.Fatalf("upload result=%#v consume=%d put=%d mode=%o closed=%v", result, consumer.calls, store.putCalls, store.putMode, body.closed)
	}
	assertArtifactTempDirEmpty(t, directory)
}

func TestArtifactGatewayUploadRejectsLengthsOverflowAndDigestBeforeFinalObject(t *testing.T) {
	payload := []byte("artifact-payload")
	for _, test := range []struct {
		name          string
		body          []byte
		contentLength int64
		mutateRecord  func(*domainappdev.ArtifactGrantRecord)
	}{
		{name: "negative length", body: payload, contentLength: -2},
		{name: "declared over max", body: payload, contentLength: int64(len(payload) + 5)},
		{name: "declared not exact", body: payload, contentLength: int64(len(payload) - 1)},
		{name: "chunked over max", body: append(append([]byte(nil), payload...), []byte("extra")...), contentLength: -1},
		{name: "digest mismatch", body: []byte("tampered-content"), contentLength: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionUpload)
			if test.mutateRecord != nil {
				test.mutateRecord(record)
			}
			consumer := &artifactGatewayConsumerStub{record: record}
			store := &artifactObjectStoreStub{}
			directory := t.TempDir()
			gateway, err := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(directory))
			if err != nil {
				t.Fatal(err)
			}
			body := newTrackingArtifactBody(test.body)
			result, err := gateway.Upload(context.Background(), UploadArtifactRequest{
				Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
				Body: body, ContentLength: test.contentLength,
			})
			if result != nil || !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) || consumer.calls != 1 || store.putCalls != 0 || !body.closed {
				t.Fatalf("Upload()=%#v,%v consume=%d put=%d closed=%v", result, err, consumer.calls, store.putCalls, body.closed)
			}
			assertArtifactTempDirEmpty(t, directory)
		})
	}
}

func TestArtifactGatewayUploadFailureConsumesOnceAndCannotRestoreToken(t *testing.T) {
	payload := []byte("artifact-payload")
	capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionUpload)
	consumer := &artifactGatewayConsumerStub{record: record}
	store := &artifactObjectStoreStub{putErr: errors.New("s3://secret/internal-object")}
	gateway, err := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	request := UploadArtifactRequest{
		Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
		Body: newTrackingArtifactBody(payload), ContentLength: int64(len(payload)),
	}
	if result, err := gateway.Upload(context.Background(), request); result != nil || !errors.Is(err, domainappdev.ErrArtifactGrantStorage) || strings.Contains(err.Error(), "secret/internal-object") {
		t.Fatalf("storage failure=%#v,%v", result, err)
	}
	request.Body = newTrackingArtifactBody(payload)
	if result, err := gateway.Upload(context.Background(), request); result != nil || !errors.Is(err, domainappdev.ErrArtifactGrantConsumed) {
		t.Fatalf("retry after storage failure=%#v,%v", result, err)
	}
	if consumer.calls != 2 || store.putCalls != 1 {
		t.Fatalf("consume calls=%d put calls=%d", consumer.calls, store.putCalls)
	}
}

func TestArtifactGatewayUploadCancellationAfterConsumeLeavesNoObject(t *testing.T) {
	payload := []byte("artifact-payload")
	capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionUpload)
	ctx, cancel := context.WithCancel(context.Background())
	consumer := &artifactGatewayConsumerStub{record: record, onConsume: cancel}
	store := &artifactObjectStoreStub{}
	directory := t.TempDir()
	gateway, _ := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(directory))
	body := newTrackingArtifactBody(payload)
	result, err := gateway.Upload(ctx, UploadArtifactRequest{
		Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
		Body: body, ContentLength: int64(len(payload)),
	})
	if result != nil || !errors.Is(err, context.Canceled) || consumer.calls != 1 || store.putCalls != 0 || !body.closed {
		t.Fatalf("canceled upload=%#v,%v consume=%d put=%d closed=%v", result, err, consumer.calls, store.putCalls, body.closed)
	}
	assertArtifactTempDirEmpty(t, directory)
}

func TestArtifactGatewayDownloadValidatesBeforeReturningAnyReader(t *testing.T) {
	payload := []byte("artifact-payload")
	capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionDownload)
	body := newTrackingArtifactBody(payload)
	metadataDigest := record.Spec.Digest
	consumer := &artifactGatewayConsumerStub{record: record}
	store := &artifactObjectStoreStub{object: &ArtifactObjectRead{Body: body, Size: int64(len(payload)), Digest: &metadataDigest}}
	directory := t.TempDir()
	gateway, err := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(directory))
	if err != nil {
		t.Fatal(err)
	}
	download, err := gateway.Download(context.Background(), DownloadArtifactRequest{
		Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactTempDirEmpty(t, directory)
	content, err := io.ReadAll(download)
	if err != nil || !bytes.Equal(content, payload) || !body.closed {
		t.Fatalf("download content=%q err=%v source closed=%v", content, err, body.closed)
	}
	for _, formatted := range []string{fmt.Sprintf("%v", download), fmt.Sprintf("%+v", download), fmt.Sprintf("%#v", download)} {
		if strings.Contains(formatted, record.Spec.ObjectKey) || !strings.Contains(formatted, "REDACTED") {
			t.Fatalf("download leaked internals: %q", formatted)
		}
	}
	if encoded, err := json.Marshal(download); err == nil || len(encoded) != 0 {
		t.Fatalf("download JSON=%q,%v", encoded, err)
	}
	if err := download.Close(); err != nil {
		t.Fatal(err)
	}
	assertArtifactTempDirEmpty(t, directory)
}

func TestArtifactGatewayUnlinkFailureWritesNoPlaintextAndClosesEverything(t *testing.T) {
	payload := []byte("artifact-payload")
	for _, direction := range []domainappdev.ArtifactGrantDirection{
		domainappdev.ArtifactGrantDirectionUpload,
		domainappdev.ArtifactGrantDirectionDownload,
	} {
		t.Run(string(direction), func(t *testing.T) {
			capability, record := artifactGatewayFixture(t, payload, direction)
			consumer := &artifactGatewayConsumerStub{record: record}
			directory := t.TempDir()
			unlinkErr := errors.New("unlink unavailable")
			ops := &artifactGatewayUnlinkFailureOps{err: unlinkErr}
			store := &artifactObjectStoreStub{}
			source := newTrackingArtifactBody(payload)
			if direction == domainappdev.ArtifactGrantDirectionDownload {
				store.object = &ArtifactObjectRead{Body: source, Size: record.Spec.Size}
			}
			gateway, err := NewArtifactGateway(consumer, store,
				WithArtifactGatewayTempDir(directory), WithArtifactGatewayTempFileOps(ops))
			if err != nil {
				t.Fatal(err)
			}
			if direction == domainappdev.ArtifactGrantDirectionUpload {
				result, runErr := gateway.Upload(context.Background(), UploadArtifactRequest{
					Capability: capability, Audience: record.Spec.Audience, Direction: direction,
					Body: source, ContentLength: int64(len(payload)),
				})
				if result != nil || !errors.Is(runErr, domainappdev.ErrArtifactGrantStorage) || store.putCalls != 0 {
					t.Fatalf("upload unlink failure = %#v, %v, puts=%d", result, runErr, store.putCalls)
				}
			} else {
				download, runErr := gateway.Download(context.Background(), DownloadArtifactRequest{
					Capability: capability, Audience: record.Spec.Audience, Direction: direction,
				})
				if download != nil || !errors.Is(runErr, domainappdev.ErrArtifactGrantStorage) {
					t.Fatalf("download unlink failure = %#v, %v", download, runErr)
				}
			}
			if ops.unlinkCalls != 1 || ops.sizeAtUnlink != 0 || !source.closed || source.reads != 0 {
				t.Fatalf("unlink calls=%d size=%d source closed=%v reads=%d", ops.unlinkCalls, ops.sizeAtUnlink, source.closed, source.reads)
			}
			if _, err := ops.file.Write([]byte("must-fail")); err == nil {
				t.Fatal("staging descriptor remained open after unlink failure")
			}
			assertArtifactTempDirEmpty(t, directory)
		})
	}
}

func TestArtifactDownloadCloseIsIdempotentAndReturnsRealCloseError(t *testing.T) {
	closeErr := errors.New("real descriptor close error")
	reader := &artifactDownloadCloseReader{closeErr: closeErr}
	download := &ArtifactDownload{file: reader}
	if err := download.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("first Close() = %v", err)
	}
	if err := download.Close(); err != nil {
		t.Fatalf("second Close() = %v", err)
	}
	if reader.closeCalls != 1 {
		t.Fatalf("close calls = %d", reader.closeCalls)
	}
}

func TestArtifactGatewayDownloadMetadataOrDigestFailureReturnsNoReaderAndCleansUp(t *testing.T) {
	payload := []byte("artifact-payload")
	for _, test := range []struct {
		name   string
		object func(*domainappdev.ArtifactGrantRecord) *ArtifactObjectRead
		reads  bool
	}{
		{name: "metadata size mismatch", object: func(record *domainappdev.ArtifactGrantRecord) *ArtifactObjectRead {
			return &ArtifactObjectRead{Body: newTrackingArtifactBody(payload), Size: record.Spec.Size + 1}
		}},
		{name: "metadata digest mismatch", object: func(record *domainappdev.ArtifactGrantRecord) *ArtifactObjectRead {
			wrong := sha256.Sum256([]byte("wrong"))
			return &ArtifactObjectRead{Body: newTrackingArtifactBody(payload), Size: record.Spec.Size, Digest: artifactDigestPointer(domainappdev.ArtifactGrantDigest(wrong))}
		}},
		{name: "stream digest mismatch", reads: true, object: func(record *domainappdev.ArtifactGrantRecord) *ArtifactObjectRead {
			return &ArtifactObjectRead{Body: newTrackingArtifactBody([]byte("tampered-content")), Size: record.Spec.Size}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionDownload)
			object := test.object(record)
			tracked := object.Body.(*trackingArtifactBody)
			consumer := &artifactGatewayConsumerStub{record: record}
			store := &artifactObjectStoreStub{object: object}
			directory := t.TempDir()
			gateway, _ := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(directory))
			download, err := gateway.Download(context.Background(), DownloadArtifactRequest{
				Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
			})
			if download != nil || !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) || !tracked.closed {
				t.Fatalf("Download()=%#v,%v source closed=%v", download, err, tracked.closed)
			}
			if !test.reads && tracked.reads != 0 {
				t.Fatalf("metadata failure read %d source chunks", tracked.reads)
			}
			assertArtifactTempDirEmpty(t, directory)
		})
	}
}

func TestArtifactGatewayDownloadStopsMaliciousStreamAtMaxPlusOneBeforeReturningReader(t *testing.T) {
	payload := []byte("artifact-payload")
	capability, record := artifactGatewayFixture(t, payload, domainappdev.ArtifactGrantDirectionDownload)
	stream := &maliciousArtifactStream{}
	consumer := &artifactGatewayConsumerStub{record: record}
	store := &artifactObjectStoreStub{object: &ArtifactObjectRead{
		Body: stream, Size: record.Spec.Size,
	}}
	directory := t.TempDir()
	gateway, err := NewArtifactGateway(consumer, store, WithArtifactGatewayTempDir(directory))
	if err != nil {
		t.Fatal(err)
	}
	download, err := gateway.Download(context.Background(), DownloadArtifactRequest{
		Capability: capability, Audience: record.Spec.Audience, Direction: record.Spec.Direction,
	})
	if download != nil || !errors.Is(err, domainappdev.ErrArtifactGrantInvalid) {
		t.Fatalf("malicious download = %#v, %v", download, err)
	}
	if stream.readBytes != record.Spec.MaxSize+1 || !stream.closed {
		t.Fatalf("malicious stream read=%d want=%d closed=%v", stream.readBytes, record.Spec.MaxSize+1, stream.closed)
	}
	assertArtifactTempDirEmpty(t, directory)
}

func artifactDigestPointer(value domainappdev.ArtifactGrantDigest) *domainappdev.ArtifactGrantDigest {
	return &value
}

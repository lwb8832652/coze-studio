// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type ArtifactGrantConsumer interface {
	Consume(context.Context, ConsumeArtifactGrantRequest) (*domainappdev.ArtifactGrantRecord, error)
}

type ArtifactObjectRead struct {
	Body   io.ReadCloser
	Size   int64
	Digest *domainappdev.ArtifactGrantDigest
}

type ArtifactObjectStore interface {
	PutArtifact(context.Context, string, io.Reader, int64) error
	OpenArtifact(context.Context, string, int64) (*ArtifactObjectRead, error)
}

type ArtifactGatewayOption func(*ArtifactGateway) error

type ArtifactGatewayTempFileOps interface {
	CreateTemp(directory, pattern string) (*os.File, error)
	Unlink(path string) error
}

type osArtifactGatewayTempFileOps struct{}

func (osArtifactGatewayTempFileOps) CreateTemp(directory, pattern string) (*os.File, error) {
	return os.CreateTemp(directory, pattern)
}

func (osArtifactGatewayTempFileOps) Unlink(path string) error { return os.Remove(path) }

func WithArtifactGatewayTempDir(directory string) ArtifactGatewayOption {
	return func(gateway *ArtifactGateway) error {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			return domainappdev.ErrArtifactGrantInvalid
		}
		gateway.tempDir = directory
		return nil
	}
}

func WithArtifactGatewayTempFileOps(fileOps ArtifactGatewayTempFileOps) ArtifactGatewayOption {
	return func(gateway *ArtifactGateway) error {
		if fileOps == nil {
			return domainappdev.ErrArtifactGrantInvalid
		}
		gateway.tempFileOps = fileOps
		return nil
	}
}

type ArtifactGateway struct {
	consumer    ArtifactGrantConsumer
	store       ArtifactObjectStore
	tempDir     string
	tempFileOps ArtifactGatewayTempFileOps
}

// NewArtifactGrantGatewayCapability reconstructs the opaque capability at the
// provider-only HTTP boundary after provider authentication has supplied the
// authoritative audience. It deliberately exposes neither capability fields
// nor a plaintext serialization API.
func NewArtifactGrantGatewayCapability(
	grantID domainappdev.ArtifactGrantID,
	token domainappdev.ArtifactGrantToken,
	audience domainappdev.ArtifactGrantAudience,
	direction domainappdev.ArtifactGrantDirection,
) (*ArtifactGrantCapability, error) {
	if grantID.IsZero() || token.IsZero() ||
		domainappdev.ValidateArtifactGrantAudience(audience) != nil ||
		domainappdev.ValidateArtifactGrantDirection(direction) != nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	return &ArtifactGrantCapability{
		grantID: grantID, token: token, audience: audience, direction: direction,
	}, nil
}

func NewArtifactGateway(consumer ArtifactGrantConsumer, store ArtifactObjectStore, options ...ArtifactGatewayOption) (*ArtifactGateway, error) {
	if consumer == nil || store == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	gateway := &ArtifactGateway{
		consumer: consumer, store: store, tempDir: os.TempDir(),
		tempFileOps: osArtifactGatewayTempFileOps{},
	}
	for _, option := range options {
		if option == nil || option(gateway) != nil {
			return nil, domainappdev.ErrArtifactGrantInvalid
		}
	}
	return gateway, nil
}

type UploadArtifactRequest struct {
	Capability    *ArtifactGrantCapability
	Audience      domainappdev.ArtifactGrantAudience
	Direction     domainappdev.ArtifactGrantDirection
	Body          io.ReadCloser
	ContentLength int64
}

type ArtifactUploadResult struct {
	Size   int64
	Digest domainappdev.ArtifactGrantDigest
}

func (gateway *ArtifactGateway) Upload(ctx context.Context, request UploadArtifactRequest) (*ArtifactUploadResult, error) {
	if request.Body != nil {
		defer request.Body.Close()
	}
	if err := validateArtifactGatewayCapability(ctx, request.Capability, request.Audience, request.Direction, domainappdev.ArtifactGrantDirectionUpload); err != nil || request.Body == nil {
		if err != nil {
			return nil, err
		}
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	record, err := gateway.consumer.Consume(ctx, ConsumeArtifactGrantRequest{
		GrantID: request.Capability.grantID, Token: request.Capability.token,
		Audience: request.Audience, Direction: request.Direction,
	})
	if err != nil {
		return nil, normalizeArtifactGatewayGrantError(ctx, err)
	}
	if !validArtifactGrantLifecycleRecord(record, request.Capability.grantID, request.Audience, request.Direction, domainappdev.ArtifactGrantStateConsumed) {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	if request.ContentLength < -1 || request.ContentLength > record.Spec.MaxSize ||
		(request.ContentLength >= 0 && request.ContentLength != record.Spec.Size) {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	staged, err := createArtifactGatewayTemp(gateway.tempFileOps, gateway.tempDir)
	if err != nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	defer closeArtifactGatewayTemp(staged)
	size, digest, err := stageArtifactGatewayStream(ctx, staged, request.Body, record.Spec.MaxSize)
	if err != nil {
		return nil, err
	}
	if size != record.Spec.Size || digest != record.Spec.Digest {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	if err := gateway.store.PutArtifact(ctx, record.Spec.ObjectKey, staged, size); err != nil {
		return nil, normalizeArtifactGatewayStorageError(ctx, err)
	}
	return &ArtifactUploadResult{Size: size, Digest: digest}, nil
}

type DownloadArtifactRequest struct {
	Capability *ArtifactGrantCapability
	Audience   domainappdev.ArtifactGrantAudience
	Direction  domainappdev.ArtifactGrantDirection
}

// ArtifactGatewayDownloadResponse transfers ownership of an already fully
// validated download stream to the internal HTTP gateway. It is intentionally
// not serializable or generally formattable.
type ArtifactGatewayDownloadResponse struct {
	Size int64
	Body io.ReadCloser
}

func (*ArtifactGatewayDownloadResponse) String() string {
	return "[REDACTED validated artifact gateway response]"
}
func (*ArtifactGatewayDownloadResponse) GoString() string {
	return "[REDACTED validated artifact gateway response]"
}
func (*ArtifactGatewayDownloadResponse) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED validated artifact gateway response]")
}
func (*ArtifactGatewayDownloadResponse) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrArtifactGrantSecret
}

type ArtifactDownload struct {
	Size   int64
	Digest domainappdev.ArtifactGrantDigest

	mu     sync.Mutex
	file   io.ReadCloser
	closed bool
}

func (download *ArtifactDownload) Read(buffer []byte) (int, error) {
	download.mu.Lock()
	defer download.mu.Unlock()
	if download.closed || download.file == nil {
		return 0, io.ErrClosedPipe
	}
	return download.file.Read(buffer)
}

func (download *ArtifactDownload) Close() error {
	if download == nil {
		return nil
	}
	download.mu.Lock()
	defer download.mu.Unlock()
	if download.closed {
		return nil
	}
	download.closed = true
	closeErr := download.file.Close()
	download.file = nil
	return closeErr
}

func (*ArtifactDownload) String() string   { return "[REDACTED validated artifact download]" }
func (*ArtifactDownload) GoString() string { return "[REDACTED validated artifact download]" }
func (*ArtifactDownload) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED validated artifact download]")
}
func (*ArtifactDownload) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrArtifactGrantSecret
}

func (gateway *ArtifactGateway) Download(ctx context.Context, request DownloadArtifactRequest) (*ArtifactDownload, error) {
	if err := validateArtifactGatewayCapability(ctx, request.Capability, request.Audience, request.Direction, domainappdev.ArtifactGrantDirectionDownload); err != nil {
		return nil, err
	}
	record, err := gateway.consumer.Consume(ctx, ConsumeArtifactGrantRequest{
		GrantID: request.Capability.grantID, Token: request.Capability.token,
		Audience: request.Audience, Direction: request.Direction,
	})
	if err != nil {
		return nil, normalizeArtifactGatewayGrantError(ctx, err)
	}
	if !validArtifactGrantLifecycleRecord(record, request.Capability.grantID, request.Audience, request.Direction, domainappdev.ArtifactGrantStateConsumed) {
		return nil, domainappdev.ErrArtifactGrantUnavailable
	}
	object, err := gateway.store.OpenArtifact(ctx, record.Spec.ObjectKey, record.Spec.MaxSize)
	if err != nil {
		return nil, normalizeArtifactGatewayStorageError(ctx, err)
	}
	if object == nil || object.Body == nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	defer object.Body.Close()
	if object.Size < 0 || object.Size > record.Spec.MaxSize || object.Size != record.Spec.Size ||
		(object.Digest != nil && *object.Digest != record.Spec.Digest) {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	staged, err := createArtifactGatewayTemp(gateway.tempFileOps, gateway.tempDir)
	if err != nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	keepStaged := false
	defer func() {
		if !keepStaged {
			closeArtifactGatewayTemp(staged)
		}
	}()
	size, digest, err := stageArtifactGatewayStream(ctx, staged, object.Body, record.Spec.MaxSize)
	if err != nil {
		return nil, err
	}
	if size != record.Spec.Size || size != object.Size || digest != record.Spec.Digest {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	keepStaged = true
	return &ArtifactDownload{Size: size, Digest: digest, file: staged}, nil
}

// OpenDownload is the narrow provider HTTP adapter surface. Download performs
// all storage, size, and digest validation before this method returns.
func (gateway *ArtifactGateway) OpenDownload(ctx context.Context, request DownloadArtifactRequest) (*ArtifactGatewayDownloadResponse, error) {
	download, err := gateway.Download(ctx, request)
	if err != nil {
		return nil, err
	}
	return &ArtifactGatewayDownloadResponse{Size: download.Size, Body: download}, nil
}

func validateArtifactGatewayCapability(ctx context.Context, capability *ArtifactGrantCapability, audience domainappdev.ArtifactGrantAudience, direction, requiredDirection domainappdev.ArtifactGrantDirection) error {
	if ctx == nil || capability == nil || capability.grantID.IsZero() || capability.token.IsZero() ||
		capability.audience != audience || capability.direction != direction || direction != requiredDirection ||
		domainappdev.ValidateArtifactGrantAudience(audience) != nil || domainappdev.ValidateArtifactGrantDirection(direction) != nil {
		return domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func createArtifactGatewayTemp(fileOps ArtifactGatewayTempFileOps, directory string) (*os.File, error) {
	if fileOps == nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	file, err := fileOps.CreateTemp(directory, "coze-appdev-artifact-*")
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	path := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = fileOps.Unlink(path)
		_ = os.Remove(path)
		return nil, err
	}
	if err := fileOps.Unlink(path); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, domainappdev.ErrArtifactGrantStorage
	}
	return file, nil
}

func closeArtifactGatewayTemp(file io.Closer) {
	if file == nil {
		return
	}
	_ = file.Close()
}

func stageArtifactGatewayStream(ctx context.Context, destination io.Writer, source io.Reader, maxSize int64) (int64, domainappdev.ArtifactGrantDigest, error) {
	if maxSize < 0 || maxSize > domainappdev.MaxArtifactGrantObjectBytes {
		return 0, domainappdev.ArtifactGrantDigest{}, domainappdev.ErrArtifactGrantInvalid
	}
	hash := sha256.New()
	limited := &io.LimitedReader{R: &artifactGatewayContextReader{ctx: ctx, reader: source}, N: maxSize + 1}
	size, err := io.Copy(io.MultiWriter(destination, hash), limited)
	if ctx.Err() != nil {
		return 0, domainappdev.ArtifactGrantDigest{}, ctx.Err()
	}
	if err != nil {
		return 0, domainappdev.ArtifactGrantDigest{}, domainappdev.ErrArtifactGrantStorage
	}
	if size > maxSize {
		return 0, domainappdev.ArtifactGrantDigest{}, domainappdev.ErrArtifactGrantInvalid
	}
	var digest domainappdev.ArtifactGrantDigest
	copy(digest[:], hash.Sum(nil))
	return size, digest, nil
}

type artifactGatewayContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *artifactGatewayContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func normalizeArtifactGatewayStorageError(ctx context.Context, err error) error {
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

func normalizeArtifactGatewayGrantError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	for _, safe := range []error{
		domainappdev.ErrArtifactGrantAudienceMismatch,
		domainappdev.ErrArtifactGrantDirectionMismatch,
		domainappdev.ErrArtifactGrantExpired,
		domainappdev.ErrArtifactGrantRevoked,
		domainappdev.ErrArtifactGrantConsumed,
		domainappdev.ErrArtifactGrantDenied,
		domainappdev.ErrArtifactGrantInvalid,
		domainappdev.ErrArtifactGrantUnavailable,
	} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return domainappdev.ErrArtifactGrantUnavailable
}

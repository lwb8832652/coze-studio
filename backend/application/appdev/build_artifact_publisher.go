// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	BuildArtifactPublishOperation = "build.publish"
	DefaultBuildArtifactGrantTTL  = 60 * time.Second
	buildArtifactRevokeTimeout    = 2 * time.Second
)

var (
	ErrBuildArtifactPublisherUnsupported = errors.New("appdev build artifact publisher is unsupported")
	ErrBuildArtifactPublishUnavailable   = errors.New("appdev build artifact publication is unavailable")
)

type BuildArtifactGrantLifecycle interface {
	Issue(context.Context, IssueArtifactGrantRequest) (*ArtifactGrantCapability, error)
	Revoke(context.Context, RevokeArtifactGrantRequest) error
}

type BuildArtifactUploadEndpointBuilder interface {
	ArtifactUploadEndpoint(context.Context, domainappdev.ArtifactGrantID) (string, error)
}

type BuildArtifactStoreVerifier interface {
	VerifyBuildArtifact(context.Context, string, domainappdev.ArtifactGrantDigest, int64) (bool, error)
}

type BuildArtifactPublisherService struct {
	grants    BuildArtifactGrantLifecycle
	endpoints BuildArtifactUploadEndpointBuilder
	store     BuildArtifactStoreVerifier
	grantTTL  time.Duration
	urlPolicy ArtifactCapabilityURLPolicy
}

type BuildArtifactPublisherOption func(*BuildArtifactPublisherService) error

func WithBuildArtifactURLPolicy(policy ArtifactCapabilityURLPolicy) BuildArtifactPublisherOption {
	return func(service *BuildArtifactPublisherService) error {
		if policy == nil {
			return domainappdev.ErrArtifactGrantInvalid
		}
		service.urlPolicy = policy
		return nil
	}
}

func NewBuildArtifactPublisherService(
	grants BuildArtifactGrantLifecycle,
	endpoints BuildArtifactUploadEndpointBuilder,
	store BuildArtifactStoreVerifier,
	grantTTL time.Duration,
	options ...BuildArtifactPublisherOption,
) (*BuildArtifactPublisherService, error) {
	if grants == nil || endpoints == nil || store == nil || domainappdev.ValidateArtifactGrantTTL(grantTTL) != nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	service := &BuildArtifactPublisherService{
		grants: grants, endpoints: endpoints, store: store, grantTTL: grantTTL,
		urlPolicy: NewArtifactCapabilityURLPolicy(false),
	}
	for _, option := range options {
		if option == nil || option(service) != nil {
			return nil, domainappdev.ErrArtifactGrantInvalid
		}
	}
	return service, nil
}

type PublishBuildArtifactCommand struct {
	Audience            domainappdev.ArtifactGrantAudience
	Generation          uint64
	ProviderExecutionID string
	Descriptor          infrasandbox.ArtifactDescriptor
}

type BuildArtifactReceipt struct {
	objectKey string
	digest    domainappdev.ArtifactGrantDigest
	size      int64
}

func (receipt *BuildArtifactReceipt) InternalObjectKey() string {
	if receipt == nil {
		return ""
	}
	return receipt.objectKey
}
func (receipt *BuildArtifactReceipt) Digest() domainappdev.ArtifactGrantDigest {
	if receipt == nil {
		return domainappdev.ArtifactGrantDigest{}
	}
	return receipt.digest
}
func (receipt *BuildArtifactReceipt) Size() int64 {
	if receipt == nil {
		return 0
	}
	return receipt.size
}
func (*BuildArtifactReceipt) String() string   { return "BuildArtifactReceipt{object:<redacted>}" }
func (*BuildArtifactReceipt) GoString() string { return "BuildArtifactReceipt{object:<redacted>}" }
func (*BuildArtifactReceipt) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "BuildArtifactReceipt{object:<redacted>}")
}
func (*BuildArtifactReceipt) MarshalJSON() ([]byte, error) {
	return nil, domainappdev.ErrArtifactGrantSecret
}

func (service *BuildArtifactPublisherService) Publish(
	ctx context.Context,
	provider infrasandbox.RuntimeProvider,
	command PublishBuildArtifactCommand,
) (*BuildArtifactReceipt, error) {
	if ctx == nil || provider == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	publisher, ok := provider.(infrasandbox.ArtifactPublisher)
	if !ok {
		return nil, ErrBuildArtifactPublisherUnsupported
	}
	return service.PublishWithPublisher(ctx, publisher, command)
}

// PublishWithPublisher accepts only the outbound artifact capability. This lets
// a resumed router selection forward publication without exposing its runtime.
func (service *BuildArtifactPublisherService) PublishWithPublisher(
	ctx context.Context,
	publisher infrasandbox.ArtifactPublisher,
	command PublishBuildArtifactCommand,
) (*BuildArtifactReceipt, error) {
	if ctx == nil || publisher == nil {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	descriptor, digest, err := normalizeBuildArtifactCommand(command)
	if err != nil {
		return nil, err
	}
	objectKey := deriveBuildArtifactObjectKey(command, descriptor)
	verified, err := service.store.VerifyBuildArtifact(ctx, objectKey, digest, descriptor.Size)
	if err != nil {
		return nil, normalizeBuildArtifactPublishError(ctx, err)
	}
	if verified {
		return newBuildArtifactReceipt(objectKey, digest, descriptor.Size), nil
	}

	capability, err := service.grants.Issue(ctx, IssueArtifactGrantRequest{
		Spec: domainappdev.ArtifactGrantSpec{
			Audience: command.Audience, Direction: domainappdev.ArtifactGrantDirectionUpload,
			ObjectKey: objectKey, Digest: digest, Size: descriptor.Size, MaxSize: descriptor.Size,
		},
		TTL: service.grantTTL,
	})
	if err != nil || capability == nil {
		return nil, normalizeBuildArtifactPublishError(ctx, err)
	}
	uploadURL, err := service.endpoints.ArtifactUploadEndpoint(ctx, capability.GrantID())
	if err != nil || service.urlPolicy.Validate(uploadURL) != nil || capability.BearerToken() == "" ||
		capability.ExpiresAt().IsZero() || !capability.ExpiresAt().After(time.Now()) {
		service.revokeCapability(ctx, capability, command.Audience)
		return nil, ErrBuildArtifactPublishUnavailable
	}

	result, publishErr := publisher.PublishArtifact(ctx, command.ProviderExecutionID, infrasandbox.ArtifactPublishRequest{
		Descriptor: descriptor, UploadURL: uploadURL, Token: capability.BearerToken(), ExpiresAt: capability.ExpiresAt(),
	})
	verified, verifyErr := service.store.VerifyBuildArtifact(ctx, objectKey, digest, descriptor.Size)
	if verifyErr == nil && verified {
		return newBuildArtifactReceipt(objectKey, digest, descriptor.Size), nil
	}
	service.revokeCapability(ctx, capability, command.Audience)
	if publishErr != nil {
		return nil, normalizeBuildArtifactPublishError(ctx, publishErr)
	}
	if verifyErr != nil || !result.Accepted {
		return nil, normalizeBuildArtifactPublishError(ctx, verifyErr)
	}
	return nil, ErrBuildArtifactPublishUnavailable
}

func normalizeBuildArtifactCommand(command PublishBuildArtifactCommand) (infrasandbox.ArtifactDescriptor, domainappdev.ArtifactGrantDigest, error) {
	if domainappdev.ValidateArtifactGrantAudience(command.Audience) != nil ||
		command.Audience.ProviderScope != domainsandbox.ScopeAppDev || command.Audience.Operation != BuildArtifactPublishOperation ||
		command.Generation == 0 || !validBuildArtifactExecutionID(command.ProviderExecutionID) {
		return infrasandbox.ArtifactDescriptor{}, domainappdev.ArtifactGrantDigest{}, domainappdev.ErrArtifactGrantInvalid
	}
	descriptor, err := infrasandbox.NormalizeArtifactDescriptor(command.Descriptor)
	if err != nil || descriptor.Size > domainappdev.MaxArtifactGrantObjectBytes {
		return infrasandbox.ArtifactDescriptor{}, domainappdev.ArtifactGrantDigest{}, domainappdev.ErrArtifactGrantInvalid
	}
	digest, err := domainappdev.ParseArtifactGrantDigest(descriptor.Digest)
	if err != nil || digest.IsZero() {
		return infrasandbox.ArtifactDescriptor{}, domainappdev.ArtifactGrantDigest{}, domainappdev.ErrArtifactGrantInvalid
	}
	return descriptor, digest, nil
}

func deriveBuildArtifactObjectKey(command PublishBuildArtifactCommand, descriptor infrasandbox.ArtifactDescriptor) string {
	hasher := sha256.New()
	for _, value := range []string{
		"appdev-build-artifact-v1", command.Audience.SpaceID, command.Audience.ProjectID,
		strconv.FormatUint(command.Generation, 10), command.Audience.ProviderKey,
		string(command.Audience.ProviderScope), command.Audience.Operation,
		command.ProviderExecutionID, string(descriptor.Kind), descriptor.Digest, strconv.FormatInt(descriptor.Size, 10),
	} {
		_, _ = hasher.Write([]byte{byte(len(value) >> 8), byte(len(value))})
		_, _ = hasher.Write([]byte(value))
	}
	identity := hex.EncodeToString(hasher.Sum(nil))
	return "appdev/build-artifacts/v1/spaces/" + command.Audience.SpaceID +
		"/projects/" + command.Audience.ProjectID + "/generations/" +
		strconv.FormatUint(command.Generation, 10) + "/" + identity + ".zip"
}

func (service *BuildArtifactPublisherService) revokeCapability(ctx context.Context, capability *ArtifactGrantCapability, audience domainappdev.ArtifactGrantAudience) {
	if capability == nil {
		return
	}
	revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), buildArtifactRevokeTimeout)
	defer cancel()
	_ = service.grants.Revoke(revokeCtx, RevokeArtifactGrantRequest{
		GrantID: capability.GrantID(), Token: capability.token, Audience: audience,
		Direction: domainappdev.ArtifactGrantDirectionUpload,
	})
}

func newBuildArtifactReceipt(objectKey string, digest domainappdev.ArtifactGrantDigest, size int64) *BuildArtifactReceipt {
	return &BuildArtifactReceipt{objectKey: objectKey, digest: digest, size: size}
}

func validBuildArtifactUploadURL(value string) bool {
	return NewArtifactCapabilityURLPolicy(false).Validate(value) == nil
}

func validBuildArtifactExecutionID(value string) bool {
	if value == "" || len(value) > infrasandbox.MaxIdentifierBytes {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || index > 0 && strings.ContainsRune("._-:", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func normalizeBuildArtifactPublishError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return ErrBuildArtifactPublishUnavailable
}

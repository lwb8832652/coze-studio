// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

var ErrProviderHTTPWiringInvalid = errors.New("appdev provider http wiring is invalid")

type ProviderPreviewURLProjector interface {
	ProjectAppDevPreviewURL(context.Context, string, string, string) (string, error)
}

type ProviderArtifactGatewayService interface {
	Upload(context.Context, UploadArtifactRequest) (*ArtifactUploadResult, error)
	OpenDownload(context.Context, DownloadArtifactRequest) (*ArtifactGatewayDownloadResponse, error)
}

type ProviderArtifactAuthenticator interface {
	AuthenticateArtifactProvider(context.Context, string, string) (domainappdev.ArtifactGrantAudience, error)
}

type ProviderSecureTransportInput struct {
	DirectHTTPS    bool
	RemoteAddress  string
	ForwardedProto string
}

type ProviderSecureTransportVerifier interface {
	VerifyArtifactProviderTransport(context.Context, ProviderSecureTransportInput) error
}

type ProviderHTTPDependencies struct {
	gateway       ProviderArtifactGatewayService
	authenticator ProviderArtifactAuthenticator
	transport     ProviderSecureTransportVerifier
	projector     ProviderPreviewURLProjector
}

func NewProviderHTTPDependencies(
	gateway ProviderArtifactGatewayService,
	authenticator ProviderArtifactAuthenticator,
	transport ProviderSecureTransportVerifier,
	projector ProviderPreviewURLProjector,
) (*ProviderHTTPDependencies, error) {
	if gateway == nil || authenticator == nil || transport == nil || projector == nil {
		return nil, ErrProviderHTTPWiringInvalid
	}
	return &ProviderHTTPDependencies{
		gateway: gateway, authenticator: authenticator, transport: transport, projector: projector,
	}, nil
}

func (dependencies *ProviderHTTPDependencies) ArtifactGateway() ProviderArtifactGatewayService {
	if dependencies == nil {
		return nil
	}
	return dependencies.gateway
}

func (dependencies *ProviderHTTPDependencies) ArtifactAuthenticator() ProviderArtifactAuthenticator {
	if dependencies == nil {
		return nil
	}
	return dependencies.authenticator
}

func (dependencies *ProviderHTTPDependencies) SecureTransportVerifier() ProviderSecureTransportVerifier {
	if dependencies == nil {
		return nil
	}
	return dependencies.transport
}

func (dependencies *ProviderHTTPDependencies) PreviewURLProjector() ProviderPreviewURLProjector {
	if dependencies == nil {
		return nil
	}
	return dependencies.projector
}

func (*ProviderHTTPDependencies) String() string {
	return "ProviderHTTPDependencies{capabilities:<redacted>}"
}

func (*ProviderHTTPDependencies) GoString() string {
	return "ProviderHTTPDependencies{capabilities:<redacted>}"
}

func (*ProviderHTTPDependencies) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderHTTPDependencies{capabilities:<redacted>}")
}

func (*ProviderHTTPDependencies) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderHTTPWiringInvalid
}

type providerHTTPRegistryEntry struct {
	dependencies *ProviderHTTPDependencies
	generation   uint64
}

type ProviderHTTPPublication struct {
	entry *providerHTTPRegistryEntry
}

func (publication ProviderHTTPPublication) IsCurrent() bool {
	return publication.entry != nil && providerHTTPDependenciesRegistry.Load() == publication.entry
}

func (ProviderHTTPPublication) String() string {
	return "ProviderHTTPPublication{owner:<redacted>}"
}

func (ProviderHTTPPublication) GoString() string {
	return "ProviderHTTPPublication{owner:<redacted>}"
}

func (ProviderHTTPPublication) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderHTTPPublication{owner:<redacted>}")
}

var (
	providerHTTPDependenciesRegistry atomic.Pointer[providerHTTPRegistryEntry]
	providerHTTPGeneration           atomic.Uint64
)

func PublishProviderHTTPDependencies(dependencies *ProviderHTTPDependencies) (ProviderHTTPPublication, error) {
	if dependencies == nil || dependencies.gateway == nil || dependencies.authenticator == nil ||
		dependencies.transport == nil || dependencies.projector == nil {
		return ProviderHTTPPublication{}, ErrProviderHTTPWiringInvalid
	}
	entry := &providerHTTPRegistryEntry{
		dependencies: dependencies,
		generation:   providerHTTPGeneration.Add(1),
	}
	providerHTTPDependenciesRegistry.Store(entry)
	return ProviderHTTPPublication{entry: entry}, nil
}

func UnpublishProviderHTTPDependencies(publication ProviderHTTPPublication) bool {
	if publication.entry == nil {
		return false
	}
	return providerHTTPDependenciesRegistry.CompareAndSwap(publication.entry, nil)
}

func CurrentProviderHTTPDependencies() *ProviderHTTPDependencies {
	current := providerHTTPDependenciesRegistry.Load()
	if current == nil || current.dependencies == nil {
		return nil
	}
	return current.dependencies
}

var _ fmt.Stringer = (*ProviderHTTPDependencies)(nil)
var _ fmt.GoStringer = (*ProviderHTTPDependencies)(nil)
var _ json.Marshaler = (*ProviderHTTPDependencies)(nil)
var _ fmt.Stringer = ProviderHTTPPublication{}
var _ fmt.GoStringer = ProviderHTTPPublication{}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type providerWiringGatewayFake struct{}

func (providerWiringGatewayFake) Upload(context.Context, UploadArtifactRequest) (*ArtifactUploadResult, error) {
	return &ArtifactUploadResult{}, nil
}
func (providerWiringGatewayFake) OpenDownload(context.Context, DownloadArtifactRequest) (*ArtifactGatewayDownloadResponse, error) {
	return &ArtifactGatewayDownloadResponse{}, nil
}

type providerWiringAuthenticatorFake struct{}

func (providerWiringAuthenticatorFake) AuthenticateArtifactProvider(context.Context, string, string) (domainappdev.ArtifactGrantAudience, error) {
	return domainappdev.ArtifactGrantAudience{}, nil
}

type providerWiringTransportFake struct{}

func (providerWiringTransportFake) VerifyArtifactProviderTransport(context.Context, ProviderSecureTransportInput) error {
	return nil
}

type providerWiringProjectorFake struct{}

func (providerWiringProjectorFake) ProjectAppDevPreviewURL(context.Context, string, string, string) (string, error) {
	return "https://preview.example.test/opaque", nil
}

func TestProviderHTTPDependenciesRegistryDefaultsFailClosedAndUsesOwnedPublication(t *testing.T) {
	require.Nil(t, CurrentProviderHTTPDependencies())

	dependencies, err := NewProviderHTTPDependencies(
		providerWiringGatewayFake{}, providerWiringAuthenticatorFake{},
		providerWiringTransportFake{}, providerWiringProjectorFake{},
	)
	require.NoError(t, err)
	publication, err := PublishProviderHTTPDependencies(dependencies)
	require.NoError(t, err)
	t.Cleanup(func() { UnpublishProviderHTTPDependencies(publication) })
	require.NotNil(t, CurrentProviderHTTPDependencies())
	require.NotContains(t, fmt.Sprintf("%v %+v %#v", dependencies, dependencies, dependencies), "preview.example.test")
	_, err = json.Marshal(dependencies)
	require.Error(t, err)

	require.True(t, UnpublishProviderHTTPDependencies(publication))
	require.Nil(t, CurrentProviderHTTPDependencies())
}

func TestProviderHTTPRegistryOldOwnerCannotClearNewPublication(t *testing.T) {
	require.Nil(t, CurrentProviderHTTPDependencies())
	first, err := NewProviderHTTPDependencies(
		providerWiringGatewayFake{}, providerWiringAuthenticatorFake{},
		providerWiringTransportFake{}, providerWiringProjectorFake{},
	)
	require.NoError(t, err)
	second, err := NewProviderHTTPDependencies(
		providerWiringGatewayFake{}, providerWiringAuthenticatorFake{},
		providerWiringTransportFake{}, providerWiringProjectorFake{},
	)
	require.NoError(t, err)

	firstOwner, err := PublishProviderHTTPDependencies(first)
	require.NoError(t, err)
	secondOwner, err := PublishProviderHTTPDependencies(second)
	require.NoError(t, err)
	t.Cleanup(func() { UnpublishProviderHTTPDependencies(secondOwner) })

	require.False(t, UnpublishProviderHTTPDependencies(firstOwner))
	require.Same(t, second, CurrentProviderHTTPDependencies())
	require.True(t, UnpublishProviderHTTPDependencies(secondOwner))
	require.Nil(t, CurrentProviderHTTPDependencies())
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type artifactURLPolicyEndpoint struct{ value string }

func (endpoint artifactURLPolicyEndpoint) ArtifactDownloadEndpoint(context.Context, domainappdev.ArtifactGrantID) (string, error) {
	return endpoint.value, nil
}
func (endpoint artifactURLPolicyEndpoint) ArtifactUploadEndpoint(context.Context, domainappdev.ArtifactGrantID) (string, error) {
	return endpoint.value, nil
}

func TestArtifactCapabilityURLPolicyIsSharedByRuntimeAndBuild(t *testing.T) {
	debugPolicy := NewArtifactCapabilityURLPolicy(true)
	require.NoError(t, debugPolicy.Validate("http://127.0.0.1:8080/internal/artifacts/grant"))
	require.Error(t, debugPolicy.Validate("http://localhost:8080/internal/artifacts/grant"))
	require.Error(t, NewArtifactCapabilityURLPolicy(false).Validate("http://127.0.0.1:8080/internal/artifacts/grant"))

	runtime, _, _, _, _, selection, _ := runtimeStartFixture(t)
	// ArtifactReference remains canonical HTTPS even when the separately
	// injected debug policy permits a loopback control endpoint.
	const artifactEndpoint = "https://artifacts.example.test/internal/artifacts/grant"
	runtime.endpoints = artifactURLPolicyEndpoint{value: artifactEndpoint}
	runtime.config.ArtifactURLPolicy = debugPolicy
	_, err := runtime.Start(context.Background(), ProviderRuntimeStartInput{
		SpaceID: runtimeStartSpace, ProjectID: runtimeStartProject,
		OperationID: runtimeStartOperation, ActorID: runtimeStartActor,
	})
	require.NoError(t, err)
	require.Len(t, selection.executeRequests, 1)

	content := []byte("build")
	command := buildArtifactFixture(content)
	grants := &buildArtifactGrantFake{}
	store := &buildArtifactStoreFake{objects: map[string][]byte{}}
	publisher, err := NewBuildArtifactPublisherService(
		grants,
		artifactURLPolicyEndpoint{value: artifactEndpoint},
		store,
		DefaultBuildArtifactGrantTTL,
		WithBuildArtifactURLPolicy(debugPolicy),
	)
	require.NoError(t, err)
	provider := &buildArtifactRuntime{publish: func(_ context.Context, _ string, request infrasandbox.ArtifactPublishRequest) (infrasandbox.ArtifactPublishResult, error) {
		spec := grants.specs[request.Token]
		store.objects[spec.ObjectKey] = content
		return infrasandbox.ArtifactPublishResult{Accepted: true}, nil
	}}
	_, err = publisher.Publish(context.Background(), provider, command)
	require.NoError(t, err)
}

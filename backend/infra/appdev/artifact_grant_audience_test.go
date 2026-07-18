// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestArtifactGrantRedisResolvesOnlyBoundedAudienceMetadata(t *testing.T) {
	repository, _ := newArtifactGrantRedisRepositoryForTest(t)
	grantID, token, spec := artifactGrantRepositoryFixture(t, 0x51)
	issueArtifactGrantForRepositoryTest(t, repository, grantID, token, spec, time.Minute)

	audience, err := repository.ResolveArtifactGrantAudience(context.Background(), grantID)
	require.NoError(t, err)
	require.Equal(t, spec.Audience, audience)
	require.NotContains(t, fmt.Sprintf("%+v", audience), spec.ObjectKey)
	require.NotContains(t, fmt.Sprintf("%+v", audience), token.Bearer())
}

func TestArtifactGrantRedisAudienceResolutionFailsClosed(t *testing.T) {
	repository, _ := NewRedisArtifactGrantRepository(artifactGrantScriptRunnerStub{
		err: errors.New("redis raw token/hash/object-key"),
	})
	grantID, _, _ := artifactGrantRepositoryFixture(t, 0x61)
	audience, err := repository.ResolveArtifactGrantAudience(context.Background(), grantID)
	require.Equal(t, domainappdev.ArtifactGrantAudience{}, audience)
	require.ErrorIs(t, err, domainappdev.ErrArtifactGrantUnavailable)
	require.NotContains(t, err.Error(), "raw")
}

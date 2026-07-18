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
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/stretchr/testify/require"
)

func TestProviderBuildPublicErrorPipelineRejectsProviderMessage(t *testing.T) {
	malicious := "https://provider.invalid/build?token=secret Bearer api-key s3://bucket/private/object\n" + strings.Repeat("raw", 200)
	providerObservation, err := infrasandbox.NormalizeBuildObservation(infrasandbox.BuildObservation{
		Status:           infrasandbox.BuildStatusFailed,
		SafeErrorCode:    "provider_raw_failure",
		SafeErrorMessage: malicious,
	})
	require.NoError(t, err)

	orchestratorProjection := providerBuildProjection(ProviderExecutionMetadata{
		Generation:               19,
		ArtifactStatus:           domainappdev.ProviderExecutionArtifactFailed,
		ArtifactSafeErrorCode:    providerObservation.SafeErrorCode,
		ArtifactSafeErrorMessage: malicious,
	})
	require.Equal(t, infrasandbox.BuildSafeErrorCodeBuildFailed, orchestratorProjection.SafeErrorCode)
	require.Equal(t, infrasandbox.BuildSafeErrorMessageBuildFailed, orchestratorProjection.SafeMessage)

	apiProjection, err := providerBuildAPIResult(context.Background(), orchestratorProjection, nil)
	require.NoError(t, err)
	require.Equal(t, infrasandbox.BuildSafeErrorCodeBuildFailed, apiProjection.SafeErrorCode)
	require.Equal(t, infrasandbox.BuildSafeErrorMessageBuildFailed, apiProjection.SafeMessage)

	encoded, err := json.Marshal(apiProjection)
	require.NoError(t, err)
	public := fmt.Sprintf("%v %+v %#v %s", apiProjection, apiProjection, apiProjection, encoded)
	for _, secret := range []string{"provider.invalid", "token=secret", "Bearer", "api-key", "s3://", "private/object", "rawraw"} {
		require.NotContains(t, public, secret)
	}
}

func TestProviderBuildPublicErrorPipelineKeepsAllowlistedCodeWithServerMessage(t *testing.T) {
	providerObservation, err := infrasandbox.NormalizeBuildObservation(infrasandbox.BuildObservation{
		Status:           infrasandbox.BuildStatusFailed,
		SafeErrorCode:    infrasandbox.BuildSafeErrorCodeDependencyFailed,
		SafeErrorMessage: "provider controlled explanation",
	})
	require.NoError(t, err)

	projection := providerBuildProjection(ProviderExecutionMetadata{
		Generation:            20,
		ArtifactStatus:        domainappdev.ProviderExecutionArtifactFailed,
		ArtifactSafeErrorCode: providerObservation.SafeErrorCode,
	})
	require.Equal(t, infrasandbox.BuildSafeErrorCodeDependencyFailed, projection.SafeErrorCode)
	require.Equal(t, infrasandbox.BuildSafeErrorMessageDependencyFailed, projection.SafeMessage)
}

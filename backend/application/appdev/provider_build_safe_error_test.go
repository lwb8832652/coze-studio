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
	"testing"

	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/stretchr/testify/require"
)

func TestProviderBuildAPIResultDefensivelyNormalizesFailure(t *testing.T) {
	const malicious = "https://provider.invalid/build?token=secret Bearer api-key minio://bucket/private/object\x00raw-body"

	projection, err := providerBuildAPIResult(context.Background(), &ProviderBuildProjection{
		Generation:    7,
		State:         ProviderBuildStateFailed,
		SafeErrorCode: "provider_raw_failure",
		SafeMessage:   malicious,
	}, nil)
	require.NoError(t, err)

	require.Equal(t, infrasandbox.BuildSafeErrorCodeBuildFailed, projection.SafeErrorCode)
	require.Equal(t, infrasandbox.BuildSafeErrorMessageBuildFailed, projection.SafeMessage)

	encoded, err := json.Marshal(projection)
	require.NoError(t, err)
	formatted := fmt.Sprintf("%v %+v %#v %s", projection, projection, projection, encoded)
	for _, secret := range []string{"provider.invalid", "token=secret", "Bearer", "api-key", "minio://", "private/object", "raw-body"} {
		require.NotContains(t, formatted, secret)
	}
}

func TestProviderBuildAPIResultKeepsAllowlistedCodeWithFixedMessage(t *testing.T) {
	projection, err := providerBuildAPIResult(context.Background(), &ProviderBuildProjection{
		Generation:    8,
		State:         ProviderBuildStateFailed,
		SafeErrorCode: infrasandbox.BuildSafeErrorCodeSourceInvalid,
		SafeMessage:   "provider controlled message",
	}, nil)
	require.NoError(t, err)

	require.Equal(t, infrasandbox.BuildSafeErrorCodeSourceInvalid, projection.SafeErrorCode)
	require.Equal(t, infrasandbox.BuildSafeErrorMessageSourceInvalid, projection.SafeMessage)
}

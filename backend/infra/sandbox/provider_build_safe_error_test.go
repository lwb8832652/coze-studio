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

package sandbox

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBuildObservationUsesServerSafeErrorAllowlist(t *testing.T) {
	const malicious = "https://provider.invalid/build?token=secret Bearer api-key s3://bucket/private/object\nraw-body"

	tests := []struct {
		name        string
		code        string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "allowlisted code ignores provider message",
			code:        BuildSafeErrorCodeSourceInvalid,
			wantCode:    BuildSafeErrorCodeSourceInvalid,
			wantMessage: BuildSafeErrorMessageSourceInvalid,
		},
		{
			name:        "unknown code becomes generic failure",
			code:        "provider_raw_failure",
			wantCode:    BuildSafeErrorCodeBuildFailed,
			wantMessage: BuildSafeErrorMessageBuildFailed,
		},
		{
			name:        "empty code becomes generic failure",
			code:        "",
			wantCode:    BuildSafeErrorCodeBuildFailed,
			wantMessage: BuildSafeErrorMessageBuildFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observation, err := NormalizeBuildObservation(BuildObservation{
				Status:           BuildStatusFailed,
				SafeErrorCode:    tt.code,
				SafeErrorMessage: malicious,
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantCode, observation.SafeErrorCode)
			require.Equal(t, tt.wantMessage, observation.SafeErrorMessage)

			formatted := fmt.Sprintf("%v %+v %#v", observation, observation, observation)
			for _, secret := range []string{"provider.invalid", "token=secret", "Bearer", "api-key", "s3://", "private/object", "raw-body"} {
				require.NotContains(t, formatted, secret)
			}
			require.False(t, strings.ContainsRune(formatted, '\n'))
		})
	}
}

func TestNormalizeBuildObservationRejectsProviderErrorOnNonFailedStatus(t *testing.T) {
	_, err := NormalizeBuildObservation(BuildObservation{
		Status:           BuildStatusRunning,
		SafeErrorCode:    BuildSafeErrorCodeBuildFailed,
		SafeErrorMessage: "provider supplied message",
	})
	require.Error(t, err)
}

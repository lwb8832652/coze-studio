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

package conv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDebugJsonToStrRedactsCredentialsRecursively(t *testing.T) {
	value := map[string]any{
		"api_key": "api-sensitive-marker",
		"nested": []any{
			map[string]any{
				"password":      "password-sensitive-marker",
				"authorization": "authorization-sensitive-marker",
				"safe":          "visible-value",
				"input_tokens":  42,
			},
		},
		"headers": map[string]any{
			"X-API-Key":  "header-sensitive-marker",
			"X-Trace-ID": "trace-value",
		},
		"credential_ciphertext": "credential-sensitive-marker",
	}

	got := DebugJsonToStr(value)

	for _, marker := range []string{
		"api-sensitive-marker",
		"password-sensitive-marker",
		"authorization-sensitive-marker",
		"header-sensitive-marker",
		"credential-sensitive-marker",
	} {
		require.NotContains(t, got, marker)
	}
	require.Contains(t, got, `"safe":"visible-value"`)
	require.Contains(t, got, `"input_tokens":42`)
	require.Contains(t, got, `"X-Trace-ID":"trace-value"`)
	require.Contains(t, got, `"[REDACTED]"`)
}

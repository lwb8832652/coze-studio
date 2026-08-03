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

package storage

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetOptionsSupportSignedResponseHeaderOverrides(t *testing.T) {
	option := GetOption{}

	WithResponseContentDisposition("attachment; filename*=UTF-8''report.html")(&option)
	WithResponseContentType("text/html; charset=utf-8")(&option)
	WithResponseCacheControl("private, no-store")(&option)

	require.Equal(
		t,
		"attachment; filename*=UTF-8''report.html",
		option.ResponseContentDisposition,
	)
	require.Equal(t, "text/html; charset=utf-8", option.ResponseContentType)
	require.Equal(t, "private, no-store", option.ResponseCacheControl)
}

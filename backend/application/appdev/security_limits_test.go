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
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeAppDevOutput(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	output := SanitizeAppDevOutput(fmt.Sprintf(
		"API_KEY=secret-value Authorization: Bearer abc.def.ghi path=%s/project",
		home,
	))

	require.NotContains(t, output, "secret-value")
	require.NotContains(t, output, "abc.def.ghi")
	require.NotContains(t, output, home)
	require.GreaterOrEqual(t, strings.Count(output, "[REDACTED]"), 2)
}

func TestValidateUploadFilesRejectsMoreThanOneHundredFiles(t *testing.T) {
	items := make([]UploadFileItem, 101)
	for index := range items {
		items[index] = UploadFileItem{
			Path:    fmt.Sprintf("src/file-%03d.ts", index),
			Content: []byte("export {};"),
		}
	}

	_, err := validateUploadFiles(items)

	require.ErrorContains(t, err, "cannot exceed 100")
}

func TestAppDevHostExecutionRequiresExplicitDebugOptIn(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	require.False(t, IsAppDevHostExecutionEnabled())

	t.Setenv("APP_ENV", "debug")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	require.False(t, IsAppDevHostExecutionEnabled())

	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	require.True(t, IsAppDevHostExecutionEnabled())
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppDevProviderDocumentationMatchesProductionWiring(t *testing.T) {
	files := []string{
		"../../docs/superpowers/specs/2026-07-15-nuwax-sandbox-parity-design.md",
		"../../docs/superpowers/runbooks/local-debug-and-test.md",
		"../../AGENTS.md",
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err, file)
		text := string(content)
		require.NotContains(t, text, "APP_DEV_RUNNER_ENDPOINT=", file)
		require.NotContains(t, text, "APP_DEV_RUNNER_TOKEN=", file)
		require.Contains(t, text, "APP_DEV_RUNNER_ENDPOINT", file)
		require.Contains(t, text, "APP_DEV_RUNNER_TOKEN", file)
		for _, required := range []string{
			"APP_DEV_PREVIEW_GATEWAY_BASE_URL",
			"APP_DEV_ARTIFACT_GATEWAY_BASE_URL",
			"SANDBOX_CREDENTIAL_KEYS_JSON",
			"SANDBOX_CREDENTIAL_ACTIVE_KEY_ID",
		} {
			require.True(t, strings.Contains(text, required), "%s must document %s", file, required)
		}
	}
}

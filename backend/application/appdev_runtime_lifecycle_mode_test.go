// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"testing"

	"github.com/stretchr/testify/require"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
)

func TestAppDevProjectRuntimeLifecycleModeFromEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want appdevapp.ProjectRuntimeLifecycleMode
	}{
		{
			name: "production remains provider required when host flag is set",
			env:  map[string]string{sandboxAppEnv: "production", sandboxHostRuntimeEnabledEnv: "true"},
			want: appdevapp.ProjectRuntimeLifecycleModeProviderRequired,
		},
		{
			name: "debug without host gate remains provider required",
			env:  map[string]string{sandboxAppEnv: "debug", sandboxHostRuntimeEnabledEnv: "false"},
			want: appdevapp.ProjectRuntimeLifecycleModeProviderRequired,
		},
		{
			name: "debug double gate permits legacy runtime",
			env:  map[string]string{sandboxAppEnv: "debug", sandboxHostRuntimeEnabledEnv: "true"},
			want: appdevapp.ProjectRuntimeLifecycleModeLegacyDebug,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, appDevProjectRuntimeLifecycleMode(func(key string) string {
				return test.env[key]
			}))
		})
	}
}

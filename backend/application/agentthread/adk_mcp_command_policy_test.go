// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeBootstrapConfigLoadsOperatorCommandRules(t *testing.T) {
	root := t.TempDir()
	npx := writeADKMCPBootstrapExecutable(t, root, "npx")
	node := writeADKMCPBootstrapExecutable(t, root, "node")
	custom := writeADKMCPBootstrapExecutable(t, root, "custom-server")
	scriptRoot := filepath.Join(root, "scripts")
	require.NoError(t, os.Mkdir(scriptRoot, 0o700))
	rules, err := json.Marshal([]map[string]any{{
		"command":     custom,
		"argv_prefix": []string{"serve", "--stdio"},
	}})
	require.NoError(t, err)

	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv("APP_ENV", "debug")
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioEinoEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioDebugHostExecutionEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirRootEnv, root)
	t.Setenv(agentThreadMCPStdioWorkerIDEnv, "worker-command-policy")
	t.Setenv(agentThreadMCPStdioAllowedCommandsEnv, npx+","+node+","+custom)
	t.Setenv(agentThreadMCPStdioAllowedNpxPackagesEnv, "@modelcontextprotocol/server-github,open-meteo-mcp-server")
	t.Setenv(agentThreadMCPStdioAllowedUVXPackagesEnv, "official-uv-tool")
	t.Setenv(agentThreadMCPStdioAllowedNodeScriptRootsEnv, scriptRoot)
	t.Setenv(agentThreadMCPStdioCommandRulesJSONEnv, string(rules))

	config, err := ADKMCPRuntimeBootstrapConfigFromEnv()
	require.NoError(t, err)
	require.Equal(t, []string{"@modelcontextprotocol/server-github", "open-meteo-mcp-server"}, config.StdioAllowedNpxPackages)
	require.Equal(t, []string{"official-uv-tool"}, config.StdioAllowedUVXPackages)
	require.Equal(t, []string{scriptRoot}, config.StdioAllowedNodeScriptRoots)
	require.Len(t, config.StdioCommandRules, 1)
	require.Equal(t, custom, config.StdioCommandRules[0].Command)
	require.Equal(t, []string{"serve", "--stdio"}, config.StdioCommandRules[0].ArgvPrefix)
}

func TestADKMCPRuntimeBootstrapDefaultsOnlyOfficialDeerFlowNpxPackages(t *testing.T) {
	root := t.TempDir()
	npx := writeADKMCPBootstrapExecutable(t, root, "npx")

	clearADKMCPRuntimeBootstrapEnv(t)
	t.Setenv("APP_ENV", "debug")
	t.Setenv(agentThreadMCPRuntimeEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioEinoEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioDebugHostExecutionEnabledEnv, "true")
	t.Setenv(agentThreadMCPStdioWorkdirRootEnv, root)
	t.Setenv(agentThreadMCPStdioWorkerIDEnv, "worker-default-package")
	t.Setenv(agentThreadMCPStdioAllowedCommandsEnv, npx)
	t.Setenv(agentThreadMCPStdioAllowedNpxPackagesEnv, "")
	t.Setenv(agentThreadMCPStdioAllowedUVXPackagesEnv, "")

	config, err := ADKMCPRuntimeBootstrapConfigFromEnv()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"@modelcontextprotocol/server-github",
		"@modelcontextprotocol/server-postgres",
		"open-meteo-mcp-server",
	}, config.StdioAllowedNpxPackages)
	require.Empty(t, config.StdioAllowedUVXPackages)
}

func writeADKMCPBootstrapExecutable(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(name), 0o700))
	return path
}

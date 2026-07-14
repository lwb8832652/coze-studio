// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

const (
	adkMCPStdioSecurityHelperEnv     = "COZE_ADK_MCP_STDIO_SECURITY_HELPER"
	adkMCPStdioSecurityHelperEnvFile = "COZE_ADK_MCP_STDIO_SECURITY_ENV_FILE"
)

func TestADKMCPStdioFactoryUsesMinimalEnvironmentAndContinuouslyDrainsStderr(t *testing.T) {
	t.Setenv("HOST_MCP_SECRET", "must-not-reach-child")
	t.Setenv("DATABASE_URL", "mysql://must-not-reach-child")
	t.Setenv("MCP_AES_AUTH_SECRET", "must-not-reach-child")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/must/not/reach/child")

	executable, err := os.Executable()
	require.NoError(t, err)
	root := t.TempDir()
	envFile := filepath.Join(root, "child-env.json")
	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	execution.Command = executable
	execution.Args = []string{"-test.run=^TestADKMCPStdioSecurityHelperProcess$"}
	execution.Env = map[string]string{
		adkMCPStdioSecurityHelperEnv:     "1",
		adkMCPStdioSecurityHelperEnvFile: envFile,
	}
	require.NoError(t, os.MkdirAll(execution.WorkingDir, 0o700))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	factory := NewADKMCPRuntimeStdioEinoMCPClientFactory(
		ADKMCPRuntimeStdioEinoMCPClientFactoryOptions{
			ExecutionMode: mcpruntime.NewStdioExecutionMode("debug", true),
		},
	)
	client, err := factory.NewADKMCPRuntimeStdioEinoClient(ctx, execution)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NoError(t, client.Close())

	data, err := os.ReadFile(envFile)
	require.NoError(t, err)
	var childEnv []string
	require.NoError(t, json.Unmarshal(data, &childEnv))
	values := make(map[string]string, len(childEnv))
	for _, item := range childEnv {
		for index := 0; index < len(item); index++ {
			if item[index] == '=' {
				values[item[:index]] = item[index+1:]
				break
			}
		}
	}
	for _, forbidden := range []string{
		"HOST_MCP_SECRET",
		"DATABASE_URL",
		"MCP_AES_AUTH_SECRET",
		"GOOGLE_APPLICATION_CREDENTIALS",
	} {
		require.NotContains(t, values, forbidden)
	}
	realExecutable, err := filepath.EvalSymlinks(executable)
	require.NoError(t, err)
	require.Equal(t, filepath.Dir(realExecutable), values["PATH"])
	for key := range values {
		require.Contains(t, []string{
			adkMCPStdioSecurityHelperEnv,
			adkMCPStdioSecurityHelperEnvFile,
			"PATH",
		}, key)
	}
}

func TestADKMCPStdioSecurityHelperProcess(t *testing.T) {
	if os.Getenv(adkMCPStdioSecurityHelperEnv) != "1" {
		return
	}
	envBytes, err := json.Marshal(os.Environ())
	if err != nil || os.WriteFile(os.Getenv(adkMCPStdioSecurityHelperEnvFile), envBytes, 0o600) != nil {
		os.Exit(2)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.Method != "initialize" {
			continue
		}
		chunk := bytes.Repeat([]byte("adk-stderr-must-be-drained\n"), 4096)
		for index := 0; index < 32; index++ {
			if _, err := os.Stderr.Write(chunk); err != nil {
				os.Exit(3)
			}
		}
		_, _ = fmt.Fprintf(
			os.Stdout,
			`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"%s","capabilities":{},"serverInfo":{"name":"adk-security-helper","version":"1"}}}`+"\n",
			request.ID,
			mcpsdk.LATEST_PROTOCOL_VERSION,
		)
	}
	os.Exit(0)
}

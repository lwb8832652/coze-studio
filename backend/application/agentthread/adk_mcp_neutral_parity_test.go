// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	toolmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestADKMCPNeutralParityRejectsCanonicalDuplicateHeaders(t *testing.T) {
	transport := NewADKMCPRuntimeRemoteTransport(
		ADKMCPRuntimeRemoteTransportOptions{
			AllowedHosts:   []string{"mcp.example.com"},
			MaxConfigBytes: 4096,
			MaxHeaders:     4,
			MaxHeaderBytes: 1024,
		},
	)

	_, err := transport.parseConfig(ADKMCPRuntimeTransportCall{
		Server: &toolmodel.MCPToolServer{
			ServerType: "sse",
			Config: `{
				"url":"https://mcp.example.com/events",
				"headers":{"Authorization":"one","authorization":"two"}
			}`,
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp runtime remote config is invalid")
}

func TestADKMCPNeutralParityRejectsInvalidAllowedEnvName(t *testing.T) {
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{
			AllowedCommands:  []string{"node"},
			AllowedEnvKeys:   []string{"BAD=KEY"},
			MaxArgs:          2,
			MaxArgBytes:      128,
			MaxEnvVars:       2,
			MaxEnvValueBytes: 128,
		},
	)

	err := policy.ValidateADKMCPRuntimeStdio(
		context.Background(),
		ADKMCPRuntimeStdioSandboxCall{
			Config: ADKMCPRuntimeStdioConfig{
				Command: "node",
				Args:    []string{},
				Env:     map[string]string{"BAD=KEY": "value"},
			},
		},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp runtime stdio env is not allowed")
}

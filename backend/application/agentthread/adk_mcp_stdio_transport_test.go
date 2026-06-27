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

package agentthread

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

func TestADKMCPRuntimeStdioTransportFailsClosedWithoutPolicy(t *testing.T) {
	sandbox := &recordingADKMCPRuntimeStdioSandbox{result: "should-not-run"}
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{Sandbox: sandbox},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio policy is not configured")
	require.Zero(t, sandbox.calls)
	assertADKMCPStdioErrorDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeStdioTransportFailsClosedWithoutSandbox(t *testing.T) {
	policy := &recordingADKMCPRuntimeStdioPolicy{}
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{Policy: policy},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio sandbox is not configured")
	require.Equal(t, 1, policy.calls)
	assertADKMCPStdioErrorDoesNotLeak(t, err.Error())
}

func TestADKMCPRuntimeStdioTransportParsesConfigAndDelegates(t *testing.T) {
	policy := &recordingADKMCPRuntimeStdioPolicy{}
	sandbox := &recordingADKMCPRuntimeStdioSandbox{
		result: `{"schema":"coze.mcp_stdio_result.v1","content":"ok"}`,
	}
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{
			Policy:  policy,
			Sandbox: sandbox,
		},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.NoError(t, err)
	require.Equal(t, sandbox.result, result)
	require.Equal(t, 1, policy.calls)
	require.Equal(t, 1, sandbox.calls)
	require.Equal(t, int64(20), sandbox.call.Run.RunID)
	require.Equal(t, int64(100), sandbox.call.ServerID)
	require.Equal(t, "mcp_100_search_docs", sandbox.call.Name)
	require.Equal(t, "search-docs", sandbox.call.ToolName)
	require.Equal(t, `{"query":"secret customer path"}`, sandbox.call.Arguments)
	require.Equal(t, "npx", sandbox.call.Config.Command)
	require.Equal(t, []string{"-y", "@example/secret-mcp-server"}, sandbox.call.Config.Args)
	require.Equal(t, "/secret/workdir", sandbox.call.Config.WorkingDir)
	require.Equal(t, "stdio-secret-token", sandbox.call.Config.Env["API_TOKEN"])
	require.Equal(t, sandbox.call, policy.call)
}

func TestADKMCPRuntimeStdioTransportProjectsAuthEnv(t *testing.T) {
	policy := &recordingADKMCPRuntimeStdioPolicy{}
	sandbox := &recordingADKMCPRuntimeStdioSandbox{
		result: `{"schema":"coze.mcp_stdio_result.v1","content":"ok"}`,
	}
	transport := NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{
			Policy:  policy,
			Sandbox: sandbox,
		},
	)
	call := validADKMCPRuntimeStdioCall()
	call.Server.Config = `{
		"command":"npx",
		"args":["-y","@example/secret-mcp-server"],
		"cwd":"/secret/workdir",
		"env":{"API_TOKEN":"stale-public-token","PUBLIC_MODE":"safe"},
		"auth_env":{
			"API_TOKEN":"token",
			"CLIENT_SECRET":"oauth.client_secret"
		}
	}`
	call.Server.Auth = `{
		"token":"auth-secret-token",
		"oauth":{"client_secret":"nested-secret-value"}
	}`

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		call,
	)

	require.NoError(t, err)
	require.Equal(t, sandbox.result, result)
	require.Equal(t, 1, policy.calls)
	require.Equal(t, 1, sandbox.calls)
	require.Equal(t, "auth-secret-token", sandbox.call.Config.Env["API_TOKEN"])
	require.Equal(t, "nested-secret-value", sandbox.call.Config.Env["CLIENT_SECRET"])
	require.Equal(t, "safe", sandbox.call.Config.Env["PUBLIC_MODE"])
	require.Equal(t, sandbox.call, policy.call)
}

func TestADKMCPRuntimeStdioTransportRejectsInvalidAuthEnvSafely(
	t *testing.T,
) {
	tests := []struct {
		name   string
		config string
		auth   string
	}{
		{
			name: "invalid auth env shape",
			config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":"stdio-secret-token"},
				"auth_env":{"API_TOKEN":7}
			}`,
			auth: `{"token":"auth-secret-token"}`,
		},
		{
			name: "invalid env name",
			config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":"stdio-secret-token"},
				"auth_env":{"BAD-NAME":"token"}
			}`,
			auth: `{"token":"auth-secret-token"}`,
		},
		{
			name: "invalid auth json",
			config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":"stdio-secret-token"},
				"auth_env":{"API_TOKEN":"token"}
			}`,
			auth: `{"token":`,
		},
		{
			name: "missing auth field",
			config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":"stdio-secret-token"},
				"auth_env":{"API_TOKEN":"missing.secret"}
			}`,
			auth: `{"token":"auth-secret-token"}`,
		},
		{
			name: "non string auth field",
			config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":"stdio-secret-token"},
				"auth_env":{"API_TOKEN":"token"}
			}`,
			auth: `{"token":123}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &recordingADKMCPRuntimeStdioPolicy{}
			sandbox := &recordingADKMCPRuntimeStdioSandbox{}
			transport := NewADKMCPRuntimeStdioTransport(
				ADKMCPRuntimeStdioTransportOptions{
					Policy:  policy,
					Sandbox: sandbox,
				},
			)
			call := validADKMCPRuntimeStdioCall()
			call.Server.Config = tt.config
			call.Server.Auth = tt.auth

			result, err := transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				call,
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Zero(t, policy.calls)
			require.Zero(t, sandbox.calls)
			assertADKMCPStdioErrorDoesNotLeak(t, err.Error())
		})
	}
}

func TestADKMCPRuntimeStdioTransportRejectsInvalidConfigSafely(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{
			name:   "invalid json",
			config: `{"command":`,
		},
		{
			name: "missing command",
			config: `{
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":"stdio-secret-token"}
			}`,
		},
		{
			name: "non string arg",
			config: `{
				"command":"npx",
				"args":["-y",7],
				"env":{"API_TOKEN":"stdio-secret-token"}
			}`,
		},
		{
			name: "non string env",
			config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"env":{"API_TOKEN":7}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &recordingADKMCPRuntimeStdioPolicy{}
			sandbox := &recordingADKMCPRuntimeStdioSandbox{}
			transport := NewADKMCPRuntimeStdioTransport(
				ADKMCPRuntimeStdioTransportOptions{
					Policy:  policy,
					Sandbox: sandbox,
				},
			)
			call := validADKMCPRuntimeStdioCall()
			call.Server.Config = tt.config

			result, err := transport.InvokeADKMCPRuntimeTransport(
				context.Background(),
				call,
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), "mcp runtime stdio config is invalid")
			require.Zero(t, policy.calls)
			require.Zero(t, sandbox.calls)
			assertADKMCPStdioErrorDoesNotLeak(t, err.Error())
		})
	}
}

func TestADKMCPRuntimeStdioTransportSanitizesPolicyAndSandboxErrors(
	t *testing.T,
) {
	t.Run("policy", func(t *testing.T) {
		policy := &recordingADKMCPRuntimeStdioPolicy{
			err: fmt.Errorf(
				`deny npx @example/secret-mcp-server with token stdio-secret-token`,
			),
		}
		sandbox := &recordingADKMCPRuntimeStdioSandbox{}
		transport := NewADKMCPRuntimeStdioTransport(
			ADKMCPRuntimeStdioTransportOptions{
				Policy:  policy,
				Sandbox: sandbox,
			},
		)

		result, err := transport.InvokeADKMCPRuntimeTransport(
			context.Background(),
			validADKMCPRuntimeStdioCall(),
		)

		require.Error(t, err)
		require.Empty(t, result)
		require.Contains(t, err.Error(), "mcp runtime stdio policy denied")
		require.Equal(t, 1, policy.calls)
		require.Zero(t, sandbox.calls)
		assertADKMCPStdioErrorDoesNotLeak(t, err.Error())
	})

	t.Run("sandbox", func(t *testing.T) {
		policy := &recordingADKMCPRuntimeStdioPolicy{}
		sandbox := &recordingADKMCPRuntimeStdioSandbox{
			err: fmt.Errorf(
				`spawn npx failed for /secret/workdir with stdio-secret-token`,
			),
		}
		transport := NewADKMCPRuntimeStdioTransport(
			ADKMCPRuntimeStdioTransportOptions{
				Policy:  policy,
				Sandbox: sandbox,
			},
		)

		result, err := transport.InvokeADKMCPRuntimeTransport(
			context.Background(),
			validADKMCPRuntimeStdioCall(),
		)

		require.Error(t, err)
		require.Empty(t, result)
		require.Contains(t, err.Error(), "mcp runtime stdio transport failed")
		require.Equal(t, 1, policy.calls)
		require.Equal(t, 1, sandbox.calls)
		assertADKMCPStdioErrorDoesNotLeak(t, err.Error())
	})
}

func validADKMCPRuntimeStdioCall() ADKMCPRuntimeTransportCall {
	return ADKMCPRuntimeTransportCall{
		Run:       &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		Name:      "mcp_100_search_docs",
		ToolName:  "search-docs",
		Arguments: `{"query":"secret customer path"}`,
		Server: &toolapi.MCPToolServer{
			ServerID:   100,
			SpaceID:    30,
			Name:       "docs-mcp-secret",
			ServerType: "stdio",
			Config: `{
				"command":"npx",
				"args":["-y","@example/secret-mcp-server"],
				"cwd":"/secret/workdir",
				"env":{"API_TOKEN":"stdio-secret-token"}
			}`,
			Auth: `{"token":"auth-secret"}`,
		},
	}
}

type recordingADKMCPRuntimeStdioPolicy struct {
	call  ADKMCPRuntimeStdioSandboxCall
	calls int
	err   error
}

func (p *recordingADKMCPRuntimeStdioPolicy) ValidateADKMCPRuntimeStdio(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) error {
	p.calls++
	p.call = call

	return p.err
}

type recordingADKMCPRuntimeStdioSandbox struct {
	call   ADKMCPRuntimeStdioSandboxCall
	calls  int
	result string
	err    error
}

func (s *recordingADKMCPRuntimeStdioSandbox) InvokeADKMCPRuntimeStdio(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) (string, error) {
	s.calls++
	s.call = call
	if s.err != nil {
		return "", s.err
	}

	return s.result, nil
}

func assertADKMCPStdioErrorDoesNotLeak(t *testing.T, text string) {
	t.Helper()
	assertADKMCPTransportErrorDoesNotLeak(t, text)
	require.NotContains(t, text, "npx")
	require.NotContains(t, text, "@example/secret-mcp-server")
	require.NotContains(t, text, "/secret/workdir")
	require.NotContains(t, text, "stdio-secret-token")
	require.NotContains(t, text, "auth-secret")
	require.NotContains(t, text, "nested-secret-value")
	require.NotContains(t, text, "missing.secret")
	require.NotContains(t, text, "BAD-NAME")
}

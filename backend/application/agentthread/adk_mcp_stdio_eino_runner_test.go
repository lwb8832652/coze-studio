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
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioEinoRunnerInvokesTargetTool(t *testing.T) {
	root := t.TempDir()
	factory := &recordingADKMCPRuntimeStdioEinoClientFactory{
		client: &recordingADKMCPRuntimeStdioEinoClient{},
	}
	provider := &recordingADKMCPRuntimeStdioEinoToolProvider{
		tools: []tool.BaseTool{
			&recordingADKMCPRuntimeStdioEinoTool{name: "other"},
			&recordingADKMCPRuntimeStdioEinoTool{
				name:   "search-docs",
				result: `{"content":[{"type":"text","text":"ok"}]}`,
			},
		},
	}
	runner := NewADKMCPRuntimeStdioEinoRunner(
		ADKMCPRuntimeStdioEinoRunnerOptions{
			ClientFactory:  factory,
			ToolProvider:   provider,
			MaxOutputBytes: 1024,
		},
	)

	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	result, err := runner.RunADKMCPRuntimeStdio(context.Background(), execution)

	require.NoError(t, err)
	require.JSONEq(t, `{"content":[{"type":"text","text":"ok"}]}`, result)
	require.Equal(t, execution, factory.execution)
	require.Same(t, factory.client, provider.client)
	require.Equal(t, "search-docs", provider.toolName)
	target := provider.tools[1].(*recordingADKMCPRuntimeStdioEinoTool)
	require.Equal(t, execution.Arguments, target.arguments)
	require.Equal(t, 1, factory.client.closeCalls)
}

func TestADKMCPRuntimeStdioEinoRunnerFailsClosedWithSanitizedErrors(
	t *testing.T,
) {
	root := t.TempDir()
	secret := "stdio-secret-token"

	tests := []struct {
		name        string
		runner      *ADKMCPRuntimeStdioEinoRunner
		execution   ADKMCPRuntimeStdioSandboxExecution
		errContains string
	}{
		{
			name: "invalid execution",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						client: &recordingADKMCPRuntimeStdioEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{},
				},
			),
			execution:   ADKMCPRuntimeStdioSandboxExecution{},
			errContains: "mcp runtime stdio eino execution is invalid",
		},
		{
			name: "missing client factory",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{},
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino client factory is not configured",
		},
		{
			name: "client failure",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						err: errors.New("spawn failed " + secret),
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{},
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino client failed",
		},
		{
			name: "tool discovery failure",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						client: &recordingADKMCPRuntimeStdioEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{
						err: errors.New("list failed " + secret),
					},
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino tool discovery failed",
		},
		{
			name: "tool not found",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						client: &recordingADKMCPRuntimeStdioEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{
						tools: []tool.BaseTool{
							&recordingADKMCPRuntimeStdioEinoTool{name: "other"},
						},
					},
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino tool not found",
		},
		{
			name: "tool is not invokable",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						client: &recordingADKMCPRuntimeStdioEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{
						tools: []tool.BaseTool{
							&recordingADKMCPRuntimeStdioEinoMetadataOnlyTool{
								name: "search-docs",
							},
						},
					},
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino tool is not invokable",
		},
		{
			name: "tool call failure",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						client: &recordingADKMCPRuntimeStdioEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{
						tools: []tool.BaseTool{
							&recordingADKMCPRuntimeStdioEinoTool{
								name: "search-docs",
								err:  errors.New("call failed " + secret),
							},
						},
					},
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino tool call failed",
		},
		{
			name: "output budget exceeded",
			runner: NewADKMCPRuntimeStdioEinoRunner(
				ADKMCPRuntimeStdioEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeStdioEinoClientFactory{
						client: &recordingADKMCPRuntimeStdioEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeStdioEinoToolProvider{
						tools: []tool.BaseTool{
							&recordingADKMCPRuntimeStdioEinoTool{
								name:   "search-docs",
								result: "oversized output",
							},
						},
					},
					MaxOutputBytes: 4,
				},
			),
			execution:   validADKMCPRuntimeStdioSandboxExecution(root),
			errContains: "mcp runtime stdio eino output exceeds budget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.runner.RunADKMCPRuntimeStdio(
				context.Background(),
				tt.execution,
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), tt.errContains)
			require.NotContains(t, err.Error(), secret)
			require.NotContains(t, err.Error(), root)
		})
	}
}

type recordingADKMCPRuntimeStdioEinoClient struct {
	closeCalls int
}

func (c *recordingADKMCPRuntimeStdioEinoClient) Close() error {
	c.closeCalls++

	return nil
}

type recordingADKMCPRuntimeStdioEinoClientFactory struct {
	client    *recordingADKMCPRuntimeStdioEinoClient
	execution ADKMCPRuntimeStdioSandboxExecution
	err       error
}

func (f *recordingADKMCPRuntimeStdioEinoClientFactory) NewADKMCPRuntimeStdioEinoClient(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioEinoClient, error) {
	f.execution = execution
	if f.err != nil {
		return nil, f.err
	}
	if f.client == nil {
		f.client = &recordingADKMCPRuntimeStdioEinoClient{}
	}

	return f.client, nil
}

type recordingADKMCPRuntimeStdioEinoToolProvider struct {
	client   ADKMCPRuntimeStdioEinoClient
	toolName string
	tools    []tool.BaseTool
	err      error
}

func (p *recordingADKMCPRuntimeStdioEinoToolProvider) ADKMCPRuntimeStdioEinoTools(
	ctx context.Context,
	client ADKMCPRuntimeStdioEinoClient,
	toolName string,
) ([]tool.BaseTool, error) {
	p.client = client
	p.toolName = toolName
	if p.err != nil {
		return nil, p.err
	}

	return p.tools, nil
}

type recordingADKMCPRuntimeStdioEinoTool struct {
	name      string
	result    string
	arguments string
	err       error
}

func (t *recordingADKMCPRuntimeStdioEinoTool) Info(
	ctx context.Context,
) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: "test tool",
	}, nil
}

func (t *recordingADKMCPRuntimeStdioEinoTool) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	opts ...tool.Option,
) (string, error) {
	t.arguments = argumentsInJSON
	if t.err != nil {
		return "", t.err
	}

	return t.result, nil
}

type recordingADKMCPRuntimeStdioEinoMetadataOnlyTool struct {
	name string
}

func (t *recordingADKMCPRuntimeStdioEinoMetadataOnlyTool) Info(
	ctx context.Context,
) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: "metadata only",
	}, nil
}

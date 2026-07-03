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
	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeRemoteEinoRunnerInvokesTargetTool(t *testing.T) {
	factory := &recordingADKMCPRuntimeRemoteEinoClientFactory{
		client: &recordingADKMCPRuntimeRemoteEinoClient{},
	}
	provider := &recordingADKMCPRuntimeRemoteEinoToolProvider{
		tools: []tool.BaseTool{
			&recordingADKMCPRuntimeStdioEinoTool{name: "other"},
			&recordingADKMCPRuntimeStdioEinoTool{
				name:   "search-docs",
				result: `{"content":[{"type":"text","text":"ok"}]}`,
			},
		},
	}
	runner := NewADKMCPRuntimeRemoteEinoRunner(
		ADKMCPRuntimeRemoteEinoRunnerOptions{
			ClientFactory:  factory,
			ToolProvider:   provider,
			MaxOutputBytes: 1024,
		},
	)

	execution := validADKMCPRuntimeRemoteExecutionForTest()
	result, err := runner.RunADKMCPRuntimeRemote(context.Background(), execution)

	require.NoError(t, err)
	require.JSONEq(t, `{"content":[{"type":"text","text":"ok"}]}`, result)
	require.Equal(t, execution, factory.execution)
	require.Same(t, factory.client, provider.client)
	require.Equal(t, "search-docs", provider.toolName)
	target := provider.tools[1].(*recordingADKMCPRuntimeStdioEinoTool)
	require.Equal(t, execution.Arguments, target.arguments)
	require.Equal(t, 1, factory.client.closeCalls)
}

func TestADKMCPRuntimeRemoteEinoRunnerFailsClosedWithSanitizedErrors(
	t *testing.T,
) {
	secret := "remote-secret-token"
	url := "https://mcp.example.test/mcp"

	tests := []struct {
		name        string
		runner      *ADKMCPRuntimeRemoteEinoRunner
		execution   ADKMCPRuntimeRemoteExecution
		errContains string
	}{
		{
			name: "invalid execution",
			runner: NewADKMCPRuntimeRemoteEinoRunner(
				ADKMCPRuntimeRemoteEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeRemoteEinoClientFactory{
						client: &recordingADKMCPRuntimeRemoteEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeRemoteEinoToolProvider{},
				},
			),
			execution:   ADKMCPRuntimeRemoteExecution{},
			errContains: "mcp runtime remote eino execution is invalid",
		},
		{
			name: "client failure",
			runner: NewADKMCPRuntimeRemoteEinoRunner(
				ADKMCPRuntimeRemoteEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeRemoteEinoClientFactory{
						err: errors.New("connect " + url + " with " + secret),
					},
					ToolProvider: &recordingADKMCPRuntimeRemoteEinoToolProvider{},
				},
			),
			execution:   validADKMCPRuntimeRemoteExecutionForTest(),
			errContains: "mcp runtime remote eino client failed",
		},
		{
			name: "tool discovery failure",
			runner: NewADKMCPRuntimeRemoteEinoRunner(
				ADKMCPRuntimeRemoteEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeRemoteEinoClientFactory{
						client: &recordingADKMCPRuntimeRemoteEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeRemoteEinoToolProvider{
						err: errors.New("list " + url + " with " + secret),
					},
				},
			),
			execution:   validADKMCPRuntimeRemoteExecutionForTest(),
			errContains: "mcp runtime remote eino tool discovery failed",
		},
		{
			name: "output budget exceeded",
			runner: NewADKMCPRuntimeRemoteEinoRunner(
				ADKMCPRuntimeRemoteEinoRunnerOptions{
					ClientFactory: &recordingADKMCPRuntimeRemoteEinoClientFactory{
						client: &recordingADKMCPRuntimeRemoteEinoClient{},
					},
					ToolProvider: &recordingADKMCPRuntimeRemoteEinoToolProvider{
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
			execution:   validADKMCPRuntimeRemoteExecutionForTest(),
			errContains: "mcp runtime remote eino output exceeds budget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.runner.RunADKMCPRuntimeRemote(
				context.Background(),
				tt.execution,
			)

			require.Error(t, err)
			require.Empty(t, result)
			require.Contains(t, err.Error(), tt.errContains)
			require.NotContains(t, err.Error(), secret)
			require.NotContains(t, err.Error(), url)
		})
	}
}

func validADKMCPRuntimeRemoteExecutionForTest() ADKMCPRuntimeRemoteExecution {
	return ADKMCPRuntimeRemoteExecution{
		Run:           &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		Name:          "mcp_100_search_docs",
		ServerID:      100,
		ToolName:      "search-docs",
		Arguments:     `{"query":"coze"}`,
		TransportType: adkMCPRuntimeTransportStreamableHTTP,
		URL:           "https://mcp.example.test/mcp",
		Headers:       map[string]string{"Authorization": "Bearer remote-secret-token"},
	}
}

type recordingADKMCPRuntimeRemoteEinoClient struct {
	closeCalls int
}

func (c *recordingADKMCPRuntimeRemoteEinoClient) Close() error {
	c.closeCalls++

	return nil
}

type recordingADKMCPRuntimeRemoteEinoClientFactory struct {
	client    *recordingADKMCPRuntimeRemoteEinoClient
	execution ADKMCPRuntimeRemoteExecution
	err       error
}

func (f *recordingADKMCPRuntimeRemoteEinoClientFactory) NewADKMCPRuntimeRemoteEinoClient(
	ctx context.Context,
	execution ADKMCPRuntimeRemoteExecution,
) (ADKMCPRuntimeRemoteEinoClient, error) {
	f.execution = execution
	if f.err != nil {
		return nil, f.err
	}
	if f.client == nil {
		f.client = &recordingADKMCPRuntimeRemoteEinoClient{}
	}

	return f.client, nil
}

type recordingADKMCPRuntimeRemoteEinoToolProvider struct {
	client   ADKMCPRuntimeRemoteEinoClient
	toolName string
	tools    []tool.BaseTool
	err      error
}

func (p *recordingADKMCPRuntimeRemoteEinoToolProvider) ADKMCPRuntimeRemoteEinoTools(
	ctx context.Context,
	client ADKMCPRuntimeRemoteEinoClient,
	toolName string,
) ([]tool.BaseTool, error) {
	p.client = client
	p.toolName = toolName
	if p.err != nil {
		return nil, p.err
	}

	return p.tools, nil
}

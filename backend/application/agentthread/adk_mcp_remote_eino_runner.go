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
	"net/http"
	"strings"
	"time"

	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultADKMCPRuntimeRemoteEinoClientName    = "coze-studio"
	defaultADKMCPRuntimeRemoteEinoClientVersion = "1.0.0"
)

type ADKMCPRuntimeRemoteEinoClient interface {
	Close() error
}

type ADKMCPRuntimeRemoteEinoClientFactory interface {
	NewADKMCPRuntimeRemoteEinoClient(
		ctx context.Context,
		execution ADKMCPRuntimeRemoteExecution,
	) (ADKMCPRuntimeRemoteEinoClient, error)
}

type ADKMCPRuntimeRemoteEinoToolProvider interface {
	ADKMCPRuntimeRemoteEinoTools(
		ctx context.Context,
		client ADKMCPRuntimeRemoteEinoClient,
		toolName string,
	) ([]tool.BaseTool, error)
}

type ADKMCPRuntimeRemoteEinoRunnerOptions struct {
	ClientFactory  ADKMCPRuntimeRemoteEinoClientFactory
	ToolProvider   ADKMCPRuntimeRemoteEinoToolProvider
	MaxOutputBytes int
	Timeout        time.Duration
}

type ADKMCPRuntimeRemoteEinoRunner struct {
	clientFactory  ADKMCPRuntimeRemoteEinoClientFactory
	toolProvider   ADKMCPRuntimeRemoteEinoToolProvider
	maxOutputBytes int
	timeout        time.Duration
}

type ADKMCPRuntimeRemoteEinoMCPClientFactoryOptions struct {
	ClientName    string
	ClientVersion string
	Timeout       time.Duration
}

type ADKMCPRuntimeRemoteEinoMCPClientFactory struct {
	clientName    string
	clientVersion string
	timeout       time.Duration
}

type ADKMCPRuntimeRemoteEinoMCPToolProvider struct{}

func NewADKMCPRuntimeRemoteEinoRunner(
	options ADKMCPRuntimeRemoteEinoRunnerOptions,
) *ADKMCPRuntimeRemoteEinoRunner {
	maxOutputBytes := options.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultADKMCPRuntimeExecutorMaxOutputBytes
	}

	return &ADKMCPRuntimeRemoteEinoRunner{
		clientFactory:  options.ClientFactory,
		toolProvider:   options.ToolProvider,
		maxOutputBytes: maxOutputBytes,
		timeout:        options.Timeout,
	}
}

func (r *ADKMCPRuntimeRemoteEinoRunner) RunADKMCPRuntimeRemote(
	ctx context.Context,
	execution ADKMCPRuntimeRemoteExecution,
) (string, error) {
	if !validADKMCPRuntimeRemoteExecution(execution) {
		return "", errors.New("mcp runtime remote eino execution is invalid")
	}
	if r == nil || r.clientFactory == nil {
		return "", errors.New("mcp runtime remote eino client factory is not configured")
	}
	if r.toolProvider == nil {
		return "", errors.New("mcp runtime remote eino tool provider is not configured")
	}

	client, err := r.clientFactory.NewADKMCPRuntimeRemoteEinoClient(
		ctx,
		execution,
	)
	if err != nil || client == nil {
		return "", errors.New("mcp runtime remote eino client failed")
	}
	defer func() { _ = client.Close() }()

	tools, err := r.toolProvider.ADKMCPRuntimeRemoteEinoTools(
		ctx,
		client,
		strings.TrimSpace(execution.ToolName),
	)
	if err != nil {
		return "", errors.New("mcp runtime remote eino tool discovery failed")
	}

	target, found, err := findADKMCPRuntimeStdioEinoTool(
		ctx,
		tools,
		strings.TrimSpace(execution.ToolName),
	)
	if err != nil {
		return "", errors.New("mcp runtime remote eino tool discovery failed")
	}
	if !found {
		return "", errors.New("mcp runtime remote eino tool not found")
	}
	invokable, ok := target.(tool.InvokableTool)
	if !ok {
		return "", errors.New("mcp runtime remote eino tool is not invokable")
	}

	result, err := invokable.InvokableRun(ctx, strings.TrimSpace(execution.Arguments))
	if err != nil {
		return "", errors.New("mcp runtime remote eino tool call failed")
	}
	if len([]byte(result)) > r.outputByteLimit() {
		return "", errors.New("mcp runtime remote eino output exceeds budget")
	}

	return result, nil
}

func NewADKMCPRuntimeRemoteEinoMCPClientFactory(
	options ADKMCPRuntimeRemoteEinoMCPClientFactoryOptions,
) *ADKMCPRuntimeRemoteEinoMCPClientFactory {
	clientName := strings.TrimSpace(options.ClientName)
	if clientName == "" {
		clientName = defaultADKMCPRuntimeRemoteEinoClientName
	}
	clientVersion := strings.TrimSpace(options.ClientVersion)
	if clientVersion == "" {
		clientVersion = defaultADKMCPRuntimeRemoteEinoClientVersion
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultADKMCPRuntimeExecutorTimeout
	}

	return &ADKMCPRuntimeRemoteEinoMCPClientFactory{
		clientName:    clientName,
		clientVersion: clientVersion,
		timeout:       timeout,
	}
}

func (f *ADKMCPRuntimeRemoteEinoMCPClientFactory) NewADKMCPRuntimeRemoteEinoClient(
	ctx context.Context,
	execution ADKMCPRuntimeRemoteExecution,
) (ADKMCPRuntimeRemoteEinoClient, error) {
	if !validADKMCPRuntimeRemoteExecution(execution) {
		return nil, errors.New("mcp runtime remote eino execution is invalid")
	}

	transport, err := f.newTransport(execution)
	if err != nil {
		return nil, err
	}
	client := mcpclient.NewClient(transport)
	if err := client.Start(ctx); err != nil {
		return nil, err
	}

	initRequest := mcpsdk.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcpsdk.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcpsdk.Implementation{
		Name:    f.clientNameOrDefault(),
		Version: f.clientVersionOrDefault(),
	}
	if _, err := client.Initialize(ctx, initRequest); err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

func (p *ADKMCPRuntimeRemoteEinoMCPToolProvider) ADKMCPRuntimeRemoteEinoTools(
	ctx context.Context,
	client ADKMCPRuntimeRemoteEinoClient,
	toolName string,
) ([]tool.BaseTool, error) {
	mcpClient, ok := client.(mcpclient.MCPClient)
	if !ok {
		return nil, errors.New("mcp runtime remote eino client is invalid")
	}

	return einomcp.GetTools(
		ctx,
		&einomcp.Config{
			Cli:          mcpClient,
			ToolNameList: []string{strings.TrimSpace(toolName)},
		},
	)
}

func (r *ADKMCPRuntimeRemoteEinoRunner) outputByteLimit() int {
	if r == nil || r.maxOutputBytes <= 0 {
		return defaultADKMCPRuntimeExecutorMaxOutputBytes
	}

	return r.maxOutputBytes
}

func (f *ADKMCPRuntimeRemoteEinoMCPClientFactory) newTransport(
	execution ADKMCPRuntimeRemoteExecution,
) (mcptransport.Interface, error) {
	headers := cloneADKMCPRuntimeStringMap(execution.Headers)
	httpClient := &http.Client{Timeout: f.timeoutOrDefault()}
	switch execution.TransportType {
	case adkMCPRuntimeTransportSSE:
		return mcptransport.NewSSE(
			strings.TrimSpace(execution.URL),
			mcptransport.WithHeaderFunc(
				func(context.Context) map[string]string {
					return cloneADKMCPRuntimeStringMap(headers)
				},
			),
			mcptransport.WithHTTPClient(httpClient),
		)
	case adkMCPRuntimeTransportStreamableHTTP:
		return mcptransport.NewStreamableHTTP(
			strings.TrimSpace(execution.URL),
			mcptransport.WithHTTPHeaders(headers),
			mcptransport.WithHTTPBasicClient(httpClient),
			mcptransport.WithHTTPTimeout(f.timeoutOrDefault()),
		)
	default:
		return nil, errors.New("mcp runtime remote eino transport is unsupported")
	}
}

func (f *ADKMCPRuntimeRemoteEinoMCPClientFactory) clientNameOrDefault() string {
	if f == nil || strings.TrimSpace(f.clientName) == "" {
		return defaultADKMCPRuntimeRemoteEinoClientName
	}

	return strings.TrimSpace(f.clientName)
}

func (f *ADKMCPRuntimeRemoteEinoMCPClientFactory) clientVersionOrDefault() string {
	if f == nil || strings.TrimSpace(f.clientVersion) == "" {
		return defaultADKMCPRuntimeRemoteEinoClientVersion
	}

	return strings.TrimSpace(f.clientVersion)
}

func (f *ADKMCPRuntimeRemoteEinoMCPClientFactory) timeoutOrDefault() time.Duration {
	if f == nil || f.timeout <= 0 {
		return defaultADKMCPRuntimeExecutorTimeout
	}

	return f.timeout
}

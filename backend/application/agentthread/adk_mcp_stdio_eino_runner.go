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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultADKMCPRuntimeStdioEinoClientName    = "coze-studio"
	defaultADKMCPRuntimeStdioEinoClientVersion = "1.0.0"
)

type ADKMCPRuntimeStdioEinoClient interface {
	Close() error
}

type ADKMCPRuntimeStdioEinoClientFactory interface {
	NewADKMCPRuntimeStdioEinoClient(
		ctx context.Context,
		execution ADKMCPRuntimeStdioSandboxExecution,
	) (ADKMCPRuntimeStdioEinoClient, error)
}

type ADKMCPRuntimeStdioEinoToolProvider interface {
	ADKMCPRuntimeStdioEinoTools(
		ctx context.Context,
		client ADKMCPRuntimeStdioEinoClient,
		toolName string,
	) ([]tool.BaseTool, error)
}

type ADKMCPRuntimeStdioEinoRunnerOptions struct {
	ClientFactory  ADKMCPRuntimeStdioEinoClientFactory
	ToolProvider   ADKMCPRuntimeStdioEinoToolProvider
	MaxOutputBytes int
}

type ADKMCPRuntimeStdioEinoRunner struct {
	clientFactory  ADKMCPRuntimeStdioEinoClientFactory
	toolProvider   ADKMCPRuntimeStdioEinoToolProvider
	maxOutputBytes int
}

type ADKMCPRuntimeStdioEinoMCPClientFactoryOptions struct {
	ClientName    string
	ClientVersion string
}

type ADKMCPRuntimeStdioEinoMCPClientFactory struct {
	clientName    string
	clientVersion string
}

type ADKMCPRuntimeStdioEinoMCPToolProvider struct{}

func NewADKMCPRuntimeStdioEinoRunner(
	options ADKMCPRuntimeStdioEinoRunnerOptions,
) *ADKMCPRuntimeStdioEinoRunner {
	maxOutputBytes := options.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultADKMCPRuntimeExecutorMaxOutputBytes
	}

	return &ADKMCPRuntimeStdioEinoRunner{
		clientFactory:  options.ClientFactory,
		toolProvider:   options.ToolProvider,
		maxOutputBytes: maxOutputBytes,
	}
}

func (r *ADKMCPRuntimeStdioEinoRunner) RunADKMCPRuntimeStdio(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (string, error) {
	if !validADKMCPRuntimeStdioEinoExecution(execution) {
		return "", errors.New("mcp runtime stdio eino execution is invalid")
	}
	if r == nil || r.clientFactory == nil {
		return "", errors.New("mcp runtime stdio eino client factory is not configured")
	}
	if r.toolProvider == nil {
		return "", errors.New("mcp runtime stdio eino tool provider is not configured")
	}

	client, err := r.clientFactory.NewADKMCPRuntimeStdioEinoClient(
		ctx,
		execution,
	)
	if err != nil || client == nil {
		return "", errors.New("mcp runtime stdio eino client failed")
	}
	defer func() { _ = client.Close() }()

	tools, err := r.toolProvider.ADKMCPRuntimeStdioEinoTools(
		ctx,
		client,
		strings.TrimSpace(execution.ToolName),
	)
	if err != nil {
		return "", errors.New("mcp runtime stdio eino tool discovery failed")
	}

	target, found, err := findADKMCPRuntimeStdioEinoTool(
		ctx,
		tools,
		strings.TrimSpace(execution.ToolName),
	)
	if err != nil {
		return "", errors.New("mcp runtime stdio eino tool discovery failed")
	}
	if !found {
		return "", errors.New("mcp runtime stdio eino tool not found")
	}
	invokable, ok := target.(tool.InvokableTool)
	if !ok {
		return "", errors.New("mcp runtime stdio eino tool is not invokable")
	}

	result, err := invokable.InvokableRun(ctx, strings.TrimSpace(execution.Arguments))
	if err != nil {
		return "", errors.New("mcp runtime stdio eino tool call failed")
	}
	if len([]byte(result)) > r.outputByteLimit() {
		return "", errors.New("mcp runtime stdio eino output exceeds budget")
	}

	return result, nil
}

func NewADKMCPRuntimeStdioEinoMCPClientFactory(
	options ADKMCPRuntimeStdioEinoMCPClientFactoryOptions,
) *ADKMCPRuntimeStdioEinoMCPClientFactory {
	clientName := strings.TrimSpace(options.ClientName)
	if clientName == "" {
		clientName = defaultADKMCPRuntimeStdioEinoClientName
	}
	clientVersion := strings.TrimSpace(options.ClientVersion)
	if clientVersion == "" {
		clientVersion = defaultADKMCPRuntimeStdioEinoClientVersion
	}

	return &ADKMCPRuntimeStdioEinoMCPClientFactory{
		clientName:    clientName,
		clientVersion: clientVersion,
	}
}

func (f *ADKMCPRuntimeStdioEinoMCPClientFactory) NewADKMCPRuntimeStdioEinoClient(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioEinoClient, error) {
	if !validADKMCPRuntimeStdioEinoExecution(execution) {
		return nil, errors.New("mcp runtime stdio eino execution is invalid")
	}
	env := adkMCPRuntimeStdioEinoEnvList(execution.Env)
	stdio := mcptransport.NewStdioWithOptions(
		strings.TrimSpace(execution.Command),
		env,
		append([]string(nil), execution.Args...),
		mcptransport.WithCommandFunc(
			adkMCPRuntimeStdioEinoCommandFunc(
				strings.TrimSpace(execution.WorkingDir),
			),
		),
	)
	client := mcpclient.NewClient(stdio)
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

func (p *ADKMCPRuntimeStdioEinoMCPToolProvider) ADKMCPRuntimeStdioEinoTools(
	ctx context.Context,
	client ADKMCPRuntimeStdioEinoClient,
	toolName string,
) ([]tool.BaseTool, error) {
	mcpClient, ok := client.(mcpclient.MCPClient)
	if !ok {
		return nil, errors.New("mcp runtime stdio eino client is invalid")
	}

	return einomcp.GetTools(
		ctx,
		&einomcp.Config{
			Cli:          mcpClient,
			ToolNameList: []string{strings.TrimSpace(toolName)},
		},
	)
}

func (r *ADKMCPRuntimeStdioEinoRunner) outputByteLimit() int {
	if r == nil || r.maxOutputBytes <= 0 {
		return defaultADKMCPRuntimeExecutorMaxOutputBytes
	}

	return r.maxOutputBytes
}

func (f *ADKMCPRuntimeStdioEinoMCPClientFactory) clientNameOrDefault() string {
	if f == nil || strings.TrimSpace(f.clientName) == "" {
		return defaultADKMCPRuntimeStdioEinoClientName
	}

	return strings.TrimSpace(f.clientName)
}

func (f *ADKMCPRuntimeStdioEinoMCPClientFactory) clientVersionOrDefault() string {
	if f == nil || strings.TrimSpace(f.clientVersion) == "" {
		return defaultADKMCPRuntimeStdioEinoClientVersion
	}

	return strings.TrimSpace(f.clientVersion)
}

func findADKMCPRuntimeStdioEinoTool(
	ctx context.Context,
	tools []tool.BaseTool,
	toolName string,
) (tool.BaseTool, bool, error) {
	toolName = strings.TrimSpace(toolName)
	for _, item := range tools {
		if item == nil {
			continue
		}
		info, err := item.Info(ctx)
		if err != nil {
			return nil, false, err
		}
		if info != nil && strings.TrimSpace(info.Name) == toolName {
			return item, true, nil
		}
	}

	return nil, false, nil
}

func validADKMCPRuntimeStdioEinoExecution(
	execution ADKMCPRuntimeStdioSandboxExecution,
) bool {
	return execution.Run != nil &&
		execution.Run.RunID > 0 &&
		execution.Run.ThreadID > 0 &&
		execution.Run.SpaceID > 0 &&
		execution.ServerID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(execution.Name)) &&
		strings.TrimSpace(execution.ToolName) != "" &&
		validADKMCPRuntimeArguments(strings.TrimSpace(execution.Arguments)) &&
		strings.TrimSpace(execution.Command) != "" &&
		filepath.IsAbs(strings.TrimSpace(execution.WorkingDir))
}

func adkMCPRuntimeStdioEinoEnvList(env map[string]string) []string {
	if len(env) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}

	return result
}

func adkMCPRuntimeStdioEinoCommandFunc(
	workingDir string,
) mcptransport.CommandFunc {
	return func(
		ctx context.Context,
		command string,
		env []string,
		args []string,
	) (*exec.Cmd, error) {
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Env = append(os.Environ(), env...)
		cmd.Dir = workingDir

		return cmd, nil
	}
}

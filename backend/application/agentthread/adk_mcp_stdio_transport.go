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
	"strings"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

const defaultADKMCPRuntimeStdioMaxConfigBytes = 16 << 10

type ADKMCPRuntimeStdioTransportOptions struct {
	Policy         ADKMCPRuntimeStdioPolicy
	Sandbox        ADKMCPRuntimeStdioSandbox
	WorkdirManager ADKMCPRuntimeStdioWorkdirProjector
	MaxConfigBytes int
}

type ADKMCPRuntimeStdioTransport struct {
	policy         ADKMCPRuntimeStdioPolicy
	sandbox        ADKMCPRuntimeStdioSandbox
	workdirManager ADKMCPRuntimeStdioWorkdirProjector
	maxConfigBytes int
}

type ADKMCPRuntimeStdioPolicy interface {
	ValidateADKMCPRuntimeStdio(
		ctx context.Context,
		call ADKMCPRuntimeStdioSandboxCall,
	) error
}

type ADKMCPRuntimeStdioPolicyFunc func(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) error

func (f ADKMCPRuntimeStdioPolicyFunc) ValidateADKMCPRuntimeStdio(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) error {
	if f == nil {
		return errors.New("mcp runtime stdio policy is not configured")
	}
	return f(ctx, call)
}

type ADKMCPRuntimeStdioSandbox interface {
	InvokeADKMCPRuntimeStdio(
		ctx context.Context,
		call ADKMCPRuntimeStdioSandboxCall,
	) (string, error)
}

type ADKMCPRuntimeStdioSandboxFunc func(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) (string, error)

func (f ADKMCPRuntimeStdioSandboxFunc) InvokeADKMCPRuntimeStdio(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) (string, error) {
	if f == nil {
		return "", errors.New("mcp runtime stdio sandbox is not configured")
	}
	return f(ctx, call)
}

type ADKMCPRuntimeStdioSandboxCall struct {
	Run       *RunSummary
	Name      string
	ServerID  int64
	ToolName  string
	Arguments string
	Config    ADKMCPRuntimeStdioConfig
}

type ADKMCPRuntimeStdioConfig struct {
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
}

func NewADKMCPRuntimeStdioTransport(
	options ADKMCPRuntimeStdioTransportOptions,
) *ADKMCPRuntimeStdioTransport {
	maxConfigBytes := options.MaxConfigBytes
	if maxConfigBytes <= 0 {
		maxConfigBytes = defaultADKMCPRuntimeStdioMaxConfigBytes
	}
	return &ADKMCPRuntimeStdioTransport{
		policy:         options.Policy,
		sandbox:        options.Sandbox,
		workdirManager: options.WorkdirManager,
		maxConfigBytes: maxConfigBytes,
	}
}

func (t *ADKMCPRuntimeStdioTransport) InvokeADKMCPRuntimeTransport(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (string, error) {
	config, err := parseADKMCPRuntimeStdioConfig(call, t.configByteLimit())
	if err != nil {
		return "", err
	}
	if t != nil && t.workdirManager != nil {
		config, err = t.projectWorkdir(ctx, call, config)
		if err != nil {
			return "", err
		}
	}
	sandboxCall := ADKMCPRuntimeStdioSandboxCall{
		Run:       call.Run,
		Name:      strings.TrimSpace(call.Name),
		ServerID:  call.Server.ServerID,
		ToolName:  strings.TrimSpace(call.ToolName),
		Arguments: strings.TrimSpace(call.Arguments),
		Config:    config,
	}
	if err := t.validate(ctx, sandboxCall); err != nil {
		return "", err
	}
	if t == nil || t.sandbox == nil {
		return "", errors.New("mcp runtime stdio sandbox is not configured")
	}
	result, err := t.sandbox.InvokeADKMCPRuntimeStdio(ctx, sandboxCall)
	if err != nil {
		return "", errors.New("mcp runtime stdio transport failed")
	}
	return result, nil
}

func (t *ADKMCPRuntimeStdioTransport) configByteLimit() int {
	if t == nil || t.maxConfigBytes <= 0 {
		return defaultADKMCPRuntimeStdioMaxConfigBytes
	}
	return t.maxConfigBytes
}

func (t *ADKMCPRuntimeStdioTransport) projectWorkdir(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
	config ADKMCPRuntimeStdioConfig,
) (ADKMCPRuntimeStdioConfig, error) {
	if call.Server == nil {
		return ADKMCPRuntimeStdioConfig{}, errors.New("mcp runtime stdio workdir is invalid")
	}
	projection, err := t.workdirManager.ProjectADKMCPRuntimeStdioWorkdir(
		ctx,
		ADKMCPRuntimeStdioWorkdirRequest{
			Run:      call.Run,
			Name:     strings.TrimSpace(call.Name),
			ServerID: call.Server.ServerID,
			ToolName: strings.TrimSpace(call.ToolName),
		},
	)
	if err != nil || strings.TrimSpace(projection.WorkingDir) == "" {
		return ADKMCPRuntimeStdioConfig{}, errors.New("mcp runtime stdio workdir is invalid")
	}
	config.WorkingDir = strings.TrimSpace(projection.WorkingDir)
	return config, nil
}

func (t *ADKMCPRuntimeStdioTransport) validate(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) error {
	if t == nil || t.policy == nil {
		return errors.New("mcp runtime stdio policy is not configured")
	}
	if err := t.policy.ValidateADKMCPRuntimeStdio(ctx, call); err != nil {
		return errors.New("mcp runtime stdio policy denied")
	}
	return nil
}

func parseADKMCPRuntimeStdioConfig(
	call ADKMCPRuntimeTransportCall,
	maxConfigBytes int,
) (ADKMCPRuntimeStdioConfig, error) {
	if call.Server == nil {
		return ADKMCPRuntimeStdioConfig{}, errors.New("mcp runtime stdio config is invalid")
	}
	config, err := mcpruntime.ParseStdioConnection(
		mcpruntime.Connection{
			ServerType: call.Server.ServerType,
			Config:     call.Server.Config,
			Auth:       call.Server.Auth,
		},
		maxConfigBytes,
	)
	if err != nil {
		return ADKMCPRuntimeStdioConfig{}, errors.New("mcp runtime stdio config is invalid")
	}
	return ADKMCPRuntimeStdioConfig{
		Command:    config.Command,
		Args:       append([]string(nil), config.Args...),
		Env:        cloneADKMCPRuntimeStringMap(config.Env),
		WorkingDir: config.WorkingDir,
	}, nil
}

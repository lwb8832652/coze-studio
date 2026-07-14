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

const (
	defaultADKMCPRuntimeRemoteMaxConfigBytes = 16 << 10
	defaultADKMCPRuntimeRemoteMaxHeaders     = 16
	defaultADKMCPRuntimeRemoteMaxHeaderBytes = 4096
)

type ADKMCPRuntimeRemoteTransportOptions struct {
	Runner            ADKMCPRuntimeRemoteRunner
	AllowedHosts      []string
	AllowInsecureHTTP bool
	MaxConfigBytes    int
	MaxHeaders        int
	MaxHeaderBytes    int
}

type ADKMCPRuntimeRemoteTransport struct {
	runner            ADKMCPRuntimeRemoteRunner
	allowedHosts      []string
	allowInsecureHTTP bool
	maxConfigBytes    int
	maxHeaders        int
	maxHeaderBytes    int
}

type ADKMCPRuntimeRemoteRunner interface {
	RunADKMCPRuntimeRemote(
		ctx context.Context,
		execution ADKMCPRuntimeRemoteExecution,
	) (string, error)
}

type ADKMCPRuntimeRemoteRunnerFunc func(
	ctx context.Context,
	execution ADKMCPRuntimeRemoteExecution,
) (string, error)

func (f ADKMCPRuntimeRemoteRunnerFunc) RunADKMCPRuntimeRemote(
	ctx context.Context,
	execution ADKMCPRuntimeRemoteExecution,
) (string, error) {
	if f == nil {
		return "", errors.New("mcp runtime remote runner is not configured")
	}
	return f(ctx, execution)
}

type ADKMCPRuntimeRemoteExecution struct {
	Run             *RunSummary
	Name            string
	ServerID        int64
	ToolName        string
	Arguments       string
	TransportType   string
	URL             string
	Headers         map[string]string
	AllowedHosts    []string
	AllowLocalDebug bool
}

type adkMCPRuntimeRemoteConfig struct {
	TransportType string
	URL           string
	Headers       map[string]string
}

func NewADKMCPRuntimeRemoteTransport(
	options ADKMCPRuntimeRemoteTransportOptions,
) *ADKMCPRuntimeRemoteTransport {
	maxConfigBytes := options.MaxConfigBytes
	if maxConfigBytes <= 0 {
		maxConfigBytes = defaultADKMCPRuntimeRemoteMaxConfigBytes
	}
	maxHeaders := options.MaxHeaders
	if maxHeaders < 0 {
		maxHeaders = 0
	}
	if maxHeaders == 0 {
		maxHeaders = defaultADKMCPRuntimeRemoteMaxHeaders
	}
	maxHeaderBytes := options.MaxHeaderBytes
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = defaultADKMCPRuntimeRemoteMaxHeaderBytes
	}
	return &ADKMCPRuntimeRemoteTransport{
		runner:            options.Runner,
		allowedHosts:      append([]string(nil), options.AllowedHosts...),
		allowInsecureHTTP: options.AllowInsecureHTTP,
		maxConfigBytes:    maxConfigBytes,
		maxHeaders:        maxHeaders,
		maxHeaderBytes:    maxHeaderBytes,
	}
}

func (t *ADKMCPRuntimeRemoteTransport) InvokeADKMCPRuntimeTransport(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (string, error) {
	config, err := t.parseConfig(call)
	if err != nil {
		return "", err
	}
	execution := ADKMCPRuntimeRemoteExecution{
		Run:             call.Run,
		Name:            strings.TrimSpace(call.Name),
		ServerID:        call.Server.ServerID,
		ToolName:        strings.TrimSpace(call.ToolName),
		Arguments:       strings.TrimSpace(call.Arguments),
		TransportType:   config.TransportType,
		URL:             config.URL,
		Headers:         cloneADKMCPRuntimeStringMap(config.Headers),
		AllowedHosts:    append([]string(nil), t.allowedHosts...),
		AllowLocalDebug: t.allowInsecureHTTP,
	}
	if !validADKMCPRuntimeRemoteExecution(execution) {
		return "", errors.New("mcp runtime remote config is invalid")
	}
	if t == nil || t.runner == nil {
		return "", errors.New("mcp runtime remote runner is not configured")
	}
	result, err := t.runner.RunADKMCPRuntimeRemote(ctx, execution)
	if err != nil {
		return "", errors.New("mcp runtime remote transport failed")
	}
	return result, nil
}

func (t *ADKMCPRuntimeRemoteTransport) parseConfig(
	call ADKMCPRuntimeTransportCall,
) (adkMCPRuntimeRemoteConfig, error) {
	if t == nil || call.Server == nil {
		return adkMCPRuntimeRemoteConfig{}, errors.New("mcp runtime remote config is invalid")
	}
	resolved, err := (mcpruntime.Policy{
		RemoteEnabled:        true,
		RemoteAllowedHosts:   append([]string(nil), t.allowedHosts...),
		AllowInsecureHTTP:    t.allowInsecureHTTP,
		RemoteMaxConfigBytes: t.maxConfigBytes,
		RemoteMaxHeaders:     t.maxHeaders,
		RemoteMaxHeaderBytes: t.maxHeaderBytes,
	}).ParseRemote(mcpruntime.Connection{
		ServerType: call.Server.ServerType,
		Config:     call.Server.Config,
		Auth:       call.Server.Auth,
	})
	if err != nil {
		return adkMCPRuntimeRemoteConfig{}, errors.New("mcp runtime remote config is invalid")
	}
	return adkMCPRuntimeRemoteConfig{
		TransportType: resolved.ServerType,
		URL:           resolved.URL,
		Headers:       cloneADKMCPRuntimeStringMap(resolved.Headers),
	}, nil
}

func validADKMCPRuntimeRemoteExecution(
	execution ADKMCPRuntimeRemoteExecution,
) bool {
	return execution.Run != nil &&
		execution.Run.RunID > 0 &&
		execution.Run.ThreadID > 0 &&
		execution.Run.SpaceID > 0 &&
		execution.ServerID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(execution.Name)) &&
		strings.TrimSpace(execution.ToolName) != "" &&
		validADKMCPRuntimeArguments(strings.TrimSpace(execution.Arguments)) &&
		(execution.TransportType == adkMCPRuntimeTransportSSE ||
			execution.TransportType == adkMCPRuntimeTransportStreamableHTTP) &&
		strings.TrimSpace(execution.URL) != ""
}

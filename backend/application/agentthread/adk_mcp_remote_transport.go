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
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
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
	allowedHosts      map[string]struct{}
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
	Run           *RunSummary
	Name          string
	ServerID      int64
	ToolName      string
	Arguments     string
	TransportType string
	URL           string
	Headers       map[string]string
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
		allowedHosts:      adkMCPRuntimeRemoteHostSet(options.AllowedHosts),
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
		Run:           call.Run,
		Name:          strings.TrimSpace(call.Name),
		ServerID:      call.Server.ServerID,
		ToolName:      strings.TrimSpace(call.ToolName),
		Arguments:     strings.TrimSpace(call.Arguments),
		TransportType: config.TransportType,
		URL:           config.URL,
		Headers:       cloneADKMCPRuntimeStringMap(config.Headers),
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
	if call.Server == nil {
		return adkMCPRuntimeRemoteConfig{},
			errors.New("mcp runtime remote config is invalid")
	}
	transportType := normalizeADKMCPRuntimeTransportType(call)
	if transportType != adkMCPRuntimeTransportSSE &&
		transportType != adkMCPRuntimeTransportStreamableHTTP {
		return adkMCPRuntimeRemoteConfig{},
			errors.New("mcp runtime remote config is invalid")
	}
	configJSON := strings.TrimSpace(call.Server.Config)
	if configJSON == "" || len([]byte(configJSON)) > t.configByteLimit() {
		return adkMCPRuntimeRemoteConfig{},
			errors.New("mcp runtime remote config is invalid")
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return adkMCPRuntimeRemoteConfig{},
			errors.New("mcp runtime remote config is invalid")
	}

	remoteURL, err := t.validateURL(firstADKMCPRuntimeStdioString(raw, "url"))
	if err != nil {
		return adkMCPRuntimeRemoteConfig{}, err
	}
	headers, err := parseADKMCPRuntimeStdioStringMap(raw, "headers")
	if err != nil {
		return adkMCPRuntimeRemoteConfig{},
			errors.New("mcp runtime remote config is invalid")
	}
	if err := applyADKMCPRuntimeRemoteAuthHeaders(call, raw, headers); err != nil {
		return adkMCPRuntimeRemoteConfig{}, err
	}
	if err := t.validateHeaders(headers); err != nil {
		return adkMCPRuntimeRemoteConfig{}, err
	}

	return adkMCPRuntimeRemoteConfig{
		TransportType: transportType,
		URL:           remoteURL,
		Headers:       headers,
	}, nil
}

func (t *ADKMCPRuntimeRemoteTransport) configByteLimit() int {
	if t == nil || t.maxConfigBytes <= 0 {
		return defaultADKMCPRuntimeRemoteMaxConfigBytes
	}

	return t.maxConfigBytes
}

func (t *ADKMCPRuntimeRemoteTransport) headerCountLimit() int {
	if t == nil || t.maxHeaders <= 0 {
		return defaultADKMCPRuntimeRemoteMaxHeaders
	}

	return t.maxHeaders
}

func (t *ADKMCPRuntimeRemoteTransport) headerByteLimit() int {
	if t == nil || t.maxHeaderBytes <= 0 {
		return defaultADKMCPRuntimeRemoteMaxHeaderBytes
	}

	return t.maxHeaderBytes
}

func (t *ADKMCPRuntimeRemoteTransport) validateURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Host == "" {
		return "", errors.New("mcp runtime remote config is invalid")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("mcp runtime remote config is invalid")
	}
	switch parsed.Scheme {
	case "https":
	case "http":
		if !t.allowInsecureHTTP && !adkMCPRuntimeRemoteLocalHost(parsed.Hostname()) {
			return "", errors.New("mcp runtime remote config is invalid")
		}
	default:
		return "", errors.New("mcp runtime remote config is invalid")
	}
	if !t.hostAllowed(parsed.Host) {
		return "", errors.New("mcp runtime remote config is invalid")
	}

	return parsed.String(), nil
}

func (t *ADKMCPRuntimeRemoteTransport) validateHeaders(
	headers map[string]string,
) error {
	if len(headers) > t.headerCountLimit() {
		return errors.New("mcp runtime remote config is invalid")
	}
	totalBytes := 0
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if !validADKMCPRuntimeRemoteHeaderName(key) {
			return errors.New("mcp runtime remote config is invalid")
		}
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("mcp runtime remote config is invalid")
		}
		totalBytes += len([]byte(key)) + len([]byte(value))
		if totalBytes > t.headerByteLimit() {
			return errors.New("mcp runtime remote config is invalid")
		}
	}

	return nil
}

func (t *ADKMCPRuntimeRemoteTransport) hostAllowed(hostport string) bool {
	if t == nil || len(t.allowedHosts) == 0 {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(hostport))
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if _, ok := t.allowedHosts[host]; ok {
		return true
	}
	if _, ok := t.allowedHosts[strings.ToLower(strings.TrimSpace(hostport))]; ok {
		return true
	}

	return false
}

func applyADKMCPRuntimeRemoteAuthHeaders(
	call ADKMCPRuntimeTransportCall,
	rawConfig map[string]any,
	headers map[string]string,
) error {
	authHeaders, err := parseADKMCPRuntimeStdioStringMap(rawConfig, "auth_headers")
	if err != nil {
		return errors.New("mcp runtime remote config is invalid")
	}
	if len(authHeaders) == 0 {
		return nil
	}
	authPayload, err := parseADKMCPRuntimeStdioAuthPayload(call)
	if err != nil {
		return errors.New("mcp runtime remote config is invalid")
	}
	for headerName, authPath := range authHeaders {
		value, err := resolveADKMCPRuntimeStdioAuthString(authPayload, authPath)
		if err != nil {
			return errors.New("mcp runtime remote config is invalid")
		}
		headers[headerName] = value
	}

	return nil
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

func validADKMCPRuntimeRemoteHeaderName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r <= 32 || r >= 127 {
			return false
		}
		if strings.ContainsRune("()<>@,;:\\\"/[]?={}", r) {
			return false
		}
	}

	return true
}

func adkMCPRuntimeRemoteHostSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			set[value] = struct{}{}
		}
	}

	return set
}

func adkMCPRuntimeRemoteLocalHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" ||
		host == "127.0.0.1" ||
		host == "::1" ||
		host == "[::1]"
}

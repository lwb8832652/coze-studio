// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Policy struct {
	RemoteEnabled        bool
	RemoteAllowedHosts   []string
	AllowInsecureHTTP    bool
	RemoteMaxConfigBytes int
	RemoteMaxHeaders     int
	RemoteMaxHeaderBytes int
	HTTPTimeout          time.Duration

	StdioEnabled                   bool
	StdioWorkdirRoot               string
	StdioAllowedCommands           []string
	StdioAllowedNpxPackages        []string
	StdioAllowedUVXPackages        []string
	StdioAllowedNodeScriptRoots    []string
	StdioCommandRules              []StdioCommandRule
	StdioCommandPolicy             *StdioCommandPolicy
	StdioAllowedWorkingDirPrefixes []string
	StdioAllowedEnvKeys            []string
	StdioMaxConfigBytes            int
	StdioMaxArgs                   int
	StdioMaxArgBytes               int
	StdioMaxEnvVars                int
	StdioMaxEnvValueBytes          int
	RejectUserWorkingDir           bool
	RequireWorkingDir              bool
}

type RemoteConfig struct {
	ServerType  string
	URL         string
	Headers     map[string]string
	HTTPTimeout time.Duration
}

type StdioConfig struct {
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
}

type ResolvedConnection struct {
	ServerType  string
	Remote      *RemoteConfig
	Stdio       *StdioConfig
	HTTPTimeout time.Duration
}

func NormalizeServerType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ServerTypeStdio:
		return ServerTypeStdio, nil
	case ServerTypeSSE:
		return ServerTypeSSE, nil
	case "http", "streamable-http", ServerTypeStreamableHTTP:
		return ServerTypeStreamableHTTP, nil
	default:
		return "", ErrInvalidConnection
	}
}

func (p Policy) Validate() error {
	if !p.RemoteEnabled && !p.StdioEnabled {
		return ErrPolicyDenied
	}
	if p.RemoteEnabled {
		if len(normalizedSet(p.RemoteAllowedHosts)) == 0 ||
			p.RemoteMaxConfigBytes <= 0 || p.RemoteMaxHeaders <= 0 ||
			p.RemoteMaxHeaderBytes <= 0 || p.HTTPTimeout <= 0 {
			return ErrPolicyDenied
		}
		if _, err := NewSafeHTTPPolicy(SafeHTTPPolicyOptions{
			AllowedHosts:    p.RemoteAllowedHosts,
			AllowLocalDebug: p.AllowInsecureHTTP,
		}); err != nil {
			return ErrPolicyDenied
		}
	}
	if p.StdioEnabled {
		if !filepath.IsAbs(strings.TrimSpace(p.StdioWorkdirRoot)) ||
			len(normalizedSet(p.StdioAllowedCommands)) == 0 ||
			p.StdioMaxConfigBytes <= 0 || p.StdioMaxArgs < 0 ||
			p.StdioMaxArgBytes <= 0 || p.StdioMaxEnvVars < 0 ||
			p.StdioMaxEnvValueBytes <= 0 {
			return ErrPolicyDenied
		}
		if _, err := p.commandPolicy(); err != nil {
			return ErrPolicyDenied
		}
	}
	return nil
}

func (p Policy) Resolve(connection Connection) (ResolvedConnection, error) {
	serverType, err := NormalizeServerType(connection.ServerType)
	if err != nil {
		return ResolvedConnection{}, err
	}
	switch serverType {
	case ServerTypeStdio:
		config, err := p.ParseStdio(connection)
		if err != nil {
			return ResolvedConnection{}, err
		}
		return ResolvedConnection{ServerType: serverType, Stdio: &config}, nil
	case ServerTypeSSE, ServerTypeStreamableHTTP:
		config, err := p.ParseRemote(connection)
		if err != nil {
			return ResolvedConnection{}, err
		}
		return ResolvedConnection{
			ServerType:  serverType,
			Remote:      &config,
			HTTPTimeout: config.HTTPTimeout,
		}, nil
	default:
		return ResolvedConnection{}, ErrInvalidConnection
	}
}

func (p Policy) ParseRemote(connection Connection) (RemoteConfig, error) {
	serverType, err := NormalizeServerType(connection.ServerType)
	if err != nil || !p.RemoteEnabled ||
		(serverType != ServerTypeSSE && serverType != ServerTypeStreamableHTTP) {
		return RemoteConfig{}, ErrPolicyDenied
	}
	raw, err := parseConfigObject(connection.Config, p.RemoteMaxConfigBytes)
	if err != nil {
		return RemoteConfig{}, err
	}
	if err := validateConfigObjectKeys(raw, "url", "headers", "auth_headers"); err != nil {
		return RemoteConfig{}, ErrInvalidConnection
	}
	remoteURL, err := parseStringField(raw, "url", true)
	if err != nil {
		return RemoteConfig{}, ErrInvalidConnection
	}
	remoteURL, err = p.validateRemoteURL(remoteURL)
	if err != nil {
		return RemoteConfig{}, err
	}
	headers, err := parseStringMapField(raw, "headers")
	if err != nil {
		return RemoteConfig{}, ErrInvalidConnection
	}
	authHeaders, err := parseStringMapField(raw, "auth_headers")
	if err != nil {
		return RemoteConfig{}, ErrInvalidConnection
	}
	if len(authHeaders) > 0 {
		auth, err := parseAuthObject(connection.Auth)
		if err != nil {
			return RemoteConfig{}, ErrInvalidConnection
		}
		for headerName, authPath := range authHeaders {
			value, err := resolveAuthString(auth, authPath)
			if err != nil {
				return RemoteConfig{}, ErrInvalidConnection
			}
			headers[headerName] = value
		}
	}
	headers, err = p.validateHeaders(headers)
	if err != nil {
		return RemoteConfig{}, err
	}
	return RemoteConfig{
		ServerType:  serverType,
		URL:         remoteURL,
		Headers:     headers,
		HTTPTimeout: p.HTTPTimeout,
	}, nil
}

func (p Policy) ParseStdio(connection Connection) (StdioConfig, error) {
	if !p.StdioEnabled {
		return StdioConfig{}, ErrPolicyDenied
	}
	config, err := ParseStdioConnection(connection, p.StdioMaxConfigBytes)
	if err != nil {
		return StdioConfig{}, err
	}
	if err := p.ValidateStdio(config); err != nil {
		return StdioConfig{}, err
	}
	commandPolicy, err := p.commandPolicy()
	if err != nil {
		return StdioConfig{}, ErrStdioCommandDenied
	}
	config.Command, err = commandPolicy.Resolve(config.Command, config.Args, config.WorkingDir)
	if err != nil {
		return StdioConfig{}, ErrStdioCommandDenied
	}
	return config, nil
}

// ParseStdioConnection parses config and projects auth without applying an
// execution policy. Both ADK and management apply their policy after any
// Coze-owned working directory projection.
func ParseStdioConnection(
	connection Connection,
	maxConfigBytes int,
) (StdioConfig, error) {
	serverType, err := NormalizeServerType(connection.ServerType)
	if err != nil || serverType != ServerTypeStdio {
		return StdioConfig{}, ErrInvalidConnection
	}
	raw, err := parseConfigObject(connection.Config, maxConfigBytes)
	if err != nil {
		return StdioConfig{}, err
	}
	if err := validateConfigObjectKeys(
		raw,
		"command", "args", "auth_args", "env", "auth_env", "cwd", "working_dir", "workingDir",
	); err != nil {
		return StdioConfig{}, ErrInvalidConnection
	}
	command, err := parseStringField(raw, "command", true)
	if err != nil {
		return StdioConfig{}, ErrInvalidConnection
	}
	args, err := parseStringSliceField(raw, "args")
	if err != nil {
		return StdioConfig{}, ErrInvalidConnection
	}
	authArgs, authArgsPresent, err := parseAuthArgReferences(raw, "auth_args")
	if err != nil {
		return StdioConfig{}, ErrInvalidConnection
	}
	_, argsPresent := raw["args"]
	if argsPresent && authArgsPresent {
		return StdioConfig{}, ErrInvalidConnection
	}
	if authArgsPresent {
		auth, err := parseAuthObject(connection.Auth)
		if err != nil {
			return StdioConfig{}, ErrInvalidConnection
		}
		args, err = resolveAuthArgs(auth, authArgs)
		if err != nil {
			return StdioConfig{}, ErrInvalidConnection
		}
	}
	env, err := parseStringMapField(raw, "env")
	if err != nil {
		return StdioConfig{}, ErrInvalidConnection
	}
	authEnv, err := parseStringMapField(raw, "auth_env")
	if err != nil {
		return StdioConfig{}, ErrInvalidConnection
	}
	if len(authEnv) > 0 {
		auth, err := parseAuthObject(connection.Auth)
		if err != nil {
			return StdioConfig{}, ErrInvalidConnection
		}
		for envName, authPath := range authEnv {
			if !validEnvName(envName) {
				return StdioConfig{}, ErrInvalidConnection
			}
			value, err := resolveAuthString(auth, authPath)
			if err != nil {
				return StdioConfig{}, ErrInvalidConnection
			}
			env[envName] = value
		}
	}
	workingDir := firstStringField(raw, "cwd", "working_dir", "workingDir")
	config := StdioConfig{
		Command:    strings.TrimSpace(command),
		Args:       append([]string(nil), args...),
		Env:        cloneStringMap(env),
		WorkingDir: strings.TrimSpace(workingDir),
	}
	return config, nil
}

func (p Policy) ValidateStdio(config StdioConfig) error {
	command := strings.TrimSpace(config.Command)
	if command == "" || strings.ContainsRune(command, 0) {
		return ErrStdioCommandDenied
	}
	if p.StdioMaxArgs >= 0 && len(config.Args) > p.StdioMaxArgs {
		return ErrStdioArgsLimitExceeded
	}
	argBytes := 0
	for _, arg := range config.Args {
		if strings.ContainsRune(arg, 0) {
			return ErrStdioArgsLimitExceeded
		}
		argBytes += len([]byte(arg))
		if p.StdioMaxArgBytes >= 0 && argBytes > p.StdioMaxArgBytes {
			return ErrStdioArgsLimitExceeded
		}
	}
	allowedEnv := normalizedSet(p.StdioAllowedEnvKeys)
	for key, value := range config.Env {
		if !validEnvName(key) {
			return ErrStdioEnvDenied
		}
		if _, ok := allowedEnv[key]; !ok {
			return ErrStdioEnvDenied
		}
		if strings.ContainsRune(value, 0) ||
			(p.StdioMaxEnvValueBytes >= 0 && len([]byte(value)) > p.StdioMaxEnvValueBytes) {
			return ErrStdioEnvLimitExceeded
		}
	}
	if p.StdioMaxEnvVars >= 0 && len(config.Env) > p.StdioMaxEnvVars {
		return ErrStdioEnvLimitExceeded
	}
	workingDir := strings.TrimSpace(config.WorkingDir)
	if workingDir == "" {
		if p.RequireWorkingDir {
			return ErrStdioWorkingDirRequired
		}
	} else {
		if p.RejectUserWorkingDir || !filepath.IsAbs(workingDir) {
			return ErrStdioWorkingDirDenied
		}
		cleanWorkingDir := filepath.Clean(workingDir)
		allowed := false
		for _, prefix := range cleanAbsolutePaths(p.StdioAllowedWorkingDirPrefixes) {
			if pathWithin(cleanWorkingDir, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return ErrStdioWorkingDirDenied
		}
	}

	commandPolicy, err := p.commandPolicy()
	if err != nil {
		return ErrStdioCommandDenied
	}
	if _, err := commandPolicy.Resolve(command, config.Args, config.WorkingDir); err != nil {
		return ErrStdioCommandDenied
	}
	return nil
}

func (p Policy) ValidateRemoteURL(value string) error {
	_, err := p.validateRemoteURL(value)
	return err
}

func (p Policy) validateRemoteURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", ErrInvalidConnection
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", ErrInvalidConnection
	}
	policy, err := NewSafeHTTPPolicy(SafeHTTPPolicyOptions{
		AllowedHosts:    p.RemoteAllowedHosts,
		AllowLocalDebug: p.AllowInsecureHTTP,
	})
	if err != nil || policy.ValidateURL(parsed) != nil {
		return "", ErrInvalidConnection
	}
	return parsed.String(), nil
}

func (p Policy) validateHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) > p.RemoteMaxHeaders {
		return nil, ErrInvalidConnection
	}
	result := make(map[string]string, len(headers))
	canonicalNames := make(map[string]struct{}, len(headers))
	totalBytes := 0
	for rawKey, value := range headers {
		key := strings.TrimSpace(rawKey)
		if key != rawKey || !validHeaderName(key) || strings.ContainsAny(value, "\r\n") {
			return nil, ErrInvalidConnection
		}
		canonical := strings.ToLower(key)
		if _, exists := canonicalNames[canonical]; exists {
			return nil, ErrInvalidConnection
		}
		canonicalNames[canonical] = struct{}{}
		totalBytes += len([]byte(key)) + len([]byte(value))
		if totalBytes > p.RemoteMaxHeaderBytes {
			return nil, ErrInvalidConnection
		}
		result[key] = value
	}
	return result, nil
}

func NewCommandContext(
	ctx context.Context,
	config StdioConfig,
) (*exec.Cmd, error) {
	return NewCommandContextWithEnv(
		ctx,
		config.Command,
		config.Args,
		sortedEnv(config.Env),
		config.WorkingDir,
	)
}

func NewCommandContextWithEnv(
	ctx context.Context,
	command string,
	args []string,
	env []string,
	workingDir string,
) (*exec.Cmd, error) {
	command = strings.TrimSpace(command)
	workingDir = strings.TrimSpace(workingDir)
	if command == "" || strings.ContainsRune(command, 0) ||
		workingDir == "" || !filepath.IsAbs(workingDir) {
		return nil, ErrInvalidConnection
	}
	for _, argument := range append([]string(nil), args...) {
		if strings.ContainsRune(argument, 0) {
			return nil, ErrInvalidConnection
		}
	}
	resolvedCommand, childEnv, err := resolveCommandEnvironment(command, env)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, resolvedCommand, append([]string(nil), args...)...)
	cmd.Env = childEnv
	cmd.Dir = filepath.Clean(workingDir)
	return cmd, nil
}

func resolveCommandEnvironment(command string, env []string) (string, []string, error) {
	environment := make(map[string]string, len(env)+1)
	for _, item := range append([]string(nil), env...) {
		if strings.ContainsRune(item, 0) {
			return "", nil, ErrInvalidConnection
		}
		separator := strings.IndexByte(item, '=')
		if separator <= 0 {
			return "", nil, ErrInvalidConnection
		}
		key, value := item[:separator], item[separator+1:]
		if !validEnvName(key) {
			return "", nil, ErrInvalidConnection
		}
		if key == "PATH" {
			return "", nil, ErrInvalidConnection
		}
		if _, duplicate := environment[key]; duplicate {
			return "", nil, ErrInvalidConnection
		}
		environment[key] = value
	}

	resolvedCommand, err := resolveExecutable(command)
	if err != nil {
		return "", nil, err
	}
	environment["PATH"] = filepath.Dir(resolvedCommand)
	return resolvedCommand, sortedEnv(environment), nil
}

func resolveExecutable(command string) (string, error) {
	if filepath.IsAbs(command) {
		command = filepath.Clean(command)
		resolved, err := filepath.EvalSymlinks(command)
		if err != nil || !filepath.IsAbs(resolved) {
			return "", ErrInvalidConnection
		}
		command = filepath.Clean(resolved)
		if !isExecutableFile(command) {
			return "", ErrInvalidConnection
		}
		return command, nil
	}
	if strings.ContainsRune(command, filepath.Separator) {
		return "", ErrInvalidConnection
	}
	resolved, err := exec.LookPath(command)
	if err != nil {
		return "", ErrInvalidConnection
	}
	if !filepath.IsAbs(resolved) {
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return "", ErrInvalidConnection
		}
	}
	resolved = filepath.Clean(resolved)
	resolved, err = filepath.EvalSymlinks(resolved)
	if err != nil || !filepath.IsAbs(resolved) {
		return "", ErrInvalidConnection
	}
	resolved = filepath.Clean(resolved)
	if !isExecutableFile(resolved) {
		return "", ErrInvalidConnection
	}
	return resolved, nil
}

func (p Policy) commandPolicy() (*StdioCommandPolicy, error) {
	if p.StdioCommandPolicy != nil {
		return p.StdioCommandPolicy, nil
	}
	return NewStdioCommandPolicy(StdioCommandPolicyOptions{
		AllowedCommands: p.StdioAllowedCommands,
		NpxPackages:     p.StdioAllowedNpxPackages,
		UvxPackages:     p.StdioAllowedUVXPackages,
		NodeScriptRoots: p.StdioAllowedNodeScriptRoots,
		CommandRules:    p.StdioCommandRules,
	})
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func parseConfigObject(rawConfig string, maxBytes int) (map[string]json.RawMessage, error) {
	rawConfig = strings.TrimSpace(rawConfig)
	if rawConfig == "" || maxBytes <= 0 || len([]byte(rawConfig)) > maxBytes {
		return nil, ErrInvalidConnection
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawConfig), &raw); err != nil || raw == nil {
		return nil, ErrInvalidConnection
	}
	return raw, nil
}

func validateConfigObjectKeys(raw map[string]json.RawMessage, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range raw {
		if _, ok := allowedSet[key]; !ok {
			return ErrInvalidConnection
		}
	}
	return nil
}

func parseAuthObject(rawAuth string) (map[string]any, error) {
	var auth map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawAuth)), &auth); err != nil || auth == nil {
		return nil, ErrInvalidConnection
	}
	return auth, nil
}

func parseStringField(raw map[string]json.RawMessage, key string, required bool) (string, error) {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		if required {
			return "", ErrInvalidConnection
		}
		return "", nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil || (required && strings.TrimSpace(result) == "") {
		return "", ErrInvalidConnection
	}
	return result, nil
}

func firstStringField(raw map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		value, err := parseStringField(raw, key, false)
		if err == nil && value != "" {
			return value
		}
	}
	return ""
}

func parseStringSliceField(raw map[string]json.RawMessage, key string) ([]string, error) {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return []string{}, nil
	}
	var result []string
	if err := json.Unmarshal(value, &result); err != nil || result == nil {
		return nil, ErrInvalidConnection
	}
	return result, nil
}

func parseAuthArgReferences(
	raw map[string]json.RawMessage,
	key string,
) (map[string]string, bool, error) {
	value, present := raw[key]
	if !present || string(value) == "null" {
		return map[string]string{}, false, nil
	}
	var references map[string]string
	if err := json.Unmarshal(value, &references); err != nil || references == nil {
		return nil, false, ErrInvalidConnection
	}
	result := make(map[string]string, len(references))
	for index := 0; index < len(references); index++ {
		indexKey := strconv.Itoa(index)
		reference, ok := references[indexKey]
		if !ok || reference != "args."+indexKey {
			return nil, false, ErrInvalidConnection
		}
		result[indexKey] = reference
	}
	if len(result) != len(references) {
		return nil, false, ErrInvalidConnection
	}
	return result, true, nil
}

func resolveAuthArgs(auth map[string]any, references map[string]string) ([]string, error) {
	result := make([]string, len(references))
	for index := 0; index < len(references); index++ {
		value, err := resolveAuthStringValue(auth, references[strconv.Itoa(index)], true)
		if err != nil {
			return nil, ErrInvalidConnection
		}
		result[index] = value
	}
	return result, nil
}

func parseStringMapField(raw map[string]json.RawMessage, key string) (map[string]string, error) {
	value, ok := raw[key]
	if !ok || string(value) == "null" {
		return map[string]string{}, nil
	}
	var result map[string]string
	if err := json.Unmarshal(value, &result); err != nil || result == nil {
		return nil, ErrInvalidConnection
	}
	return result, nil
}

func resolveAuthString(payload map[string]any, path string) (string, error) {
	return resolveAuthStringValue(payload, path, false)
}

func resolveAuthStringValue(payload map[string]any, path string, allowEmpty bool) (string, error) {
	segments := strings.Split(strings.TrimSpace(path), ".")
	if len(segments) == 0 {
		return "", ErrInvalidConnection
	}
	var current any = payload
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		object, ok := current.(map[string]any)
		if segment == "" || !ok {
			return "", ErrInvalidConnection
		}
		value, ok := object[segment]
		if !ok {
			return "", ErrInvalidConnection
		}
		current = value
	}
	result, ok := current.(string)
	if !ok || !allowEmpty && strings.TrimSpace(result) == "" {
		return "", ErrInvalidConnection
	}
	return result, nil
}

func normalizedSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func StringSet(values []string) map[string]struct{} {
	return normalizedSet(values)
}

func remoteHostAllowed(hostport string, allowed []string) bool {
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			set[value] = struct{}{}
		}
	}
	hostport = strings.ToLower(strings.TrimSpace(hostport))
	if _, ok := set[hostport]; ok {
		return true
	}
	host := hostport
	if parsedHost, _, err := net.SplitHostPort(hostport); err == nil {
		host = strings.Trim(parsedHost, "[]")
	}
	_, ok := set[host]
	return ok
}

func isLocalHost(host string) bool {
	host = strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func validHeaderName(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r <= 32 || r >= 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={}", r) {
			return false
		}
	}
	return true
}

func validEnvName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for index, r := range value {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func sortedEnv(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result
}

func cleanAbsolutePaths(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if filepath.IsAbs(value) {
			result = append(result, filepath.Clean(value))
		}
	}
	return result
}

func pathWithin(target, root string) bool {
	if target == root {
		return true
	}
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func PathWithin(target, root string) bool {
	return pathWithin(target, root)
}

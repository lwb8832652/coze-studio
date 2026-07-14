// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const (
	mcpConfigWriteOnlySentinel                  = "__COZE_MCP_WRITE_ONLY__"
	mcpConfigCredentialMigrationMaxCASAttempts = 3
)

var errMCPConfigCredentialMigration = errors.New("mcp config credential migration failed")

type mcpConfigTransport uint8

const (
	mcpConfigTransportRemote mcpConfigTransport = iota + 1
	mcpConfigTransportStdio
)

type mcpConfigCredentialKind struct {
	configField string
	authField   string
	refField    string
	refPrefix   string
}

var (
	mcpHeaderCredentialKind = mcpConfigCredentialKind{
		configField: "headers", authField: "headers", refField: "auth_headers", refPrefix: "headers.",
	}
	mcpEnvCredentialKind = mcpConfigCredentialKind{
		configField: "env", authField: "env", refField: "auth_env", refPrefix: "env.",
	}
)

type mcpConfigCredentialOptions struct {
	existingAuth        map[string]any
	configOverridesAuth bool
	storedMigration     bool
}

func canonicalizeMCPUpsertCredentials(
	serverType string,
	configRaw string,
	authRaw string,
	existing *toolapi.MCPToolServer,
	preserveAuth bool,
) (string, string, error) {
	var existingAuth map[string]any
	if existing != nil {
		var err error
		existingAuth, err = parseMCPJSONObject(existing.Auth, "auth")
		if err != nil {
			return "", "", err
		}
	}
	config, auth, err := canonicalizeMCPConfigCredentials(
		serverType,
		configRaw,
		authRaw,
		mcpConfigCredentialOptions{
			existingAuth:        existingAuth,
			configOverridesAuth: preserveAuth,
		},
	)
	if err != nil {
		return "", "", err
	}
	if jsonObjectsSemanticallyEqual(normalizeJSONText(configRaw), config) {
		config = normalizeJSONText(configRaw)
	}
	if jsonObjectsSemanticallyEqual(normalizeJSONText(authRaw), auth) {
		auth = normalizeJSONText(authRaw)
	}
	return config, auth, nil
}

func canonicalizeStoredMCPServerCredentials(
	server *toolapi.MCPToolServer,
) (*toolapi.MCPToolServer, bool, error) {
	if server == nil {
		return nil, false, errMCPConfigCredentialMigration
	}
	config, auth, err := canonicalizeMCPConfigCredentials(
		server.ServerType,
		server.Config,
		server.Auth,
		mcpConfigCredentialOptions{storedMigration: true},
	)
	if err != nil {
		return cloneServer(server), false, err
	}
	changed := !jsonObjectsSemanticallyEqual(server.Config, config) ||
		!jsonObjectsSemanticallyEqual(server.Auth, auth)
	canonical := cloneServer(server)
	if changed {
		canonical.Config = config
		canonical.Auth = auth
	}
	return canonical, changed, nil
}

func canonicalizeMCPConfigCredentials(
	serverType string,
	configRaw string,
	authRaw string,
	options mcpConfigCredentialOptions,
) (string, string, error) {
	config, err := parseMCPJSONObject(normalizeJSONText(configRaw), "config")
	if err != nil {
		return "", "", err
	}
	transport, err := validateMCPConfigSchema(serverType, config)
	if err != nil {
		return "", "", err
	}
	auth, err := parseMCPJSONObject(normalizeJSONText(authRaw), "auth")
	if err != nil {
		return "", "", err
	}

	switch transport {
	case mcpConfigTransportRemote:
		err = canonicalizeMCPConfigCredentialKind(config, auth, mcpHeaderCredentialKind, options)
	case mcpConfigTransportStdio:
		if err = canonicalizeMCPStdioArgs(config, auth, options); err == nil {
			err = canonicalizeMCPConfigCredentialKind(config, auth, mcpEnvCredentialKind, options)
		}
	default:
		err = invalidMCPConfigCredentialError()
	}
	if err != nil {
		return "", "", err
	}
	configJSON, err := marshalMCPJSONObject(config)
	if err != nil {
		return "", "", err
	}
	authJSON, err := marshalMCPJSONObject(auth)
	if err != nil {
		return "", "", err
	}
	return configJSON, authJSON, nil
}

func validateMCPConfigSchema(serverType string, config map[string]any) (mcpConfigTransport, error) {
	transport, err := mcpConfigTransportForServerType(serverType)
	if err != nil {
		return 0, err
	}
	allowed := map[string]struct{}{}
	switch transport {
	case mcpConfigTransportRemote:
		for _, key := range []string{"url", "headers", "auth_headers"} {
			allowed[key] = struct{}{}
		}
	case mcpConfigTransportStdio:
		for _, key := range []string{"command", "args", "auth_args", "env", "auth_env", "cwd", "working_dir", "workingDir"} {
			allowed[key] = struct{}{}
		}
	}
	for key := range config {
		if _, ok := allowed[key]; !ok {
			return 0, invalidMCPConfigCredentialError()
		}
	}
	if transport == mcpConfigTransportRemote {
		if rawURL, present := config["url"]; present {
			value, ok := rawURL.(string)
			if !ok || validateCanonicalMCPRemoteURL(value) != nil {
				return 0, invalidMCPConfigCredentialError()
			}
		}
		return transport, nil
	}
	if rawCommand, present := config["command"]; present {
		command, ok := rawCommand.(string)
		if !ok || strings.TrimSpace(command) == "" || strings.ContainsRune(command, 0) {
			return 0, invalidMCPConfigCredentialError()
		}
	}
	workingDirFields := 0
	for _, key := range []string{"cwd", "working_dir", "workingDir"} {
		if raw, present := config[key]; present {
			if _, ok := raw.(string); !ok {
				return 0, invalidMCPConfigCredentialError()
			}
			workingDirFields++
		}
	}
	if workingDirFields > 1 {
		return 0, invalidMCPConfigCredentialError()
	}
	return transport, nil
}

func mcpConfigTransportForServerType(serverType string) (mcpConfigTransport, error) {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(serverType), "-", "_")) {
	case "sse", "http", "streamable_http":
		return mcpConfigTransportRemote, nil
	case "stdio":
		return mcpConfigTransportStdio, nil
	default:
		return 0, invalidMCPConfigCredentialError()
	}
}

func validateCanonicalMCPRemoteURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return invalidMCPConfigCredentialError()
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return invalidMCPConfigCredentialError()
	}
	return nil
}

func sanitizedMCPRemoteURL(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return "", false
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String(), true
}

func canonicalizeMCPStdioArgs(
	config map[string]any,
	auth map[string]any,
	options mcpConfigCredentialOptions,
) error {
	authArgs, authPresent, err := mcpAuthArgs(auth)
	if err != nil {
		return err
	}
	refs, refsPresent, err := mcpArgReferencesFromConfig(config)
	if err != nil {
		return err
	}
	rawArgs, argsPresent := config["args"]
	if argsPresent {
		values, ok := rawArgs.([]any)
		if !ok || values == nil {
			return invalidMCPConfigCredentialError()
		}
		args := make([]string, len(values))
		for index, raw := range values {
			value, ok := raw.(string)
			if !ok {
				return invalidMCPConfigCredentialError()
			}
			args[index] = value
		}
		if len(args) == 0 {
			if refsPresent && len(refs) > 0 || authPresent && !options.configOverridesAuth {
				return invalidMCPConfigCredentialError()
			}
			delete(config, "args")
			delete(config, "auth_args")
			delete(auth, "args")
			return nil
		}
		expectedRefs := mcpArgReferences(args)
		if refsPresent && !reflect.DeepEqual(refs, expectedRefs) {
			return invalidMCPConfigCredentialError()
		}
		if authPresent && !options.configOverridesAuth && !reflect.DeepEqual(authArgs, args) {
			return invalidMCPConfigCredentialError()
		}
		auth["args"] = indexedStringSliceToAnyMap(args)
		config["auth_args"] = stringMapToAnyMap(expectedRefs)
		delete(config, "args")
		return nil
	}
	if refsPresent {
		if len(refs) == 0 && !authPresent {
			delete(config, "auth_args")
			return nil
		}
		if !authPresent || !reflect.DeepEqual(refs, mcpArgReferences(authArgs)) {
			return invalidMCPConfigCredentialError()
		}
		return nil
	}
	if authPresent {
		if len(authArgs) == 0 {
			delete(auth, "args")
			return nil
		}
		config["auth_args"] = stringMapToAnyMap(mcpArgReferences(authArgs))
	}
	return nil
}

func mcpAuthArgs(auth map[string]any) ([]string, bool, error) {
	raw, present := auth["args"]
	if !present {
		return []string{}, false, nil
	}
	values, ok := raw.(map[string]any)
	if !ok || values == nil {
		return nil, false, invalidMCPConfigCredentialError()
	}
	result := make([]string, len(values))
	for index := 0; index < len(values); index++ {
		value, ok := values[strconv.Itoa(index)].(string)
		if !ok {
			return nil, false, invalidMCPConfigCredentialError()
		}
		result[index] = value
	}
	if len(values) != len(result) {
		return nil, false, invalidMCPConfigCredentialError()
	}
	return result, true, nil
}

func mcpArgReferencesFromConfig(config map[string]any) (map[string]string, bool, error) {
	raw, present := config["auth_args"]
	if !present {
		return map[string]string{}, false, nil
	}
	values, ok := raw.(map[string]any)
	if !ok || values == nil {
		return nil, false, invalidMCPConfigCredentialError()
	}
	result := make(map[string]string, len(values))
	for index := 0; index < len(values); index++ {
		key := strconv.Itoa(index)
		value, ok := values[key].(string)
		if !ok || value != "args."+key {
			return nil, false, invalidMCPConfigCredentialError()
		}
		result[key] = value
	}
	if len(values) != len(result) {
		return nil, false, invalidMCPConfigCredentialError()
	}
	return result, true, nil
}

func mcpArgReferences(args []string) map[string]string {
	result := make(map[string]string, len(args))
	for index := range args {
		key := strconv.Itoa(index)
		result[key] = "args." + key
	}
	return result
}

func indexedStringSliceToAnyMap(values []string) map[string]any {
	result := make(map[string]any, len(values))
	for index, value := range values {
		result[strconv.Itoa(index)] = value
	}
	return result
}

func canonicalizeMCPConfigCredentialKind(
	config map[string]any,
	auth map[string]any,
	kind mcpConfigCredentialKind,
	options mcpConfigCredentialOptions,
) error {
	authValues, authPresent, err := mcpCredentialStringMap(auth, kind.authField, kind)
	if err != nil {
		return err
	}
	existingValues, _, err := mcpCredentialStringMap(options.existingAuth, kind.authField, kind)
	if err != nil {
		return err
	}
	refs, refsPresent, err := mcpCredentialReferenceMap(config, kind)
	if err != nil {
		return err
	}

	inlineRaw, inlinePresent := config[kind.configField]
	if inlinePresent {
		inline, ok := inlineRaw.(map[string]any)
		if !ok || inline == nil {
			return invalidMCPConfigCredentialError()
		}
		delete(config, kind.configField)
		if len(inline) > 0 {
			resolved, err := resolveMCPInlineCredentialMap(inline, existingValues, kind)
			if err != nil {
				return err
			}
			if len(resolved) == 0 {
				delete(auth, kind.authField)
				delete(config, kind.refField)
				return nil
			}
			if authPresent && !options.configOverridesAuth && !reflect.DeepEqual(authValues, resolved) {
				return invalidMCPConfigCredentialError()
			}
			expectedRefs := mcpCredentialReferences(resolved, kind)
			if refsPresent && !reflect.DeepEqual(refs, expectedRefs) {
				return invalidMCPConfigCredentialError()
			}
			auth[kind.authField] = stringMapToAnyMap(resolved)
			config[kind.refField] = stringMapToAnyMap(expectedRefs)
			return nil
		}

		if refsPresent && len(refs) > 0 {
			if !authPresent || !reflect.DeepEqual(refs, mcpCredentialReferences(authValues, kind)) {
				return invalidMCPConfigCredentialError()
			}
			return nil
		}
		if refsPresent || options.configOverridesAuth {
			delete(auth, kind.authField)
			delete(config, kind.refField)
			return nil
		}
	}

	if refsPresent {
		if len(refs) == 0 && !authPresent {
			delete(config, kind.refField)
			return nil
		}
		if !authPresent || !reflect.DeepEqual(refs, mcpCredentialReferences(authValues, kind)) {
			return invalidMCPConfigCredentialError()
		}
		return nil
	}
	if authPresent {
		if len(authValues) == 0 {
			delete(auth, kind.authField)
			return nil
		}
		config[kind.refField] = stringMapToAnyMap(mcpCredentialReferences(authValues, kind))
	}
	return nil
}

func resolveMCPInlineCredentialMap(
	inline map[string]any,
	existing map[string]string,
	kind mcpConfigCredentialKind,
) (map[string]string, error) {
	result := make(map[string]string, len(inline))
	canonicalHeaderNames := make(map[string]struct{}, len(inline))
	for key, rawValue := range inline {
		if !validMCPConfigCredentialKey(key, kind) {
			return nil, invalidMCPConfigCredentialError()
		}
		if kind.authField == "headers" {
			canonical := strings.ToLower(key)
			if _, duplicate := canonicalHeaderNames[canonical]; duplicate {
				return nil, invalidMCPConfigCredentialError()
			}
			canonicalHeaderNames[canonical] = struct{}{}
		}
		value, ok := rawValue.(string)
		if !ok {
			return nil, invalidMCPConfigCredentialError()
		}
		if value == mcpConfigWriteOnlySentinel {
			preserved, ok := existing[key]
			if !ok || strings.TrimSpace(preserved) == "" {
				return nil, invalidMCPConfigCredentialError()
			}
			result[key] = preserved
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		result[key] = value
	}
	return result, nil
}

func mcpCredentialStringMap(
	object map[string]any,
	field string,
	kind mcpConfigCredentialKind,
) (map[string]string, bool, error) {
	if object == nil {
		return map[string]string{}, false, nil
	}
	raw, present := object[field]
	if !present {
		return map[string]string{}, false, nil
	}
	values, ok := raw.(map[string]any)
	if !ok || values == nil {
		return nil, false, invalidMCPConfigCredentialError()
	}
	result := make(map[string]string, len(values))
	canonicalHeaderNames := make(map[string]struct{}, len(values))
	for key, rawValue := range values {
		if !validMCPConfigCredentialKey(key, kind) {
			return nil, false, invalidMCPConfigCredentialError()
		}
		if kind.authField == "headers" {
			canonical := strings.ToLower(key)
			if _, duplicate := canonicalHeaderNames[canonical]; duplicate {
				return nil, false, invalidMCPConfigCredentialError()
			}
			canonicalHeaderNames[canonical] = struct{}{}
		}
		value, ok := rawValue.(string)
		if !ok || value == mcpConfigWriteOnlySentinel {
			return nil, false, invalidMCPConfigCredentialError()
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		result[key] = value
	}
	return result, true, nil
}

func mcpCredentialReferenceMap(
	config map[string]any,
	kind mcpConfigCredentialKind,
) (map[string]string, bool, error) {
	raw, present := config[kind.refField]
	if !present {
		return map[string]string{}, false, nil
	}
	values, ok := raw.(map[string]any)
	if !ok || values == nil {
		return nil, false, invalidMCPConfigCredentialError()
	}
	result := make(map[string]string, len(values))
	for key, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || !validMCPConfigCredentialKey(key, kind) || value != kind.refPrefix+key {
			return nil, false, invalidMCPConfigCredentialError()
		}
		result[key] = value
	}
	return result, true, nil
}

func mcpCredentialReferences(values map[string]string, kind mcpConfigCredentialKind) map[string]string {
	result := make(map[string]string, len(values))
	for key := range values {
		result[key] = kind.refPrefix + key
	}
	return result
}

func validMCPConfigCredentialKey(key string, kind mcpConfigCredentialKind) bool {
	if strings.TrimSpace(key) != key || key == "" || strings.Contains(key, ".") {
		return false
	}
	if kind.authField == "env" {
		for index, r := range key {
			if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || index > 0 && r >= '0' && r <= '9' {
				continue
			}
			return false
		}
	}
	return true
}

func maskMCPConfigForResponse(serverType string, raw string) string {
	config, err := parseMCPJSONObject(normalizeJSONText(raw), "config")
	if err != nil {
		return "{}"
	}
	transport, err := mcpConfigTransportForServerType(serverType)
	if err != nil {
		return "{}"
	}
	result := make(map[string]any)
	if transport == mcpConfigTransportRemote {
		if value, ok := config["url"].(string); ok {
			if safeURL, ok := sanitizedMCPRemoteURL(value); ok {
				result["url"] = safeURL
			}
		}
		projectMaskedMCPCredentialConfig(result, config, mcpHeaderCredentialKind)
	} else {
		if command, ok := config["command"].(string); ok && strings.TrimSpace(command) != "" {
			result["command"] = command
		}
		if refs, ok := safeMCPArgReferences(config["auth_args"]); ok && len(refs) > 0 {
			result["auth_args"] = refs
		}
		projectMaskedMCPCredentialConfig(result, config, mcpEnvCredentialKind)
		for _, key := range []string{"cwd", "working_dir", "workingDir"} {
			if value, ok := config[key].(string); ok {
				result[key] = value
				break
			}
		}
	}
	encoded, err := marshalMCPJSONObject(result)
	if err != nil {
		return "{}"
	}
	return encoded
}

func projectMaskedMCPCredentialConfig(
	result map[string]any,
	config map[string]any,
	kind mcpConfigCredentialKind,
) {
	masked := make(map[string]any)
	if inline, ok := config[kind.configField].(map[string]any); ok {
		for key := range inline {
			if validMCPConfigCredentialKey(key, kind) {
				masked[key] = mcpConfigWriteOnlySentinel
			}
		}
	}
	refs := safeMCPConfigCredentialReferences(config[kind.refField], kind)
	if len(refs) > 0 {
		result[kind.refField] = refs
		for key := range refs {
			masked[key] = mcpConfigWriteOnlySentinel
		}
	}
	if len(masked) > 0 {
		result[kind.configField] = masked
	}
}

func safeMCPConfigForExport(serverType string, raw string) string {
	config, err := parseMCPJSONObject(normalizeJSONText(raw), "config")
	if err != nil {
		return "{}"
	}
	transport, err := mcpConfigTransportForServerType(serverType)
	if err != nil {
		return "{}"
	}
	result := make(map[string]any)
	if transport == mcpConfigTransportRemote {
		if value, ok := config["url"].(string); ok {
			if safeURL, ok := sanitizedMCPRemoteURL(value); ok {
				result["url"] = safeURL
			}
		}
		if refs := safeMCPConfigCredentialReferences(config["auth_headers"], mcpHeaderCredentialKind); len(refs) > 0 {
			result["auth_headers"] = refs
		}
	} else {
		if command, ok := config["command"].(string); ok && strings.TrimSpace(command) != "" {
			result["command"] = command
		}
		if refs, ok := safeMCPArgReferences(config["auth_args"]); ok && len(refs) > 0 {
			result["auth_args"] = refs
		}
		if refs := safeMCPConfigCredentialReferences(config["auth_env"], mcpEnvCredentialKind); len(refs) > 0 {
			result["auth_env"] = refs
		}
	}
	encoded, err := marshalMCPJSONObject(result)
	if err != nil {
		return "{}"
	}
	return encoded
}

func safeMCPArgReferences(raw any) (map[string]string, bool) {
	values, ok := raw.(map[string]any)
	if !ok || values == nil {
		return map[string]string{}, false
	}
	result := make(map[string]string, len(values))
	for index := 0; index < len(values); index++ {
		key := strconv.Itoa(index)
		value, ok := values[key].(string)
		if !ok || value != "args."+key {
			return map[string]string{}, false
		}
		result[key] = value
	}
	return result, len(result) == len(values)
}

func safeMCPConfigCredentialReferences(raw any, kind mcpConfigCredentialKind) map[string]string {
	values, ok := raw.(map[string]any)
	if !ok || values == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(values))
	for key, rawValue := range values {
		value, ok := rawValue.(string)
		if ok && validMCPConfigCredentialKey(key, kind) && value == kind.refPrefix+key {
			result[key] = value
		}
	}
	return result
}

func validatePersistableMCPConfig(serverType string, raw string) error {
	config, err := parseMCPJSONObject(normalizeJSONText(raw), "config")
	if err != nil {
		return errors.New("mcp tool config must be a JSON object")
	}
	transport, err := validateMCPConfigSchema(serverType, config)
	if err != nil {
		return errors.New("mcp tool config violates the transport schema")
	}
	if transport == mcpConfigTransportRemote {
		if headers, present := config["headers"]; present {
			values, ok := headers.(map[string]any)
			if !ok || len(values) > 0 {
				return errors.New("mcp tool config contains plaintext credentials")
			}
		}
		if _, _, err := mcpCredentialReferenceMap(config, mcpHeaderCredentialKind); err != nil {
			return errors.New("mcp tool config contains invalid auth references")
		}
		return nil
	}
	if _, present := config["args"]; present {
		return errors.New("mcp tool config contains plaintext arguments")
	}
	if _, present := config["env"]; present {
		return errors.New("mcp tool config contains plaintext credentials")
	}
	if _, _, err := mcpArgReferencesFromConfig(config); err != nil {
		return errors.New("mcp tool config contains invalid argument references")
	}
	if _, _, err := mcpCredentialReferenceMap(config, mcpEnvCredentialKind); err != nil {
		return errors.New("mcp tool config contains invalid auth references")
	}
	return nil
}

func (s *ApplicationService) canonicalizeServerCredentials(
	ctx context.Context,
	server *toolapi.MCPToolServer,
) (*toolapi.MCPToolServer, error) {
	current := cloneServer(server)
	for attempt := 0; attempt < mcpConfigCredentialMigrationMaxCASAttempts; attempt++ {
		canonical, changed, err := canonicalizeStoredMCPServerCredentials(current)
		if err != nil {
			return current, errMCPConfigCredentialMigration
		}
		if !changed {
			return canonical, nil
		}
		canonical.UpdatedAt = nextMCPServerUpdatedAt(current.UpdatedAt)
		err = s.components.Catalog.UpdateServer(ctx, canonical, current.UpdatedAt)
		if err == nil {
			return canonical, nil
		}
		if !errors.Is(err, ErrMCPConflict) {
			return canonical, errMCPConfigCredentialMigration
		}
		if attempt+1 >= mcpConfigCredentialMigrationMaxCASAttempts {
			return canonical, errMCPConfigCredentialMigration
		}
		current, err = s.components.Catalog.Get(ctx, current.ServerID)
		if err != nil || current.SpaceID != server.SpaceID {
			return canonical, errMCPConfigCredentialMigration
		}
	}
	return current, errMCPConfigCredentialMigration
}

func parseMCPJSONObject(raw string, name string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &result); err != nil || result == nil {
		return nil, InvalidArgumentErrorf("invalid %s json: expected object", name)
	}
	return result, nil
}

func marshalMCPJSONObject(value map[string]any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", invalidMCPConfigCredentialError()
	}
	return string(encoded), nil
}

func jsonObjectsSemanticallyEqual(left string, right string) bool {
	leftObject, leftErr := parseMCPJSONObject(normalizeJSONText(left), "json")
	rightObject, rightErr := parseMCPJSONObject(normalizeJSONText(right), "json")
	return leftErr == nil && rightErr == nil && reflect.DeepEqual(leftObject, rightObject)
}

func invalidMCPConfigCredentialError() error {
	return InvalidArgumentErrorf("invalid mcp config credential mapping")
}

func stringMapToAnyMap(values map[string]string) map[string]any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(map[string]any, len(values))
	for _, key := range keys {
		result[key] = values[key]
	}
	return result
}

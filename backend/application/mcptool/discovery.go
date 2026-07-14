// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	toolmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const (
	maxDiscoveredCapabilities    = 500
	maxDiscoveredCapabilityBytes = 1024 * 1024
	maxMCPNameRunes              = 128
	maxMCPDescriptionRunes       = 4096
	maxMCPURIRunes               = 2048
	maxMCPMIMETypeRunes          = 256
	maxMCPInputSchemaBytes       = 64 * 1024
)

var (
	ErrCapabilityDiscoveryUnavailable = errors.New("MCP capability discovery is unavailable")
	ErrCapabilityDiscoveryInvalid     = errors.New("MCP capability discovery returned an invalid result")
	ErrCapabilityDiscoveryTooLarge    = errors.New("MCP capability discovery exceeds the response limit")
)

// MCPServerConnection is an internal runtime contract. Auth must never be
// copied into API responses, audit metadata, or exports.
type MCPServerConnection struct {
	ServerType string
	Config     string
	Auth       string
}

type DiscoveredCapabilities struct {
	Tools     []*toolmodel.MCPToolDefinition
	Resources []*toolmodel.MCPResource
	Prompts   []*toolmodel.MCPPrompt
}

type CapabilityDiscoverer interface {
	Discover(ctx context.Context, connection MCPServerConnection) (*DiscoveredCapabilities, error)
}

func discoverCapabilities(
	ctx context.Context,
	discoverer CapabilityDiscoverer,
	connection MCPServerConnection,
) (*DiscoveredCapabilities, error) {
	if discoverer == nil {
		return nil, ErrCapabilityDiscoveryUnavailable
	}

	capabilities, err := discoverer.Discover(ctx, connection)
	if err != nil {
		return nil, err
	}
	if capabilities == nil {
		return nil, ErrCapabilityDiscoveryInvalid
	}
	if len(capabilities.Tools)+len(capabilities.Resources)+len(capabilities.Prompts) > maxDiscoveredCapabilities {
		return nil, ErrCapabilityDiscoveryTooLarge
	}
	if err := validateDiscoveredCapabilities(capabilities); err != nil {
		return nil, err
	}

	return &DiscoveredCapabilities{
		Tools:     append([]*toolmodel.MCPToolDefinition(nil), capabilities.Tools...),
		Resources: append([]*toolmodel.MCPResource(nil), capabilities.Resources...),
		Prompts:   append([]*toolmodel.MCPPrompt(nil), capabilities.Prompts...),
	}, nil
}

func validateDiscoveredCapabilities(capabilities *DiscoveredCapabilities) error {
	encoded, err := json.Marshal(capabilities)
	if err != nil {
		return fmt.Errorf("%w: capabilities cannot be serialized", ErrCapabilityDiscoveryInvalid)
	}
	if len(encoded) > maxDiscoveredCapabilityBytes {
		return ErrCapabilityDiscoveryTooLarge
	}

	toolNames := make(map[string]struct{}, len(capabilities.Tools))
	for _, item := range capabilities.Tools {
		if item == nil || !validMCPDiscoveryName(item.Name) {
			return fmt.Errorf("%w: invalid tool name", ErrCapabilityDiscoveryInvalid)
		}
		if runeLen(item.Description) > maxMCPDescriptionRunes {
			return fmt.Errorf("%w: tool description is too long", ErrCapabilityDiscoveryInvalid)
		}
		if len(item.InputSchema) > maxMCPInputSchemaBytes || !validJSONObject(item.InputSchema) {
			return fmt.Errorf("%w: invalid tool input_schema", ErrCapabilityDiscoveryInvalid)
		}
		normalized := mcpToolGrantName(1, item.Name)
		if _, exists := toolNames[normalized]; exists {
			return fmt.Errorf("%w: duplicate normalized tool name", ErrCapabilityDiscoveryInvalid)
		}
		toolNames[normalized] = struct{}{}
	}

	resourceURIs := make(map[string]struct{}, len(capabilities.Resources))
	for _, item := range capabilities.Resources {
		if item == nil || !validMCPDiscoveryURI(item.URI) {
			return fmt.Errorf("%w: invalid resource URI", ErrCapabilityDiscoveryInvalid)
		}
		if strings.TrimSpace(item.Name) != "" && !validMCPDiscoveryName(item.Name) {
			return fmt.Errorf("%w: invalid resource name", ErrCapabilityDiscoveryInvalid)
		}
		if runeLen(item.Description) > maxMCPDescriptionRunes || runeLen(item.MIMEType) > maxMCPMIMETypeRunes {
			return fmt.Errorf("%w: resource metadata is too long", ErrCapabilityDiscoveryInvalid)
		}
		uri := strings.TrimSpace(item.URI)
		if _, exists := resourceURIs[uri]; exists {
			return fmt.Errorf("%w: duplicate resource URI", ErrCapabilityDiscoveryInvalid)
		}
		resourceURIs[uri] = struct{}{}
	}

	promptNames := make(map[string]struct{}, len(capabilities.Prompts))
	for _, item := range capabilities.Prompts {
		if item == nil || !validMCPDiscoveryName(item.Name) {
			return fmt.Errorf("%w: invalid prompt name", ErrCapabilityDiscoveryInvalid)
		}
		if runeLen(item.Description) > maxMCPDescriptionRunes {
			return fmt.Errorf("%w: prompt description is too long", ErrCapabilityDiscoveryInvalid)
		}
		normalized := mcpToolGrantName(1, item.Name)
		if _, exists := promptNames[normalized]; exists {
			return fmt.Errorf("%w: duplicate normalized prompt name", ErrCapabilityDiscoveryInvalid)
		}
		promptNames[normalized] = struct{}{}
		argumentNames := make(map[string]struct{}, len(item.Arguments))
		for _, argument := range item.Arguments {
			if argument == nil || !validMCPDiscoveryName(argument.Name) || runeLen(argument.Description) > maxMCPDescriptionRunes {
				return fmt.Errorf("%w: invalid prompt argument", ErrCapabilityDiscoveryInvalid)
			}
			normalizedArgument := mcpToolGrantName(1, argument.Name)
			if _, exists := argumentNames[normalizedArgument]; exists {
				return fmt.Errorf("%w: duplicate normalized prompt argument", ErrCapabilityDiscoveryInvalid)
			}
			argumentNames[normalizedArgument] = struct{}{}
		}
	}

	return nil
}

func validMCPDiscoveryName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || runeLen(value) > maxMCPNameRunes {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}

	return true
}

func validMCPDiscoveryURI(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || runeLen(value) > maxMCPURIRunes {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.User != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "file", "object", "internal", "s3", "gs", "oss", "cos":
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	for key := range query {
		normalized := strings.ToLower(strings.TrimSpace(key))
		for _, marker := range []string{"token", "secret", "password", "credential", "signature", "api_key", "apikey", "auth"} {
			if strings.Contains(normalized, marker) {
				return false
			}
		}
	}

	return true
}

func validJSONObject(value string) bool {
	var payload map[string]json.RawMessage
	err := json.Unmarshal([]byte(strings.TrimSpace(value)), &payload)

	return err == nil && payload != nil
}

func runeLen(value string) int {
	return len([]rune(value))
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// Package mcpruntime owns transport-neutral MCP connection validation and
// bounded management sessions. It deliberately has no dependency on the
// agentthread or mcptool application packages.
package mcpruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ServerTypeStdio          = "stdio"
	ServerTypeSSE            = "sse"
	ServerTypeStreamableHTTP = "streamable_http"

	defaultDiscoveryMaxItems       = 500
	defaultDiscoveryMaxBytes       = 1 << 20
	defaultDiscoveryMaxPageBytes   = 256 << 10
	defaultDiscoveryMaxPages       = 128
	defaultDiscoveryMaxCursorBytes = 1024
)

var (
	ErrInvalidConnection      = errors.New("MCP connection is invalid")
	ErrPolicyDenied           = errors.New("MCP connection is denied by policy")
	ErrSessionUnavailable     = errors.New("MCP session is unavailable")
	ErrDiscoveryLimitExceeded = errors.New("MCP discovery limit exceeded")

	ErrStdioCommandDenied               = fmt.Errorf("%w: stdio command denied", ErrPolicyDenied)
	ErrStdioWorkingDirRequired          = fmt.Errorf("%w: stdio working directory required", ErrPolicyDenied)
	ErrStdioWorkingDirDenied            = fmt.Errorf("%w: stdio working directory denied", ErrPolicyDenied)
	ErrStdioArgsLimitExceeded           = fmt.Errorf("%w: stdio args limit exceeded", ErrPolicyDenied)
	ErrStdioEnvDenied                   = fmt.Errorf("%w: stdio environment denied", ErrPolicyDenied)
	ErrStdioEnvLimitExceeded            = fmt.Errorf("%w: stdio environment limit exceeded", ErrPolicyDenied)
	ErrStdioFrameLimitExceeded          = errors.New("MCP stdio response exceeds frame limit")
	ErrStdioHostExecutionDenied         = errors.New("MCP stdio host execution is denied")
	ErrStdioInboundRequestLimitExceeded = errors.New("MCP stdio inbound request budget exceeded")
)

type Connection struct {
	ServerType string
	Config     string
	Auth       string
}

type CapabilityFlags struct {
	Tools     bool
	Resources bool
	Prompts   bool
}

type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type Resource struct {
	URI         string
	Name        string
	Description string
	MIMEType    string
}

type PromptArgument struct {
	Name        string
	Description string
	Required    bool
}

type Prompt struct {
	Name        string
	Description string
	Arguments   []PromptArgument
}

type ToolPage struct {
	Items      []Tool
	NextCursor string
}

type ResourcePage struct {
	Items      []Resource
	NextCursor string
}

type PromptPage struct {
	Items      []Prompt
	NextCursor string
}

// ToolCallResult intentionally excludes MCP content and structuredContent.
// Management callers need only the protocol-level completion status.
type ToolCallResult struct {
	IsError bool
}

type Session interface {
	Capabilities() CapabilityFlags
	ListToolsByPage(ctx context.Context, cursor string) (ToolPage, error)
	ListResourcesByPage(ctx context.Context, cursor string) (ResourcePage, error)
	ListPromptsByPage(ctx context.Context, cursor string) (PromptPage, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (ToolCallResult, error)
	Close() error
}

type SessionFactory interface {
	Open(ctx context.Context, connection Connection) (Session, error)
}

type DiscoveredCapabilities struct {
	Tools     []Tool
	Resources []Resource
	Prompts   []Prompt
}

type DiscoveryLimits struct {
	MaxItems       int
	MaxBytes       int
	MaxPageBytes   int
	MaxPages       int
	MaxCursorBytes int
}

func DefaultDiscoveryLimits() DiscoveryLimits {
	return DiscoveryLimits{
		MaxItems:       defaultDiscoveryMaxItems,
		MaxBytes:       defaultDiscoveryMaxBytes,
		MaxPageBytes:   defaultDiscoveryMaxPageBytes,
		MaxPages:       defaultDiscoveryMaxPages,
		MaxCursorBytes: defaultDiscoveryMaxCursorBytes,
	}
}

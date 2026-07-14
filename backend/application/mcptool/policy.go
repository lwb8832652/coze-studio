// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import "errors"

// ErrMCPDisabled is returned by management and runtime service entries when
// MCP has not been explicitly enabled by the application startup policy.
var ErrMCPDisabled = errors.New("mcp management and runtime are disabled")

//go:build !darwin && !linux

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"os/exec"
	"time"
)

func configureStdioDebugProcessGroup(*exec.Cmd) error { return ErrStdioHostExecutionDenied }

func signalStdioDebugProcessGroup(int, bool) error { return ErrStdioHostExecutionDenied }

func inspectStdioDebugEscapedDescendants(int) ([]int, error) {
	return nil, ErrStdioHostExecutionDenied
}

func terminateStdioDebugEscapedProcesses([]int) {}

func waitStdioDebugProcessesGone([]int, time.Duration) bool { return false }

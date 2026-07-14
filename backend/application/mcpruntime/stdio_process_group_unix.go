//go:build darwin || linux

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureStdioDebugProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil {
		return ErrSessionUnavailable
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func signalStdioDebugProcessGroup(processID int, force bool) error {
	if processID <= 0 {
		return ErrSessionUnavailable
	}
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	err := syscall.Kill(-processID, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

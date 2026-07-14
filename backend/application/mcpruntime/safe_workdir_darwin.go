//go:build darwin

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import "golang.org/x/sys/unix"

func safeOpenWorkdirDirectoryAt(parentFD int, name string) (int, error) {
	return unix.Openat(
		parentFD,
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
}

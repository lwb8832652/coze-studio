//go:build linux

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

func safeOpenWorkdirDirectoryAt(parentFD int, name string) (int, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat2(parentFD, name, &unix.OpenHow{
		Flags: uint64(flags),
		Resolve: unix.RESOLVE_BENEATH |
			unix.RESOLVE_NO_SYMLINKS |
			unix.RESOLVE_NO_MAGICLINKS,
	})
	if err == nil {
		return fd, nil
	}
	if !errors.Is(err, syscall.ENOSYS) && !errors.Is(err, syscall.EINVAL) &&
		!errors.Is(err, syscall.E2BIG) && !errors.Is(err, syscall.EPERM) {
		return -1, err
	}
	return unix.Openat(parentFD, name, flags, 0)
}

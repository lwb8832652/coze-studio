// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type StdioExecutionMode struct {
	debugHost bool
}

type ProductionProcessTreeIsolation string

const (
	ProductionProcessTreeIsolationCgroup       ProductionProcessTreeIsolation = "cgroup"
	ProductionProcessTreeIsolationNamespace    ProductionProcessTreeIsolation = "namespace"
	ProductionProcessTreeIsolationDedicatedUID ProductionProcessTreeIsolation = "dedicated_uid"
)

type ProductionProcessTreeHandle interface {
	Isolation() ProductionProcessTreeIsolation
	ControlRootInaccessible(canonicalControlRoot string) bool
	TerminateAndWait(ctx context.Context) error
}

type ProductionStdioBuildResult struct {
	Client      ProtocolClient
	ProcessTree ProductionProcessTreeHandle
}

func NewStdioExecutionMode(appEnv string, debugHostEnabled bool) StdioExecutionMode {
	return StdioExecutionMode{
		debugHost: strings.EqualFold(strings.TrimSpace(appEnv), "debug") && debugHostEnabled,
	}
}

func (m StdioExecutionMode) AllowsHostExecution() bool {
	return m.debugHost
}

type ProductionStdioClientBuilder interface {
	TrustedProductionRunnerExecutable() string
	// BuildWithTrustedProductionRunner transfers ownership of every non-nil
	// resource in ProductionStdioBuildResult to the runtime, including when it
	// returns an error. A builder that has started any client or process tree
	// must return all corresponding handles and must not hide them behind the
	// error; the runtime is responsible for retryable termination and cleanup.
	BuildWithTrustedProductionRunner(
		ctx context.Context,
		canonicalRunnerExecutable string,
		connection ResolvedConnection,
	) (*ProductionStdioBuildResult, error)
}

func validProductionProcessTreeIsolation(value ProductionProcessTreeIsolation) bool {
	switch value {
	case ProductionProcessTreeIsolationCgroup,
		ProductionProcessTreeIsolationNamespace,
		ProductionProcessTreeIsolationDedicatedUID:
		return true
	default:
		return false
	}
}

func validateTrustedProductionExecutable(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed != path || !filepath.IsAbs(trimmed) {
		return "", ErrStdioHostExecutionDenied
	}
	cleanPath := filepath.Clean(trimmed)
	if cleanPath != trimmed {
		return "", ErrStdioHostExecutionDenied
	}
	realPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil || !filepath.IsAbs(realPath) || realPath != cleanPath {
		return "", ErrStdioHostExecutionDenied
	}
	serviceUID := uint32(os.Geteuid())
	current := realPath
	for {
		info, statErr := os.Stat(current)
		if statErr != nil || info.Mode().Perm()&0o022 != 0 {
			return "", ErrStdioHostExecutionDenied
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid == serviceUID {
			return "", ErrStdioHostExecutionDenied
		}
		if current == realPath && (!info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0) {
			return "", ErrStdioHostExecutionDenied
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return realPath, nil
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
)

const safeWorkdirTrashDirectory = ".coze-workdir-trash"

const (
	managementSafeWorkdirPrefix        = "coze-mcp-management-"
	safeWorkdirControlDirectory        = ".coze-workdir-control"
	safeWorkdirLeaseDirectory          = "leases"
	safeWorkdirRejectedDirectory       = "rejected"
	safeWorkdirRootLockName            = "root.lock"
	safeWorkdirIdentityMarkerVersion   = 1
	safeWorkdirRootLockVersion         = 1
	maximumSafeWorkdirIdentityBytes    = 1024
	defaultSafeWorkdirDeleteMaxEntries = 4096
	defaultSafeWorkdirDeleteMaxBytes   = int64(4 << 20)
	defaultSafeWorkdirDeleteMaxDepth   = 64
	minimumSafeWorkdirDeleteMaxBytes   = int64(512)
)

var (
	ErrSafeWorkdirInvalid          = errors.New("safe workdir configuration invalid")
	ErrSafeWorkdirUnavailable      = errors.New("safe workdir unavailable")
	ErrSafeWorkdirCleanupRetryable = errors.New("safe workdir cleanup retryable")
	ErrSafeWorkdirRecoveryRejected = errors.New("safe workdir recovery rejected")
	ErrSafeWorkdirRootLocked       = errors.New("safe workdir root already locked")
)

type safeWorkdirIdentityMarker struct {
	Version        int    `json:"version"`
	RelativePath   string `json:"relative_path"`
	Device         uint64 `json:"device"`
	Inode          uint64 `json:"inode"`
	InvocationID   string `json:"invocation_id"`
	InstanceID     string `json:"instance_id"`
	LockGeneration string `json:"lock_generation"`
}

type SafeWorkdirRecoveryReport struct {
	Scanned   int
	Recovered int
	Rejected  int
	Pending   int
}

type SafeWorkdirDeleteLimits struct {
	MaxEntries int
	MaxBytes   int64
	MaxDepth   int
}

type SafeWorkdirOptions struct {
	Root         string
	DeleteLimits SafeWorkdirDeleteLimits
}

type SafeWorkdirCreateRequest struct {
	Parent string
	Prefix string
}

type SafeWorkdir struct {
	Root         string
	RelativePath string
	Path         string
	InvocationID string
}

type SafeWorkdirManager struct {
	root     string
	limits   SafeWorkdirDeleteLimits
	platform *safeWorkdirPlatform
	mu       sync.RWMutex
	deleteMu sync.Mutex
	closed   bool
}

func NewSafeWorkdirManager(options SafeWorkdirOptions) (*SafeWorkdirManager, error) {
	root := filepath.Clean(strings.TrimSpace(options.Root))
	if root == "." || !filepath.IsAbs(root) {
		return nil, ErrSafeWorkdirInvalid
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || filepath.Clean(canonical) != root {
		return nil, ErrSafeWorkdirInvalid
	}
	limits, ok := normalizeSafeWorkdirDeleteLimits(options.DeleteLimits)
	if !ok {
		return nil, ErrSafeWorkdirInvalid
	}
	platform, err := newSafeWorkdirPlatform(root)
	if err != nil {
		if errors.Is(err, ErrSafeWorkdirRootLocked) {
			return nil, ErrSafeWorkdirRootLocked
		}
		return nil, ErrSafeWorkdirInvalid
	}
	return &SafeWorkdirManager{root: root, limits: limits, platform: platform}, nil
}

func (m *SafeWorkdirManager) Root() string {
	if m == nil {
		return ""
	}
	return m.root
}

func (m *SafeWorkdirManager) ControlRoot() string {
	if m == nil {
		return ""
	}
	return filepath.Join(m.root, safeWorkdirControlDirectory)
}

func (m *SafeWorkdirManager) HasExclusiveRootLock() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return !m.closed && m.platform != nil && m.platform.hasExclusiveRootLock()
}

func (m *SafeWorkdirManager) Create(
	ctx context.Context,
	request SafeWorkdirCreateRequest,
) (SafeWorkdir, error) {
	if m == nil || ctx == nil || ctx.Err() != nil || !validSafeWorkdirPrefix(request.Prefix) {
		return SafeWorkdir{}, ErrSafeWorkdirUnavailable
	}
	parent, ok := safeWorkdirRelativeComponents(request.Parent, true)
	if !ok {
		return SafeWorkdir{}, ErrSafeWorkdirUnavailable
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.platform == nil {
		return SafeWorkdir{}, ErrSafeWorkdirUnavailable
	}
	relative, invocationID, err := m.platform.create(ctx, parent, request.Prefix)
	if err != nil {
		return SafeWorkdir{}, ErrSafeWorkdirUnavailable
	}
	if !m.platform.matchesRootPath(m.root) {
		_ = m.platform.delete(context.Background(), relative, m.limits)
		return SafeWorkdir{}, ErrSafeWorkdirUnavailable
	}
	return SafeWorkdir{
		Root:         m.root,
		RelativePath: relative,
		Path:         filepath.Join(m.root, relative),
		InvocationID: invocationID,
	}, nil
}

func (m *SafeWorkdirManager) Delete(ctx context.Context, relative string) error {
	if m == nil || ctx == nil || ctx.Err() != nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	components, ok := safeWorkdirRelativeComponents(relative, false)
	if !ok || len(components) == 0 || components[0] == safeWorkdirTrashDirectory {
		return ErrSafeWorkdirInvalid
	}
	relative = filepath.Join(components...)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.platform == nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	m.deleteMu.Lock()
	defer m.deleteMu.Unlock()
	if err := m.platform.delete(ctx, relative, m.limits); err != nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	return nil
}

func (m *SafeWorkdirManager) CleanupQuarantine(ctx context.Context) error {
	if m == nil || ctx == nil || ctx.Err() != nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.platform == nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	m.deleteMu.Lock()
	defer m.deleteMu.Unlock()
	if err := m.platform.cleanupQuarantine(ctx, m.limits); err != nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	return nil
}

func (m *SafeWorkdirManager) RecoverDirectChildren(
	ctx context.Context,
	prefix string,
) (SafeWorkdirRecoveryReport, error) {
	if m == nil || ctx == nil || ctx.Err() != nil || !validSafeWorkdirPrefix(prefix) {
		return SafeWorkdirRecoveryReport{}, ErrSafeWorkdirRecoveryRejected
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.platform == nil || !m.platform.matchesRootPath(m.root) {
		return SafeWorkdirRecoveryReport{}, ErrSafeWorkdirCleanupRetryable
	}
	m.deleteMu.Lock()
	defer m.deleteMu.Unlock()
	report, err := m.platform.recoverDirectChildren(ctx, m.root, prefix, m.limits)
	if errors.Is(err, ErrSafeWorkdirRecoveryRejected) {
		return report, ErrSafeWorkdirRecoveryRejected
	}
	if err != nil {
		return report, ErrSafeWorkdirCleanupRetryable
	}
	return report, nil
}

func (m *SafeWorkdirManager) RelativePath(path string) (string, error) {
	if m == nil {
		return "", ErrSafeWorkdirInvalid
	}
	clean := filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(clean) {
		return "", ErrSafeWorkdirInvalid
	}
	relative, err := filepath.Rel(m.root, clean)
	if err != nil {
		return "", ErrSafeWorkdirInvalid
	}
	components, ok := safeWorkdirRelativeComponents(relative, false)
	if !ok || len(components) == 0 {
		return "", ErrSafeWorkdirInvalid
	}
	return filepath.Join(components...), nil
}

func (m *SafeWorkdirManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.platform != nil {
		return m.platform.close()
	}
	return nil
}

func normalizeSafeWorkdirDeleteLimits(
	limits SafeWorkdirDeleteLimits,
) (SafeWorkdirDeleteLimits, bool) {
	if limits.MaxEntries < 0 || limits.MaxBytes < 0 || limits.MaxDepth < 0 {
		return SafeWorkdirDeleteLimits{}, false
	}
	if limits.MaxEntries == 0 {
		limits.MaxEntries = defaultSafeWorkdirDeleteMaxEntries
	}
	if limits.MaxBytes == 0 {
		limits.MaxBytes = defaultSafeWorkdirDeleteMaxBytes
	}
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaultSafeWorkdirDeleteMaxDepth
	}
	return limits, limits.MaxEntries > 0 && limits.MaxBytes >= minimumSafeWorkdirDeleteMaxBytes &&
		limits.MaxDepth > 0
}

func validSafeWorkdirPrefix(prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	return prefix != "" && len(prefix) <= 64 && prefix != "." && prefix != ".." &&
		filepath.Base(prefix) == prefix && !strings.ContainsRune(prefix, 0)
}

func safeWorkdirRelativeComponents(value string, allowEmpty bool) ([]string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, allowEmpty
	}
	if filepath.IsAbs(value) || filepath.Clean(value) != value || value == "." {
		return nil, false
	}
	components := strings.Split(value, string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." || component == ".." ||
			len(component) > 255 || strings.ContainsRune(component, 0) {
			return nil, false
		}
	}
	return components, true
}

/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

const defaultADKMCPRuntimeStdioWorkdirMode os.FileMode = 0o700

const (
	defaultADKMCPRuntimeStdioWorkdirCleanupTimeout = 10 * time.Second
	defaultADKMCPRuntimeStdioWorkdirCleanupMargin  = 30 * time.Second
	adkMCPRuntimeStdioInvocationDirPrefix          = "invocation-"
)

type adkMCPRuntimeStdioWorkdirCleanupState struct {
	once sync.Once
	err  error
}

type ADKMCPRuntimeStdioWorkdirPreparer interface {
	PrepareADKMCPRuntimeStdioWorkdir(
		ctx context.Context,
		execution ADKMCPRuntimeStdioSandboxExecution,
	) (ADKMCPRuntimeStdioPreparedWorkdir, error)
	CleanupADKMCPRuntimeStdioWorkdir(
		ctx context.Context,
		prepared ADKMCPRuntimeStdioPreparedWorkdir,
	) error
}

type ADKMCPRuntimeStdioPreparedWorkdir struct {
	Root          string
	WorkingDir    string
	LeaseID       int64
	LeaseWorkerID string

	relativePath      string
	cleanupState      *adkMCPRuntimeStdioWorkdirCleanupState
	leaseCleanupState *adkMCPRuntimeStdioWorkdirCleanupState
}

type ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions struct {
	Root           string
	DirMode        os.FileMode
	CleanupTimeout time.Duration
	DeleteLimits   mcpruntime.SafeWorkdirDeleteLimits
	Manager        *mcpruntime.SafeWorkdirManager
}

type ADKMCPRuntimeStdioFilesystemWorkdirPreparer struct {
	root           string
	manager        *mcpruntime.SafeWorkdirManager
	initErr        error
	cleanupTimeout time.Duration
}

func NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
	options ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions,
) *ADKMCPRuntimeStdioFilesystemWorkdirPreparer {
	cleanupTimeout := options.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultADKMCPRuntimeStdioWorkdirCleanupTimeout
	}
	root := filepath.Clean(strings.TrimSpace(options.Root))
	manager := options.Manager
	ownsManager := manager == nil
	var err error
	if manager == nil {
		manager, err = mcpruntime.NewSafeWorkdirManager(mcpruntime.SafeWorkdirOptions{
			Root:         root,
			DeleteLimits: options.DeleteLimits,
		})
	} else if !manager.HasExclusiveRootLock() || manager.Root() != root {
		err = mcpruntime.ErrSafeWorkdirInvalid
		manager = nil
	}
	if options.DirMode != 0 && options.DirMode.Perm() != defaultADKMCPRuntimeStdioWorkdirMode {
		err = mcpruntime.ErrSafeWorkdirInvalid
		if manager != nil && ownsManager {
			_ = manager.Close()
		}
		manager = nil
	}
	if manager != nil {
		root = manager.Root()
	}
	return &ADKMCPRuntimeStdioFilesystemWorkdirPreparer{
		root: root, manager: manager, initErr: err, cleanupTimeout: cleanupTimeout,
	}
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) PrepareADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioPreparedWorkdir, error) {
	if p == nil || p.initErr != nil || p.manager == nil || ctx == nil || ctx.Err() != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	projection, err := NewADKMCPRuntimeStdioWorkdirManager(
		ADKMCPRuntimeStdioWorkdirManagerOptions{Root: p.root},
	).ProjectADKMCPRuntimeStdioWorkdir(ctx, ADKMCPRuntimeStdioWorkdirRequest{
		Run: execution.Run, Name: execution.Name,
		ServerID: execution.ServerID, ToolName: execution.ToolName,
	})
	if err != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	parent, err := p.manager.RelativePath(projection.WorkingDir)
	if err != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	workdir, err := p.manager.Create(ctx, mcpruntime.SafeWorkdirCreateRequest{
		Parent: parent,
		Prefix: adkMCPRuntimeStdioInvocationDirPrefix,
	})
	if err != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	return ADKMCPRuntimeStdioPreparedWorkdir{
		Root: p.root, WorkingDir: workdir.Path, relativePath: workdir.RelativePath,
		cleanupState: &adkMCPRuntimeStdioWorkdirCleanupState{},
	}, nil
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) CleanupADKMCPRuntimeStdioWorkdir(
	_ context.Context,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) error {
	if p == nil || p.initErr != nil || p.manager == nil {
		return errors.New("mcp runtime stdio workdir cleanup failed")
	}
	state := prepared.cleanupState
	if state == nil {
		state = &adkMCPRuntimeStdioWorkdirCleanupState{}
	}
	state.once.Do(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), p.cleanupTimeout)
		defer cancel()
		if filepath.Clean(strings.TrimSpace(prepared.Root)) != p.root {
			state.err = errors.New("mcp runtime stdio workdir cleanup failed")
			return
		}
		relative := prepared.relativePath
		if relative == "" {
			var err error
			relative, err = p.manager.RelativePath(prepared.WorkingDir)
			if err != nil {
				state.err = errors.New("mcp runtime stdio workdir cleanup failed")
				return
			}
		}
		if !strings.HasPrefix(filepath.Base(relative), adkMCPRuntimeStdioInvocationDirPrefix) ||
			p.manager.Delete(cleanupCtx, relative) != nil {
			state.err = errors.New("mcp runtime stdio workdir cleanup failed")
		}
	})
	return state.err
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) Valid() bool {
	return p != nil && p.manager != nil && p.initErr == nil
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) Root() string {
	if p == nil {
		return ""
	}
	return p.root
}

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
)

const defaultADKMCPRuntimeStdioWorkdirMode os.FileMode = 0o700

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
}

type ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions struct {
	Root    string
	DirMode os.FileMode
}

type ADKMCPRuntimeStdioFilesystemWorkdirPreparer struct {
	root    string
	dirMode os.FileMode
}

func NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
	options ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions,
) *ADKMCPRuntimeStdioFilesystemWorkdirPreparer {
	dirMode := options.DirMode
	if dirMode == 0 {
		dirMode = defaultADKMCPRuntimeStdioWorkdirMode
	}

	return &ADKMCPRuntimeStdioFilesystemWorkdirPreparer{
		root:    filepath.Clean(strings.TrimSpace(options.Root)),
		dirMode: dirMode.Perm(),
	}
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) PrepareADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioPreparedWorkdir, error) {
	root, workingDir, ok := p.cleanAndValidate(execution.WorkingDir)
	if !ok {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	if err := os.MkdirAll(workingDir, p.dirMode); err != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	if err := os.Chmod(workingDir, p.dirMode); err != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}
	info, err := os.Stat(workingDir)
	if err != nil || !info.IsDir() {
		return ADKMCPRuntimeStdioPreparedWorkdir{},
			errors.New("mcp runtime stdio workdir prepare failed")
	}

	return ADKMCPRuntimeStdioPreparedWorkdir{
		Root:       root,
		WorkingDir: workingDir,
	}, nil
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) CleanupADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) error {
	root, workingDir, ok := p.cleanPrepared(prepared)
	if !ok || root == workingDir {
		return errors.New("mcp runtime stdio workdir cleanup failed")
	}
	if err := os.RemoveAll(workingDir); err != nil {
		return errors.New("mcp runtime stdio workdir cleanup failed")
	}

	return nil
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) cleanAndValidate(
	workingDir string,
) (string, string, bool) {
	if p == nil || !filepath.IsAbs(p.root) {
		return "", "", false
	}
	root := filepath.Clean(p.root)
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" || !filepath.IsAbs(workingDir) {
		return "", "", false
	}
	cleanWorkingDir := filepath.Clean(workingDir)
	if cleanWorkingDir == root || !adkMCPRuntimePathWithin(cleanWorkingDir, root) {
		return "", "", false
	}

	return root, cleanWorkingDir, true
}

func (p *ADKMCPRuntimeStdioFilesystemWorkdirPreparer) cleanPrepared(
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) (string, string, bool) {
	if p == nil || !filepath.IsAbs(p.root) {
		return "", "", false
	}
	root := filepath.Clean(strings.TrimSpace(prepared.Root))
	if root == "" || root != filepath.Clean(p.root) {
		return "", "", false
	}
	workingDir := strings.TrimSpace(prepared.WorkingDir)
	if workingDir == "" || !filepath.IsAbs(workingDir) {
		return "", "", false
	}
	cleanWorkingDir := filepath.Clean(workingDir)
	if !adkMCPRuntimePathWithin(cleanWorkingDir, root) {
		return "", "", false
	}

	return root, cleanWorkingDir, true
}

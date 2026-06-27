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
	"fmt"
	"path/filepath"
	"strings"
)

type ADKMCPRuntimeStdioWorkdirManagerOptions struct {
	Root string
}

type ADKMCPRuntimeStdioWorkdirManager struct {
	root string
}

type ADKMCPRuntimeStdioWorkdirProjector interface {
	ProjectADKMCPRuntimeStdioWorkdir(
		ctx context.Context,
		request ADKMCPRuntimeStdioWorkdirRequest,
	) (ADKMCPRuntimeStdioWorkdirProjection, error)
}

type ADKMCPRuntimeStdioWorkdirProjectorFunc func(
	ctx context.Context,
	request ADKMCPRuntimeStdioWorkdirRequest,
) (ADKMCPRuntimeStdioWorkdirProjection, error)

func (f ADKMCPRuntimeStdioWorkdirProjectorFunc) ProjectADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	request ADKMCPRuntimeStdioWorkdirRequest,
) (ADKMCPRuntimeStdioWorkdirProjection, error) {
	if f == nil {
		return ADKMCPRuntimeStdioWorkdirProjection{},
			errors.New("mcp runtime stdio workdir is invalid")
	}

	return f(ctx, request)
}

type ADKMCPRuntimeStdioWorkdirRequest struct {
	Run      *RunSummary
	Name     string
	ServerID int64
	ToolName string
}

type ADKMCPRuntimeStdioWorkdirProjection struct {
	Root       string
	WorkingDir string
}

func NewADKMCPRuntimeStdioWorkdirManager(
	options ADKMCPRuntimeStdioWorkdirManagerOptions,
) *ADKMCPRuntimeStdioWorkdirManager {
	return &ADKMCPRuntimeStdioWorkdirManager{
		root: filepath.Clean(strings.TrimSpace(options.Root)),
	}
}

func (m *ADKMCPRuntimeStdioWorkdirManager) ProjectADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	request ADKMCPRuntimeStdioWorkdirRequest,
) (ADKMCPRuntimeStdioWorkdirProjection, error) {
	if m == nil ||
		!filepath.IsAbs(m.root) ||
		!validADKMCPRuntimeStdioWorkdirRequest(request) {
		return ADKMCPRuntimeStdioWorkdirProjection{},
			errors.New("mcp runtime stdio workdir is invalid")
	}

	workingDir := filepath.Join(
		m.root,
		"spaces",
		fmt.Sprintf("%d", request.Run.SpaceID),
		"threads",
		fmt.Sprintf("%d", request.Run.ThreadID),
		"runs",
		fmt.Sprintf("%d", request.Run.RunID),
		"servers",
		fmt.Sprintf("%d", request.ServerID),
		"tools",
		strings.TrimSpace(request.Name),
	)
	if !adkMCPRuntimePathWithin(workingDir, m.root) {
		return ADKMCPRuntimeStdioWorkdirProjection{},
			errors.New("mcp runtime stdio workdir is invalid")
	}

	return ADKMCPRuntimeStdioWorkdirProjection{
		Root:       m.root,
		WorkingDir: workingDir,
	}, nil
}

func validADKMCPRuntimeStdioWorkdirRequest(
	request ADKMCPRuntimeStdioWorkdirRequest,
) bool {
	return request.Run != nil &&
		request.Run.RunID > 0 &&
		request.Run.ThreadID > 0 &&
		request.Run.SpaceID > 0 &&
		request.ServerID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(request.Name)) &&
		strings.TrimSpace(request.ToolName) != ""
}

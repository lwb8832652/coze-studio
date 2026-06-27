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
	"path/filepath"
	"strings"
)

type ADKMCPRuntimeStdioSandboxOptions struct {
	Runner          ADKMCPRuntimeStdioSandboxRunner
	WorkdirPreparer ADKMCPRuntimeStdioWorkdirPreparer
}

type ADKMCPRuntimeStdioSandboxAdapter struct {
	runner          ADKMCPRuntimeStdioSandboxRunner
	workdirPreparer ADKMCPRuntimeStdioWorkdirPreparer
}

type ADKMCPRuntimeStdioSandboxRunner interface {
	RunADKMCPRuntimeStdio(
		ctx context.Context,
		execution ADKMCPRuntimeStdioSandboxExecution,
	) (string, error)
}

type ADKMCPRuntimeStdioSandboxRunnerFunc func(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (string, error)

func (f ADKMCPRuntimeStdioSandboxRunnerFunc) RunADKMCPRuntimeStdio(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (string, error) {
	if f == nil {
		return "", errors.New("mcp runtime stdio runner is not configured")
	}

	return f(ctx, execution)
}

type ADKMCPRuntimeStdioSandboxExecution struct {
	Run        *RunSummary
	Name       string
	ServerID   int64
	ToolName   string
	Arguments  string
	Command    string
	Args       []string
	Env        map[string]string
	WorkingDir string
}

func NewADKMCPRuntimeStdioSandbox(
	options ADKMCPRuntimeStdioSandboxOptions,
) *ADKMCPRuntimeStdioSandboxAdapter {
	return &ADKMCPRuntimeStdioSandboxAdapter{
		runner:          options.Runner,
		workdirPreparer: options.WorkdirPreparer,
	}
}

func (s *ADKMCPRuntimeStdioSandboxAdapter) InvokeADKMCPRuntimeStdio(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) (string, error) {
	execution, err := projectADKMCPRuntimeStdioSandboxExecution(call)
	if err != nil {
		return "", err
	}
	var prepared ADKMCPRuntimeStdioPreparedWorkdir
	if s != nil && s.workdirPreparer != nil {
		prepared, err = s.workdirPreparer.PrepareADKMCPRuntimeStdioWorkdir(ctx, execution)
		if err != nil {
			return "", errors.New("mcp runtime stdio workdir prepare failed")
		}
		execution.WorkingDir = prepared.WorkingDir
	}
	if s == nil || s.runner == nil {
		return "", errors.New("mcp runtime stdio runner is not configured")
	}

	result, err := s.runner.RunADKMCPRuntimeStdio(ctx, execution)
	if s != nil && s.workdirPreparer != nil {
		cleanupErr := s.workdirPreparer.CleanupADKMCPRuntimeStdioWorkdir(ctx, prepared)
		if err == nil && cleanupErr != nil {
			return "", errors.New("mcp runtime stdio workdir cleanup failed")
		}
	}
	if err != nil {
		return "", errors.New("mcp runtime stdio runner failed")
	}

	return result, nil
}

func projectADKMCPRuntimeStdioSandboxExecution(
	call ADKMCPRuntimeStdioSandboxCall,
) (ADKMCPRuntimeStdioSandboxExecution, error) {
	if err := validateADKMCPRuntimeStdioSandboxCall(call); err != nil {
		return ADKMCPRuntimeStdioSandboxExecution{}, err
	}

	return ADKMCPRuntimeStdioSandboxExecution{
		Run:        call.Run,
		Name:       strings.TrimSpace(call.Name),
		ServerID:   call.ServerID,
		ToolName:   strings.TrimSpace(call.ToolName),
		Arguments:  strings.TrimSpace(call.Arguments),
		Command:    strings.TrimSpace(call.Config.Command),
		Args:       append([]string(nil), call.Config.Args...),
		Env:        cloneADKMCPRuntimeStringMap(call.Config.Env),
		WorkingDir: filepath.Clean(strings.TrimSpace(call.Config.WorkingDir)),
	}, nil
}

func validateADKMCPRuntimeStdioSandboxCall(
	call ADKMCPRuntimeStdioSandboxCall,
) error {
	if call.Run == nil ||
		call.Run.RunID <= 0 ||
		call.Run.ThreadID <= 0 ||
		call.Run.SpaceID <= 0 ||
		call.ServerID <= 0 ||
		!isADKSubagentToolName(strings.TrimSpace(call.Name)) ||
		strings.TrimSpace(call.ToolName) == "" ||
		!validADKMCPRuntimeArguments(strings.TrimSpace(call.Arguments)) ||
		strings.TrimSpace(call.Config.Command) == "" ||
		!filepath.IsAbs(strings.TrimSpace(call.Config.WorkingDir)) {
		return errors.New("mcp runtime stdio sandbox call is invalid")
	}

	return nil
}

func cloneADKMCPRuntimeStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}

	return clone
}

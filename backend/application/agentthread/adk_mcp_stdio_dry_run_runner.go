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
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

const defaultADKMCPRuntimeStdioDryRunMaxOutputBytes = 4 << 10

type ADKMCPRuntimeStdioDryRunRunnerOptions struct {
	MaxOutputBytes int
}

type ADKMCPRuntimeStdioDryRunRunner struct {
	maxOutputBytes int
}

type adkMCPRuntimeStdioDryRunResult struct {
	Schema               string `json:"schema"`
	Status               string `json:"status"`
	RuntimeToolName      string `json:"runtime_tool_name"`
	ServerID             int64  `json:"server_id"`
	CommandConfigured    bool   `json:"command_configured"`
	ArgsCount            int    `json:"args_count"`
	EnvCount             int    `json:"env_count"`
	WorkingDirConfigured bool   `json:"working_dir_configured"`
}

func NewADKMCPRuntimeStdioDryRunRunner(
	options ADKMCPRuntimeStdioDryRunRunnerOptions,
) *ADKMCPRuntimeStdioDryRunRunner {
	maxOutputBytes := options.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultADKMCPRuntimeStdioDryRunMaxOutputBytes
	}

	return &ADKMCPRuntimeStdioDryRunRunner{maxOutputBytes: maxOutputBytes}
}

func (r *ADKMCPRuntimeStdioDryRunRunner) RunADKMCPRuntimeStdio(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (string, error) {
	if !validADKMCPRuntimeStdioDryRunExecution(execution) {
		return "", errors.New("mcp runtime stdio dry-run execution is invalid")
	}

	payload := adkMCPRuntimeStdioDryRunResult{
		Schema:               "coze.mcp_stdio_dry_run.v1",
		Status:               "dry_run",
		RuntimeToolName:      strings.TrimSpace(execution.Name),
		ServerID:             execution.ServerID,
		CommandConfigured:    strings.TrimSpace(execution.Command) != "",
		ArgsCount:            len(execution.Args),
		EnvCount:             len(execution.Env),
		WorkingDirConfigured: strings.TrimSpace(execution.WorkingDir) != "",
	}
	result, err := json.Marshal(payload)
	if err != nil {
		return "", errors.New("mcp runtime stdio dry-run result is invalid")
	}
	if len(result) > r.outputByteLimit() {
		return "", errors.New("mcp runtime stdio dry-run output exceeds budget")
	}

	return string(result), nil
}

func (r *ADKMCPRuntimeStdioDryRunRunner) outputByteLimit() int {
	if r == nil || r.maxOutputBytes <= 0 {
		return defaultADKMCPRuntimeStdioDryRunMaxOutputBytes
	}

	return r.maxOutputBytes
}

func validADKMCPRuntimeStdioDryRunExecution(
	execution ADKMCPRuntimeStdioSandboxExecution,
) bool {
	return execution.Run != nil &&
		execution.Run.RunID > 0 &&
		execution.Run.ThreadID > 0 &&
		execution.Run.SpaceID > 0 &&
		execution.ServerID > 0 &&
		isADKSubagentToolName(strings.TrimSpace(execution.Name)) &&
		strings.TrimSpace(execution.ToolName) != "" &&
		validADKMCPRuntimeArguments(strings.TrimSpace(execution.Arguments)) &&
		strings.TrimSpace(execution.Command) != "" &&
		filepath.IsAbs(strings.TrimSpace(execution.WorkingDir))
}

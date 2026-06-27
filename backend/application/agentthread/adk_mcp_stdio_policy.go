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

type ADKMCPRuntimeStdioStaticPolicyOptions struct {
	AllowedCommands           []string
	AllowedWorkingDirPrefixes []string
	AllowedEnvKeys            []string
	MaxArgs                   int
	MaxArgBytes               int
	MaxEnvVars                int
	MaxEnvValueBytes          int
	RequireWorkingDir         bool
}

type ADKMCPRuntimeStdioStaticPolicy struct {
	allowedCommands           map[string]struct{}
	allowedWorkingDirPrefixes []string
	allowedEnvKeys            map[string]struct{}
	maxArgs                   int
	maxArgBytes               int
	maxEnvVars                int
	maxEnvValueBytes          int
	requireWorkingDir         bool
}

func NewADKMCPRuntimeStdioStaticPolicy(
	options ADKMCPRuntimeStdioStaticPolicyOptions,
) *ADKMCPRuntimeStdioStaticPolicy {
	return &ADKMCPRuntimeStdioStaticPolicy{
		allowedCommands:           adkMCPRuntimeStringSet(options.AllowedCommands),
		allowedWorkingDirPrefixes: adkMCPRuntimeCleanAbsPrefixes(options.AllowedWorkingDirPrefixes),
		allowedEnvKeys:            adkMCPRuntimeStringSet(options.AllowedEnvKeys),
		maxArgs:                   options.MaxArgs,
		maxArgBytes:               options.MaxArgBytes,
		maxEnvVars:                options.MaxEnvVars,
		maxEnvValueBytes:          options.MaxEnvValueBytes,
		requireWorkingDir:         options.RequireWorkingDir,
	}
}

func (p *ADKMCPRuntimeStdioStaticPolicy) ValidateADKMCPRuntimeStdio(
	ctx context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) error {
	if p == nil {
		return errors.New("mcp runtime stdio command is not allowed")
	}
	if err := p.validateCommand(call.Config.Command); err != nil {
		return err
	}
	if err := p.validateWorkingDir(call.Config.WorkingDir); err != nil {
		return err
	}
	if err := p.validateArgs(call.Config.Args); err != nil {
		return err
	}
	if err := p.validateEnv(call.Config.Env); err != nil {
		return err
	}

	return nil
}

func (p *ADKMCPRuntimeStdioStaticPolicy) validateCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" || len(p.allowedCommands) == 0 {
		return errors.New("mcp runtime stdio command is not allowed")
	}
	if _, ok := p.allowedCommands[command]; !ok {
		return errors.New("mcp runtime stdio command is not allowed")
	}

	return nil
}

func (p *ADKMCPRuntimeStdioStaticPolicy) validateWorkingDir(
	workingDir string,
) error {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		if p.requireWorkingDir {
			return errors.New("mcp runtime stdio working directory is required")
		}

		return nil
	}
	if len(p.allowedWorkingDirPrefixes) == 0 {
		return errors.New("mcp runtime stdio working directory is not allowed")
	}
	if !filepath.IsAbs(workingDir) {
		return errors.New("mcp runtime stdio working directory is not allowed")
	}
	cleanWorkingDir := filepath.Clean(workingDir)
	for _, prefix := range p.allowedWorkingDirPrefixes {
		if adkMCPRuntimePathWithin(cleanWorkingDir, prefix) {
			return nil
		}
	}

	return errors.New("mcp runtime stdio working directory is not allowed")
}

func (p *ADKMCPRuntimeStdioStaticPolicy) validateArgs(args []string) error {
	if p.maxArgs >= 0 && len(args) > p.maxArgs {
		return errors.New("mcp runtime stdio args exceed budget")
	}
	if p.maxArgBytes >= 0 {
		total := 0
		for _, arg := range args {
			total += len([]byte(arg))
			if total > p.maxArgBytes {
				return errors.New("mcp runtime stdio args exceed budget")
			}
		}
	}

	return nil
}

func (p *ADKMCPRuntimeStdioStaticPolicy) validateEnv(
	env map[string]string,
) error {
	for key, value := range env {
		if _, ok := p.allowedEnvKeys[key]; !ok {
			return errors.New("mcp runtime stdio env is not allowed")
		}
		if p.maxEnvValueBytes >= 0 && len([]byte(value)) > p.maxEnvValueBytes {
			return errors.New("mcp runtime stdio env exceeds budget")
		}
	}
	if p.maxEnvVars >= 0 && len(env) > p.maxEnvVars {
		return errors.New("mcp runtime stdio env exceeds budget")
	}

	return nil
}

func adkMCPRuntimeStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}

	return set
}

func adkMCPRuntimeCleanAbsPrefixes(values []string) []string {
	prefixes := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !filepath.IsAbs(value) {
			continue
		}
		prefixes = append(prefixes, filepath.Clean(value))
	}

	return prefixes
}

func adkMCPRuntimePathWithin(target string, prefix string) bool {
	if target == prefix {
		return true
	}
	rel, err := filepath.Rel(prefix, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}

	return rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}

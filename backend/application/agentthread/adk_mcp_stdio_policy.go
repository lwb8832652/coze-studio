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

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

type ADKMCPRuntimeStdioStaticPolicyOptions struct {
	AllowedCommands           []string
	AllowedNpxPackages        []string
	AllowedUVXPackages        []string
	AllowedNodeScriptRoots    []string
	CommandRules              []mcpruntime.StdioCommandRule
	CommandPolicy             *mcpruntime.StdioCommandPolicy
	AllowedWorkingDirPrefixes []string
	AllowedEnvKeys            []string
	MaxArgs                   int
	MaxArgBytes               int
	MaxEnvVars                int
	MaxEnvValueBytes          int
	RequireWorkingDir         bool
}

type ADKMCPRuntimeStdioStaticPolicy struct {
	policy mcpruntime.Policy
}

func NewADKMCPRuntimeStdioStaticPolicy(
	options ADKMCPRuntimeStdioStaticPolicyOptions,
) *ADKMCPRuntimeStdioStaticPolicy {
	return &ADKMCPRuntimeStdioStaticPolicy{
		policy: mcpruntime.Policy{
			StdioEnabled:                   true,
			StdioAllowedCommands:           append([]string(nil), options.AllowedCommands...),
			StdioAllowedNpxPackages:        append([]string(nil), options.AllowedNpxPackages...),
			StdioAllowedUVXPackages:        append([]string(nil), options.AllowedUVXPackages...),
			StdioAllowedNodeScriptRoots:    append([]string(nil), options.AllowedNodeScriptRoots...),
			StdioCommandRules:              append([]mcpruntime.StdioCommandRule(nil), options.CommandRules...),
			StdioCommandPolicy:             options.CommandPolicy,
			StdioAllowedWorkingDirPrefixes: append([]string(nil), options.AllowedWorkingDirPrefixes...),
			StdioAllowedEnvKeys:            append([]string(nil), options.AllowedEnvKeys...),
			StdioMaxArgs:                   options.MaxArgs,
			StdioMaxArgBytes:               options.MaxArgBytes,
			StdioMaxEnvVars:                options.MaxEnvVars,
			StdioMaxEnvValueBytes:          options.MaxEnvValueBytes,
			RequireWorkingDir:              options.RequireWorkingDir,
		},
	}
}

func (p *ADKMCPRuntimeStdioStaticPolicy) ValidateADKMCPRuntimeStdio(
	_ context.Context,
	call ADKMCPRuntimeStdioSandboxCall,
) error {
	if p == nil {
		return errors.New("mcp runtime stdio command is not allowed")
	}
	err := p.policy.ValidateStdio(mcpruntime.StdioConfig{
		Command:    call.Config.Command,
		Args:       append([]string(nil), call.Config.Args...),
		Env:        cloneADKMCPRuntimeStringMap(call.Config.Env),
		WorkingDir: call.Config.WorkingDir,
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mcpruntime.ErrStdioCommandDenied):
		return errors.New("mcp runtime stdio command is not allowed")
	case errors.Is(err, mcpruntime.ErrStdioWorkingDirRequired):
		return errors.New("mcp runtime stdio working directory is required")
	case errors.Is(err, mcpruntime.ErrStdioWorkingDirDenied):
		return errors.New("mcp runtime stdio working directory is not allowed")
	case errors.Is(err, mcpruntime.ErrStdioArgsLimitExceeded):
		return errors.New("mcp runtime stdio args exceed budget")
	case errors.Is(err, mcpruntime.ErrStdioEnvLimitExceeded):
		return errors.New("mcp runtime stdio env exceeds budget")
	default:
		return errors.New("mcp runtime stdio env is not allowed")
	}
}

func adkMCPRuntimeStringSet(values []string) map[string]struct{} {
	return mcpruntime.StringSet(values)
}

func adkMCPRuntimePathWithin(target, prefix string) bool {
	return mcpruntime.PathWithin(target, prefix)
}

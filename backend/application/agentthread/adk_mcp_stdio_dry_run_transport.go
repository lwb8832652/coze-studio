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
	"os"
	"path/filepath"
	"strings"

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type ADKMCPRuntimeStdioDryRunTransportOptions struct {
	WorkdirRoot       string
	LeaseRepository   domainrepo.MCPRuntimeWorkdirLeaseRepository
	IDGen             idgen.IDGenerator
	WorkerID          string
	LeaseTTLMillis    int64
	AllowedCommands   []string
	AllowedEnvKeys    []string
	MaxArgs           int
	MaxArgBytes       int
	MaxEnvVars        int
	MaxEnvValueBytes  int
	MaxConfigBytes    int
	DirMode           os.FileMode
	NowMillis         func() int64
	DryRunOutputBytes int
}

type ADKMCPRuntimeStdioRuntimeTransportOptions struct {
	WorkdirRoot      string
	LeaseRepository  domainrepo.MCPRuntimeWorkdirLeaseRepository
	IDGen            idgen.IDGenerator
	WorkerID         string
	LeaseTTLMillis   int64
	AllowedCommands  []string
	AllowedEnvKeys   []string
	MaxArgs          int
	MaxArgBytes      int
	MaxEnvVars       int
	MaxEnvValueBytes int
	MaxConfigBytes   int
	DirMode          os.FileMode
	NowMillis        func() int64
	Runner           ADKMCPRuntimeStdioSandboxRunner
}

func NewADKMCPRuntimeStdioDryRunTransport(
	options ADKMCPRuntimeStdioDryRunTransportOptions,
) *ADKMCPRuntimeStdioTransport {
	return NewADKMCPRuntimeStdioRuntimeTransport(
		ADKMCPRuntimeStdioRuntimeTransportOptions{
			WorkdirRoot:      options.WorkdirRoot,
			LeaseRepository:  options.LeaseRepository,
			IDGen:            options.IDGen,
			WorkerID:         options.WorkerID,
			LeaseTTLMillis:   options.LeaseTTLMillis,
			AllowedCommands:  options.AllowedCommands,
			AllowedEnvKeys:   options.AllowedEnvKeys,
			MaxArgs:          options.MaxArgs,
			MaxArgBytes:      options.MaxArgBytes,
			MaxEnvVars:       options.MaxEnvVars,
			MaxEnvValueBytes: options.MaxEnvValueBytes,
			MaxConfigBytes:   options.MaxConfigBytes,
			DirMode:          options.DirMode,
			NowMillis:        options.NowMillis,
			Runner: NewADKMCPRuntimeStdioDryRunRunner(
				ADKMCPRuntimeStdioDryRunRunnerOptions{
					MaxOutputBytes: options.DryRunOutputBytes,
				},
			),
		},
	)
}

func NewADKMCPRuntimeStdioRuntimeTransport(
	options ADKMCPRuntimeStdioRuntimeTransportOptions,
) *ADKMCPRuntimeStdioTransport {
	root := filepath.Clean(strings.TrimSpace(options.WorkdirRoot))
	workdirManager := NewADKMCPRuntimeStdioWorkdirManager(
		ADKMCPRuntimeStdioWorkdirManagerOptions{Root: root},
	)
	policy := NewADKMCPRuntimeStdioStaticPolicy(
		ADKMCPRuntimeStdioStaticPolicyOptions{
			AllowedCommands:           options.AllowedCommands,
			AllowedWorkingDirPrefixes: []string{root},
			AllowedEnvKeys:            options.AllowedEnvKeys,
			MaxArgs:                   options.MaxArgs,
			MaxArgBytes:               options.MaxArgBytes,
			MaxEnvVars:                options.MaxEnvVars,
			MaxEnvValueBytes:          options.MaxEnvValueBytes,
			RequireWorkingDir:         true,
		},
	)
	innerPreparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{
			Root:    root,
			DirMode: options.DirMode,
		},
	)
	leaseStore := NewApplicationADKMCPRuntimeStdioWorkdirLeaseStore(
		ApplicationADKMCPRuntimeStdioWorkdirLeaseStoreOptions{
			Repository:     options.LeaseRepository,
			IDGen:          options.IDGen,
			WorkerID:       options.WorkerID,
			LeaseTTLMillis: options.LeaseTTLMillis,
			NowMillis:      options.NowMillis,
		},
	)
	leasedPreparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner:      innerPreparer,
			LeaseStore: leaseStore,
		},
	)
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{
			Runner:          options.Runner,
			WorkdirPreparer: leasedPreparer,
		},
	)

	return NewADKMCPRuntimeStdioTransport(
		ADKMCPRuntimeStdioTransportOptions{
			Policy:         policy,
			Sandbox:        sandbox,
			WorkdirManager: workdirManager,
			MaxConfigBytes: options.MaxConfigBytes,
		},
	)
}

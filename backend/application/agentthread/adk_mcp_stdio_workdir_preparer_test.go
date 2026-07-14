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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioFilesystemWorkdirPreparerCreatesAndCleans(
	t *testing.T,
) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{
			Root:    root,
			DirMode: 0o700,
		},
	)

	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(),
		execution,
	)

	require.NoError(t, err)
	require.Equal(t, filepath.Clean(root), prepared.Root)
	require.NotEqual(t, execution.WorkingDir, prepared.WorkingDir)
	require.True(t, adkMCPRuntimePathWithin(prepared.WorkingDir, root))
	require.True(t, strings.HasPrefix(filepath.Base(prepared.WorkingDir), adkMCPRuntimeStdioInvocationDirPrefix))
	info, err := os.Stat(prepared.WorkingDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
	require.Equal(t, os.FileMode(0o700), info.Mode().Perm())

	err = preparer.CleanupADKMCPRuntimeStdioWorkdir(
		context.Background(),
		prepared,
	)

	require.NoError(t, err)
	_, err = os.Stat(prepared.WorkingDir)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(root)
	require.NoError(t, err)
}

func TestADKMCPRuntimeStdioFilesystemWorkdirPreparerIgnoresPersistedWorkingDir(
	t *testing.T,
) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	outside := filepath.Join(filepath.Dir(root), filepath.Base(root)+"-escape")
	preparer := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{
			Root:    root,
			DirMode: 0o700,
		},
	)
	execution := validADKMCPRuntimeStdioSandboxExecution(root)
	execution.WorkingDir = filepath.Join(outside, "run-20")

	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(),
		execution,
	)

	require.NoError(t, err)
	require.True(t, adkMCPRuntimePathWithin(prepared.WorkingDir, root))
	require.NoError(t, preparer.CleanupADKMCPRuntimeStdioWorkdir(context.Background(), prepared))
	_, statErr := os.Stat(outside)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestADKMCPRuntimeStdioSandboxPreparesRunsAndCleansWorkdir(t *testing.T) {
	order := []string{}
	preparer := &recordingADKMCPRuntimeStdioWorkdirPreparer{
		order: &order,
	}
	runner := &recordingADKMCPRuntimeStdioSandboxRunner{
		result: `{"schema":"coze.mcp_stdio_runner_result.v1","content":"ok"}`,
		order:  &order,
	}
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{
			Runner:          runner,
			WorkdirPreparer: preparer,
		},
	)

	result, err := sandbox.InvokeADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.NoError(t, err)
	require.Equal(t, runner.result, result)
	require.Equal(t, []string{"prepare", "run", "cleanup"}, order)
	require.Equal(t, 1, preparer.prepareCalls)
	require.Equal(t, 1, runner.calls)
	require.Equal(t, 1, preparer.cleanupCalls)
	require.Equal(t, runner.execution.WorkingDir, preparer.prepared.WorkingDir)
	require.Equal(t, preparer.prepared, preparer.cleaned)
}

func TestADKMCPRuntimeStdioSandboxCleansWorkdirAfterRunnerFailure(
	t *testing.T,
) {
	order := []string{}
	preparer := &recordingADKMCPRuntimeStdioWorkdirPreparer{
		order: &order,
	}
	runner := &recordingADKMCPRuntimeStdioSandboxRunner{
		err: fmt.Errorf(
			`spawn npx in /mnt/coze/mcp/run-20 with stdio-secret-token`,
		),
		order: &order,
	}
	sandbox := NewADKMCPRuntimeStdioSandbox(
		ADKMCPRuntimeStdioSandboxOptions{
			Runner:          runner,
			WorkdirPreparer: preparer,
		},
	)

	result, err := sandbox.InvokeADKMCPRuntimeStdio(
		context.Background(),
		validADKMCPRuntimeStdioPolicyCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio runner failed")
	assertADKMCPStdioSandboxErrorDoesNotLeak(t, err.Error())
	require.Equal(t, []string{"prepare", "run", "cleanup"}, order)
	require.Equal(t, 1, preparer.cleanupCalls)
}

func validADKMCPRuntimeStdioSandboxExecution(
	root string,
) ADKMCPRuntimeStdioSandboxExecution {
	return ADKMCPRuntimeStdioSandboxExecution{
		Run:        &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
		Name:       "mcp_100_search_docs",
		ServerID:   100,
		ToolName:   "search-docs",
		Arguments:  `{"query":"secret customer path"}`,
		Command:    "npx",
		Args:       []string{"-y", "@example/secret-mcp-server"},
		Env:        map[string]string{"API_TOKEN": "stdio-secret-token"},
		WorkingDir: filepath.Join(root, "spaces", "30", "threads", "10", "runs", "20"),
	}
}

type recordingADKMCPRuntimeStdioWorkdirPreparer struct {
	order        *[]string
	prepared     ADKMCPRuntimeStdioPreparedWorkdir
	cleaned      ADKMCPRuntimeStdioPreparedWorkdir
	prepareCalls int
	cleanupCalls int
	prepareErr   error
	cleanupErr   error
}

func (p *recordingADKMCPRuntimeStdioWorkdirPreparer) PrepareADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
) (ADKMCPRuntimeStdioPreparedWorkdir, error) {
	p.prepareCalls++
	if p.order != nil {
		*p.order = append(*p.order, "prepare")
	}
	if p.prepareErr != nil {
		return ADKMCPRuntimeStdioPreparedWorkdir{}, p.prepareErr
	}
	p.prepared = ADKMCPRuntimeStdioPreparedWorkdir{
		Root:       filepath.Dir(execution.WorkingDir),
		WorkingDir: execution.WorkingDir,
	}

	return p.prepared, nil
}

func (p *recordingADKMCPRuntimeStdioWorkdirPreparer) CleanupADKMCPRuntimeStdioWorkdir(
	ctx context.Context,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) error {
	p.cleanupCalls++
	if p.order != nil {
		*p.order = append(*p.order, "cleanup")
	}
	p.cleaned = prepared

	return p.cleanupErr
}

func assertADKMCPStdioWorkdirPreparerErrorDoesNotLeak(
	t *testing.T,
	text string,
	values ...string,
) {
	t.Helper()
	assertADKMCPStdioSandboxErrorDoesNotLeak(t, text)
	for _, value := range values {
		require.NotContains(t, text, value)
	}
}

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
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestADKMCPRuntimeStdioDryRunTransportBuildsFullSmokeChain(
	t *testing.T,
) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	repo := &recordingMCPRuntimeWorkdirLeaseRepository{
		finishedLease: &domainentity.MCPRuntimeWorkdirLease{
			ID:     9001,
			Status: domainentity.MCPRuntimeWorkdirLeaseStatusReleased,
		},
		finishOK: true,
	}
	transport := NewADKMCPRuntimeStdioDryRunTransport(
		ADKMCPRuntimeStdioDryRunTransportOptions{
			WorkdirRoot:        root,
			LeaseRepository:    repo,
			IDGen:              &mcpWorkdirLeaseSequenceIDGen{next: 9001},
			WorkerID:           "worker-a",
			LeaseTTLMillis:     120000,
			AllowedCommands:    []string{"npx"},
			AllowedNpxPackages: []string{"@example/secret-mcp-server"},
			AllowedEnvKeys:     []string{"API_TOKEN"},
			MaxArgs:            4,
			MaxArgBytes:        96,
			MaxEnvVars:         1,
			MaxEnvValueBytes:   32,
			NowMillis:          func() int64 { return 1000 },
			DryRunOutputBytes:  4096,
		},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.NoError(t, err)
	payload := decodeADKMCPRuntimeStdioDryRunResult(t, result)
	require.Equal(t, "coze.mcp_stdio_dry_run.v1", payload.Schema)
	require.Equal(t, "dry_run", payload.Status)
	require.Equal(t, "mcp_100_search_docs", payload.RuntimeToolName)
	require.Equal(t, int64(100), payload.ServerID)
	assertADKMCPStdioDryRunResultDoesNotLeak(t, result, root)

	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.createdLease)
	require.Equal(t, int64(9001), repo.createdLease.ID)
	require.Equal(t, int64(20), repo.createdLease.RunID)
	require.Equal(t, int64(10), repo.createdLease.ThreadID)
	require.Equal(t, int64(30), repo.createdLease.SpaceID)
	require.Equal(t, int64(100), repo.createdLease.ServerID)
	require.Equal(t, "mcp_100_search_docs", repo.createdLease.RuntimeToolName)
	require.Equal(t, "worker-a", repo.createdLease.WorkerID)
	require.Equal(t, int64(121000), repo.createdLease.LeaseExpiresAt)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusActive, repo.createdLease.Status)
	require.Contains(t, repo.createdLease.Workdir, root)

	require.Equal(t, 1, repo.finishCalls)
	require.Equal(t, int64(9001), repo.finishReq.LeaseID)
	require.Equal(t, "worker-a", repo.finishReq.WorkerID)
	require.Equal(t, domainentity.MCPRuntimeWorkdirLeaseStatusReleased, repo.finishReq.Status)
	require.Empty(t, repo.finishReq.LastError)

	_, statErr := os.Stat(repo.createdLease.Workdir)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestADKMCPRuntimeStdioDryRunTransportFailsClosedWithoutLeaseStore(
	t *testing.T,
) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	transport := NewADKMCPRuntimeStdioDryRunTransport(
		ADKMCPRuntimeStdioDryRunTransportOptions{
			WorkdirRoot:        root,
			AllowedCommands:    []string{"npx"},
			AllowedNpxPackages: []string{"@example/secret-mcp-server"},
			AllowedEnvKeys:     []string{"API_TOKEN"},
			MaxArgs:            4,
			MaxArgBytes:        96,
			MaxEnvVars:         1,
			MaxEnvValueBytes:   32,
		},
	)

	result, err := transport.InvokeADKMCPRuntimeTransport(
		context.Background(),
		validADKMCPRuntimeStdioCall(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "mcp runtime stdio transport failed")
	assertADKMCPStdioLeaseStoreErrorDoesNotLeak(t, err.Error())
}

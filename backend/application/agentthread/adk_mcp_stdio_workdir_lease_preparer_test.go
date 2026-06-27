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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKMCPRuntimeStdioLeasedWorkdirPreparerCreatesLeaseAfterPrepare(
	t *testing.T,
) {
	inner := &recordingADKMCPRuntimeStdioWorkdirPreparer{}
	store := &recordingADKMCPRuntimeStdioWorkdirLeaseStore{
		lease: ADKMCPRuntimeStdioWorkdirLease{
			LeaseID:  5001,
			WorkerID: "worker-a",
		},
	}
	preparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner:      inner,
			LeaseStore: store,
		},
	)
	execution := validADKMCPRuntimeStdioPolicyCall()
	projected, err := projectADKMCPRuntimeStdioSandboxExecution(execution)
	require.NoError(t, err)

	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(),
		projected,
	)

	require.NoError(t, err)
	require.Equal(t, int64(5001), prepared.LeaseID)
	require.Equal(t, "worker-a", prepared.LeaseWorkerID)
	require.Equal(t, 1, inner.prepareCalls)
	require.Equal(t, 1, store.createCalls)
	require.Equal(t, projected.Run.RunID, store.execution.Run.RunID)
	require.Equal(t, inner.prepared.WorkingDir, store.prepared.WorkingDir)
}

func TestADKMCPRuntimeStdioLeasedWorkdirPreparerReleasesLeaseAfterCleanup(
	t *testing.T,
) {
	inner := &recordingADKMCPRuntimeStdioWorkdirPreparer{}
	store := &recordingADKMCPRuntimeStdioWorkdirLeaseStore{
		lease: ADKMCPRuntimeStdioWorkdirLease{
			LeaseID:  5001,
			WorkerID: "worker-a",
		},
	}
	preparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner:      inner,
			LeaseStore: store,
		},
	)
	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(),
		mustProjectValidADKMCPRuntimeStdioExecution(t),
	)
	require.NoError(t, err)

	err = preparer.CleanupADKMCPRuntimeStdioWorkdir(
		context.Background(),
		prepared,
	)

	require.NoError(t, err)
	require.Equal(t, 1, inner.cleanupCalls)
	require.Equal(t, 1, store.finishCalls)
	require.Equal(t, int64(5001), store.finishedLease.LeaseID)
	require.Equal(t, ADKMCPRuntimeStdioWorkdirLeaseStatusReleased, store.status)
	require.Empty(t, store.errorText)
}

func TestADKMCPRuntimeStdioLeasedWorkdirPreparerMarksLeaseFailedOnCleanupError(
	t *testing.T,
) {
	inner := &recordingADKMCPRuntimeStdioWorkdirPreparer{
		cleanupErr: fmt.Errorf(
			`cleanup /mnt/coze/mcp/run-20 with stdio-secret-token`,
		),
	}
	store := &recordingADKMCPRuntimeStdioWorkdirLeaseStore{
		lease: ADKMCPRuntimeStdioWorkdirLease{
			LeaseID:  5001,
			WorkerID: "worker-a",
		},
	}
	preparer := NewADKMCPRuntimeStdioLeasedWorkdirPreparer(
		ADKMCPRuntimeStdioLeasedWorkdirPreparerOptions{
			Inner:      inner,
			LeaseStore: store,
		},
	)
	prepared, err := preparer.PrepareADKMCPRuntimeStdioWorkdir(
		context.Background(),
		mustProjectValidADKMCPRuntimeStdioExecution(t),
	)
	require.NoError(t, err)

	err = preparer.CleanupADKMCPRuntimeStdioWorkdir(
		context.Background(),
		prepared,
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "mcp runtime stdio workdir cleanup failed")
	assertADKMCPStdioWorkdirPreparerErrorDoesNotLeak(t, err.Error())
	require.Equal(t, 1, store.finishCalls)
	require.Equal(t, ADKMCPRuntimeStdioWorkdirLeaseStatusFailed, store.status)
	require.Equal(t, "cleanup failed", store.errorText)
}

func mustProjectValidADKMCPRuntimeStdioExecution(
	t *testing.T,
) ADKMCPRuntimeStdioSandboxExecution {
	t.Helper()
	execution, err := projectADKMCPRuntimeStdioSandboxExecution(
		validADKMCPRuntimeStdioPolicyCall(),
	)
	require.NoError(t, err)

	return execution
}

type recordingADKMCPRuntimeStdioWorkdirLeaseStore struct {
	execution     ADKMCPRuntimeStdioSandboxExecution
	prepared      ADKMCPRuntimeStdioPreparedWorkdir
	finishedLease ADKMCPRuntimeStdioWorkdirLease
	status        ADKMCPRuntimeStdioWorkdirLeaseStatus
	errorText     string
	lease         ADKMCPRuntimeStdioWorkdirLease
	createCalls   int
	finishCalls   int
	createErr     error
	finishErr     error
}

func (s *recordingADKMCPRuntimeStdioWorkdirLeaseStore) CreateADKMCPRuntimeStdioWorkdirLease(
	ctx context.Context,
	execution ADKMCPRuntimeStdioSandboxExecution,
	prepared ADKMCPRuntimeStdioPreparedWorkdir,
) (ADKMCPRuntimeStdioWorkdirLease, error) {
	s.createCalls++
	s.execution = execution
	s.prepared = prepared
	if s.createErr != nil {
		return ADKMCPRuntimeStdioWorkdirLease{}, s.createErr
	}

	return s.lease, nil
}

func (s *recordingADKMCPRuntimeStdioWorkdirLeaseStore) FinishADKMCPRuntimeStdioWorkdirLease(
	ctx context.Context,
	lease ADKMCPRuntimeStdioWorkdirLease,
	status ADKMCPRuntimeStdioWorkdirLeaseStatus,
	errorText string,
) error {
	s.finishCalls++
	s.finishedLease = lease
	s.status = status
	s.errorText = errorText

	return s.finishErr
}

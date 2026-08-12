// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestSignedSchedulerRunnerSignsPersistedSnapshotBeforePushing(t *testing.T) {
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	remote := &schedulerConfigurationRemoteFake{}
	runner, err := NewSignedSchedulerRunner(signer, []SchedulerConfigurationRemote{remote})
	require.NoError(t, err)
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 2

	require.NoError(t, runner.ApplySchedulerSettings(context.Background(), settings))
	require.Len(t, remote.applied, 1)
	verified, err := signer.Verify(remote.applied[0], 1)
	require.NoError(t, err)
	require.Equal(t, settings.Version, verified.Version)
}

type schedulerConfigurationRemoteFake struct {
	applied []infrasandbox.SchedulerConfiguration
}

func (f *schedulerConfigurationRemoteFake) ApplySchedulerConfiguration(_ context.Context, value infrasandbox.SchedulerConfiguration) error {
	f.applied = append(f.applied, value)
	return nil
}

func (*schedulerConfigurationRemoteFake) RuntimeStatus(context.Context) (infrasandbox.SchedulerRuntimeStatus, error) {
	return infrasandbox.SchedulerRuntimeStatus{Schema: infrasandbox.SchedulerRuntimeStatusSchemaV1, AppliedConfigurationVersion: 1, TotalWeight: 2, MemoryReserveState: infrasandbox.RuntimeMemoryReserveAvailable}, nil
}

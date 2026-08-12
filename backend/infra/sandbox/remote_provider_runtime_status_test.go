// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestRemoteProviderRuntimeStatusUsesStrictAggregateProjection(t *testing.T) {
	tests := map[string]struct {
		body    string
		wantErr error
	}{
		"valid":                     {body: `{"schema":"coze.sandbox.runner_runtime_status.v1","applied_configuration_version":3,"queued":2,"queued_by_scope":{"agent":1,"plugin":1},"running":1,"used_weight":2,"total_weight":2,"idle_containers":1,"active_containers":1,"quarantined_containers":0,"memory_reserve_state":"available"}`},
		"identifier is rejected":    {body: `{"schema":"coze.sandbox.runner_runtime_status.v1","applied_configuration_version":3,"queued":2,"queued_by_scope":{},"running":1,"used_weight":2,"total_weight":2,"idle_containers":1,"active_containers":1,"quarantined_containers":0,"memory_reserve_state":"available","execution_id":"must-not-leak"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"unknown state is rejected": {body: `{"schema":"coze.sandbox.runner_runtime_status.v1","applied_configuration_version":3,"queued":2,"queued_by_scope":{},"running":1,"used_weight":2,"total_weight":2,"idle_containers":1,"active_containers":1,"quarantined_containers":0,"memory_reserve_state":"raw-host-memory"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, request.Method)
				require.Equal(t, "/v1/runtime-status", request.URL.Path)
				return jsonResponse(request, http.StatusOK, test.body), nil
			}))
			status, err := provider.RuntimeStatus(context.Background())
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Equal(t, SchedulerRuntimeStatus{}, status)
				return
			}
			require.NoError(t, err)
			require.Equal(t, uint64(3), status.AppliedConfigurationVersion)
			require.Equal(t, 2, status.Queued)
			require.Equal(t, 1, status.QueuedByScope["agent"])
			require.Equal(t, RuntimeMemoryReserveAvailable, status.MemoryReserveState)
		})
	}
}

func TestRemoteProviderRuntimeStatusMapsTransportFailureWithoutResponseDetails(t *testing.T) {
	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("runner password must-not-leak")
	}))
	status, err := provider.RuntimeStatus(context.Background())
	require.ErrorIs(t, err, domainsandbox.ErrProviderUnhealthy)
	require.Equal(t, SchedulerRuntimeStatus{}, status)
	require.NotContains(t, err.Error(), "password")
}

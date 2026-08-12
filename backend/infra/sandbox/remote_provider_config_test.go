// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestRemoteProviderPushesOnlyCompleteSignedSchedulerConfiguration(t *testing.T) {
	signer, err := NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 2
	configuration, err := signer.Sign(settings)
	require.NoError(t, err)

	provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, request.Method)
		require.Equal(t, "/v1/configuration", request.URL.Path)
		var got SchedulerConfiguration
		require.NoError(t, json.NewDecoder(request.Body).Decode(&got))
		require.Equal(t, configuration, got)
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
	}))

	require.NoError(t, provider.ApplySchedulerConfiguration(context.Background(), configuration))
}

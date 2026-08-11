// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"net/http"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestRemoteProviderQueueStatusUsesStrictSafeProjection(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		body    string
		wantErr error
	}{
		"valid":                         {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`},
		"unknown field":                 {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE","tenant":"must-not-leak"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"duplicate field":               {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"waiting":false,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"case variant duplicate field":  {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"Waiting":false,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"negative approximate position": {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"approximate_position":-1,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"negative estimated wait":       {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"approximate_position":3,"estimated_wait_seconds":-1,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"negative deadline":             {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":-1,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"invalid reason":                {body: `{"schema":"coze.sandbox.queue_status.v1","waiting":true,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"unsafe reason"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"malformed JSON":                {body: `{"schema":"coze.sandbox.queue_status.v1"`, wantErr: domainsandbox.ErrProviderUnhealthy},
		"wrong schema":                  {body: `{"schema":"coze.sandbox.queue_status.v2","waiting":true,"approximate_position":3,"estimated_wait_seconds":12,"deadline_unix_milli":1770000000000,"cancelable":true,"reason_code":"CAPACITY_UNAVAILABLE"}`, wantErr: domainsandbox.ErrProviderUnhealthy},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Path != "/v1/executions/exec_123/queue-status" {
					t.Fatalf("queue status request = %s %s", request.Method, request.URL.Path)
				}
				return jsonResponse(request, http.StatusOK, test.body), nil
			}))
			status, err := provider.QueueStatus(context.Background(), "exec_123")
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("QueueStatus() error = %v", err)
				}
				if status != (QueueStatus{}) {
					t.Fatalf("QueueStatus() returned projection on error: %#v", status)
				}
				return
			}
			if err != nil {
				t.Fatalf("QueueStatus() error = %v", err)
			}
			if !status.Waiting || status.ApproximatePosition != 3 || status.ReasonCode != "CAPACITY_UNAVAILABLE" {
				t.Fatalf("queue status = %#v", status)
			}
		})
	}
}

func TestRemoteProviderQueueStatusRejectsInvalidExecutionIDWithoutProjection(t *testing.T) {
	t.Parallel()

	calls := 0
	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected request")
	}))
	status, err := provider.QueueStatus(context.Background(), "not/a-valid-id")
	if !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("QueueStatus() error = %v", err)
	}
	if status != (QueueStatus{}) || calls != 0 {
		t.Fatalf("QueueStatus() status/calls = %#v/%d", status, calls)
	}
}

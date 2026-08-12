// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServerMetricsExposeOnlyFixedLowCardinalityLabels(t *testing.T) {
	dependencies := &recordingDependencies{}
	server := newTestServerWithDependencies(t, dependencies.Dependencies())
	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized || dependencies.runtimeStatusCalls != 0 {
		t.Fatalf("unauthorized status/calls = %d/%d", unauthorized.Code, dependencies.runtimeStatusCalls)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || dependencies.runtimeStatusCalls != 1 {
		t.Fatalf("status/calls = %d/%d", response.Code, dependencies.runtimeStatusCalls)
	}
	body := response.Body.String()
	for _, expected := range []string{
		`coze_sandbox_runner_queue_depth{scope="agent"} 2`,
		`coze_sandbox_runner_queue_depth{scope="appdev"} 0`,
		`coze_sandbox_runner_weight{state="used"} 2`,
		`coze_sandbox_runner_memory_reserve{state="available"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics missing %q: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"execution", "tenant", "space", "container_id", "credential", "endpoint", "must-not-leak"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics leaked %q: %s", forbidden, body)
		}
	}
}

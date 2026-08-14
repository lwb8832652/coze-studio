// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestSandboxHealthCheckCallsAdapterAndPersistsBoundedProjection(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(81, 3)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	adapter := &testHealthProvider{result: infrasandbox.HealthResult{
		ProtocolVersion: infrasandbox.HealthProtocolV1,
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Features: []domainsandbox.ProviderFeature{
			domainsandbox.ProviderFeatureSandboxSessionV1,
			domainsandbox.ProviderFeatureSignedSessionContextV2,
		},
	}}
	h.factory.provider = adapter
	checkedAt := time.Date(2026, 7, 15, 5, 0, 0, 12_000_000, time.UTC)
	times := []time.Time{
		time.Date(2026, 7, 15, 5, 0, 0, 0, time.UTC),
		checkedAt,
	}
	h.service.now = func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}

	result, err := h.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{
		ProviderID: provider.ID, ExpectedVersion: 3,
	})
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if h.factory.calls != 1 || adapter.calls != 1 || adapter.closeCalls != 1 || h.factory.input.ProviderKey != provider.ProviderKey ||
		h.factory.input.CredentialSecret != provider.CredentialSecret {
		t.Fatalf("factory/adapter calls/input = %d/%d/%#v", h.factory.calls, adapter.calls, h.factory.input)
	}
	if result.Version != 4 || result.Health.Status != domainsandbox.HealthStatusHealthy ||
		result.Health.ReasonCode != healthCodeOK || result.Health.LatencyBucket != healthLatencyBucketFast ||
		!result.Health.CheckedAt.Equal(checkedAt) {
		t.Fatalf("health projection = %#v", result)
	}
	stored := h.providers.providers[provider.ID]
	if stored.Health.Status != domainsandbox.HealthStatusHealthy || stored.Version != 4 || len(h.audits.events) != 1 ||
		!reflect.DeepEqual(stored.Health.Features, adapter.result.Features) {
		t.Fatalf("stored health/audit = %#v/%#v", stored, h.audits.events)
	}
	audit := h.audits.events[0]
	if audit.Action != auditActionHealthCheck || audit.Result != auditResultSuccess ||
		audit.Metadata[domainsandbox.AuditMetadataKeyHealthCode] != healthCodeOK ||
		audit.Metadata[domainsandbox.AuditMetadataKeyVersion] != "4" {
		t.Fatalf("health audit = %#v", audit)
	}
	assertSanitizedJSON(t, []any{result, audit}, provider.EndpointSecret, provider.CredentialSecret, "private/path")
}

func TestSandboxHealthCheckPersistsSanitizedFailureAndCASRollback(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(82, 3)
	h.providers.providers[provider.ID] = provider
	secretFailure := "underlying provider error: endpoint=https://sandbox.example.test/private/path credential=credential-must-not-leak token=token-must-not-leak secret=secret-must-not-leak raw-provider-body"
	adapter := &testHealthProvider{err: errors.New(secretFailure)}
	h.factory.provider = adapter

	result, err := h.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{
		ProviderID: provider.ID, ExpectedVersion: 3,
	})
	if err != nil {
		t.Fatalf("HealthCheck(failure) error = %v", err)
	}
	if result.Health.Status != domainsandbox.HealthStatusUnhealthy || result.Health.ReasonCode != healthCodeProbeFailed ||
		result.Health.Message != healthMessageProbeFailed || result.Health.CheckedAt.IsZero() || result.Version != 4 {
		t.Fatalf("failed health projection = %#v", result)
	}
	if adapter.closeCalls != 1 {
		t.Fatalf("failed health provider close calls = %d", adapter.closeCalls)
	}
	if audit := h.audits.events[0]; audit.Result != auditResultFailure ||
		audit.Metadata[domainsandbox.AuditMetadataKeyHealthCode] != healthCodeProbeFailed {
		t.Fatalf("failed health audit = %#v", audit)
	}
	assertSanitizedJSON(t, []any{result, h.audits.events},
		secretFailure,
		provider.EndpointSecret,
		provider.CredentialSecret,
		"https://sandbox.example.test/private/path",
		"underlying provider error",
		"credential-must-not-leak",
		"token-must-not-leak",
		"secret-must-not-leak",
		"raw-provider-body",
	)

	conflictHarness := newControlPlaneHarness(t)
	conflictProvider := testProvider(83, 6)
	conflictHarness.providers.providers[conflictProvider.ID] = conflictProvider
	conflictHarness.providers.healthErr = domainsandbox.ErrVersionConflict
	if _, err := conflictHarness.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{
		ProviderID: conflictProvider.ID, ExpectedVersion: 6,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("HealthCheck(conflict) error = %v", err)
	}
	if conflictHarness.providers.providers[conflictProvider.ID].Version != 6 || len(conflictHarness.audits.events) != 0 || conflictHarness.uow.committed {
		t.Fatal("health conflict did not roll back")
	}
}

func TestSandboxHealthCheckClosesTemporaryProviderOnBuildFailureCancellationAndCloseError(t *testing.T) {
	for _, test := range []struct {
		name     string
		probeErr error
		buildErr error
		closeErr error
	}{
		{name: "build returns provider and error", buildErr: errors.New("build raw endpoint secret")},
		{name: "probe cancellation", probeErr: context.Canceled},
		{name: "close failure", closeErr: errors.New("close token secret")},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newControlPlaneHarness(t)
			provider := testProvider(90, 1)
			h.providers.providers[provider.ID] = provider
			adapter := &testHealthProvider{
				result: infrasandbox.HealthResult{ProtocolVersion: infrasandbox.HealthProtocolV1, Status: domainsandbox.HealthStatusHealthy},
				err:    test.probeErr, closeErr: test.closeErr,
			}
			h.factory.provider = adapter
			h.factory.err = test.buildErr

			result, err := h.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{ProviderID: provider.ID, ExpectedVersion: 1})
			if err != nil {
				t.Fatalf("HealthCheck() error = %v", err)
			}
			if adapter.closeCalls != 1 {
				t.Fatalf("provider close calls = %d", adapter.closeCalls)
			}
			if result.Health.Status != domainsandbox.HealthStatusUnhealthy || result.Health.ReasonCode != healthCodeProbeFailed {
				t.Fatalf("failure projection = %#v", result.Health)
			}
			assertSanitizedJSON(t, result, "build raw endpoint secret", "close token secret")
		})
	}
}

func TestSandboxHealthProjectionBucketsLatencyAndReturnsBoundedSafeSummary(t *testing.T) {
	tests := []struct {
		millis int64
		want   string
	}{
		{millis: -1, want: healthLatencyBucketUnknown},
		{millis: 0, want: healthLatencyBucketFast},
		{millis: 99, want: healthLatencyBucketFast},
		{millis: 100, want: healthLatencyBucketNormal},
		{millis: 499, want: healthLatencyBucketNormal},
		{millis: 500, want: healthLatencyBucketSlow},
		{millis: 1999, want: healthLatencyBucketSlow},
		{millis: 2000, want: healthLatencyBucketVerySlow},
		{millis: maxHealthLatencyMillis + 1, want: healthLatencyBucketUnknown},
	}
	for _, tt := range tests {
		if got := boundedHealthLatencyBucket(tt.millis); got != tt.want {
			t.Errorf("boundedHealthLatencyBucket(%d) = %q, want %q", tt.millis, got, tt.want)
		}
	}
	projection := HealthProjection{
		Status: domainsandbox.HealthStatusUnhealthy, Capabilities: []domainsandbox.Scope{},
		ReasonCode: healthCodeProbeFailed, Message: healthMessageProbeFailed,
		LatencyBucket: healthLatencyBucketNormal,
		CheckedAt:     time.Date(2026, 7, 15, 5, 0, 0, 0, time.UTC),
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if len(projection.Message) > domainsandbox.MaxHealthMessageLength ||
		!strings.Contains(string(encoded), `"message":"`+healthMessageProbeFailed+`"`) ||
		!strings.Contains(string(encoded), `"status":"unhealthy"`) ||
		!strings.Contains(string(encoded), `"checked_at":"2026-07-15T05:00:00Z"`) ||
		strings.Contains(string(encoded), "latency_millis") ||
		!strings.Contains(string(encoded), `"latency_bucket":"normal"`) {
		t.Fatalf("health projection omitted safe contract fields or exposed internal diagnostics: %s", encoded)
	}
}

func TestSandboxHealthCheckRejectsPermissionBeforeFactory(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(84, 1)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	h.providers.providers[provider.ID] = provider
	actor := testActor()
	actor.AllowedScopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}

	_, err := h.service.HealthCheck(context.Background(), actor, HealthCheckRequest{
		ProviderID: provider.ID, ExpectedVersion: 1,
	})
	if !errors.Is(err, ErrPermissionDenied) || h.factory.calls != 0 {
		t.Fatalf("HealthCheck(permission) error/factory = %v/%d", err, h.factory.calls)
	}
	if strings.Contains(err.Error(), provider.ProviderKey) {
		t.Fatalf("permission error leaked provider key: %v", err)
	}
}

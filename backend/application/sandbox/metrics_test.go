// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSandboxMetricsRecordsBoundedOperationalSignals(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewSandboxPrometheusMetricsCollector(registry)
	if err != nil {
		t.Fatalf("NewSandboxPrometheusMetricsCollector() error = %v", err)
	}
	ctx := context.Background()
	collector.RecordProviderSelection(ctx, ProviderSelectionMetricsObservation{
		Scope: domainsandbox.ScopeAgent, ProviderType: domainsandbox.ProviderTypeRemoteHTTP,
		Outcome: "success", ResultCode: "none",
	})
	collector.RecordProviderHealth(ctx, ProviderHealthMetricsObservation{
		ProviderType: domainsandbox.ProviderTypeRemoteHTTP, Status: domainsandbox.HealthStatusHealthy,
		Outcome: "success", ResultCode: "none", Elapsed: 25 * time.Millisecond,
	})
	collector.RecordExecution(ctx, SandboxExecutionMetricsObservation{
		Scope: domainsandbox.ScopeAgent, ProviderType: domainsandbox.ProviderTypeRemoteHTTP,
		Outcome: "success", ResultCode: "accepted", Elapsed: 50 * time.Millisecond,
	})
	collector.RecordCapacityRejection(ctx, CapacityRejectionMetricsObservation{
		Scope: domainsandbox.ScopeAgent, ProviderType: domainsandbox.ProviderTypeRemoteHTTP,
		ResultCode: domainsandbox.ErrCodeCapacityExhausted,
	})
	collector.RecordCredentialDecryptFailure(ctx, CredentialDecryptFailureMetricsObservation{
		ProviderType: domainsandbox.ProviderTypeRemoteHTTP,
		Field:        "credential",
		ResultCode:   domainsandbox.ErrCodeConfigurationInvalid,
	})

	if got := testutil.ToFloat64(collector.providerSelectionsTotal.WithLabelValues(
		string(domainsandbox.ProviderTypeRemoteHTTP), string(domainsandbox.ScopeAgent), "success", "none",
	)); got != 1 {
		t.Fatalf("provider selection metric = %v", got)
	}
	if got := testutil.ToFloat64(collector.providerHealthChecksTotal.WithLabelValues(
		string(domainsandbox.ProviderTypeRemoteHTTP), string(domainsandbox.HealthStatusHealthy), "success", "none",
	)); got != 1 {
		t.Fatalf("provider health metric = %v", got)
	}
	if got := testutil.ToFloat64(collector.executionsTotal.WithLabelValues(
		string(domainsandbox.ProviderTypeRemoteHTTP), string(domainsandbox.ScopeAgent), "success", "accepted",
	)); got != 1 {
		t.Fatalf("execution metric = %v", got)
	}
	if got := testutil.ToFloat64(collector.capacityRejectionsTotal.WithLabelValues(
		string(domainsandbox.ProviderTypeRemoteHTTP), string(domainsandbox.ScopeAgent),
		domainsandbox.ErrCodeCapacityExhausted,
	)); got != 1 {
		t.Fatalf("capacity metric = %v", got)
	}
	if got := testutil.ToFloat64(collector.credentialDecryptFailuresTotal.WithLabelValues(
		string(domainsandbox.ProviderTypeRemoteHTTP), "credential",
		domainsandbox.ErrCodeConfigurationInvalid,
	)); got != 1 {
		t.Fatalf("decrypt metric = %v", got)
	}
}

func TestSandboxMetricsRejectsSensitiveOrHighCardinalityLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	collector, err := NewSandboxPrometheusMetricsCollector(registry)
	if err != nil {
		t.Fatalf("NewSandboxPrometheusMetricsCollector() error = %v", err)
	}
	secret := "provider-key=secret-user-840582614-execution-123"
	collector.RecordProviderSelection(context.Background(), ProviderSelectionMetricsObservation{
		Scope:        domainsandbox.Scope(secret),
		ProviderType: domainsandbox.ProviderType(secret),
		Outcome:      secret,
		ResultCode:   secret,
	})

	if got := testutil.ToFloat64(collector.providerSelectionsTotal.WithLabelValues(
		"unknown", "unknown", "failure", "unknown",
	)); got != 1 {
		t.Fatalf("sanitized provider selection metric = %v", got)
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if rendered := fmt.Sprintf("%v", families); strings.Contains(rendered, secret) ||
		strings.Contains(rendered, "840582614") ||
		strings.Contains(rendered, "execution-123") {
		t.Fatalf("sensitive metric label leaked: %s", rendered)
	}
}

func TestSandboxMetricsReusesCollectorsOnDuplicateRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()
	first, err := NewSandboxPrometheusMetricsCollector(registry)
	if err != nil {
		t.Fatalf("first collector error = %v", err)
	}
	second, err := NewSandboxPrometheusMetricsCollector(registry)
	if err != nil {
		t.Fatalf("second collector error = %v", err)
	}
	if first.providerSelectionsTotal != second.providerSelectionsTotal ||
		first.executionDurationMs != second.executionDurationMs {
		t.Fatal("duplicate registration did not reuse existing collectors")
	}
}

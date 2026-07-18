// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSandboxAuditListIsBoundedDeterministicAndSanitized(t *testing.T) {
	h := newControlPlaneHarness(t)
	h.providers.providers[91] = testProvider(91, 2)
	h.audits.events = []*domainsandbox.ProviderAuditEvent{
		{
			ID: 1, ProviderID: 91, ActorUserID: testActor().UserID,
			Action: auditActionUpdate, Result: auditResultSuccess, RequestID: "request-1",
			Metadata: map[string]string{
				domainsandbox.AuditMetadataKeyScope:   "agent",
				domainsandbox.AuditMetadataKeyVersion: "2",
				"credential":                          "credential-must-not-leak",
				"raw_config":                          "https://sandbox.example.test/private/path",
			},
		},
		{
			ID: 2, ProviderID: 91, ActorUserID: testActor().UserID,
			Action: auditActionHealthCheck, Result: auditResultFailure, RequestID: "request-2",
			Metadata: map[string]string{domainsandbox.AuditMetadataKeyHealthCode: healthCodeProbeFailed},
		},
	}

	page, err := h.service.ListAuditEvents(context.Background(), testActor(), ListAuditEventsRequest{
		ProviderID: 91, Limit: 1,
	})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != 2 {
		t.Fatalf("ListAuditEvents() = %#v, %v", page, err)
	}
	if _, exists := page.Items[0].Metadata["credential"]; exists {
		t.Fatalf("audit projection retained prohibited metadata: %#v", page.Items[0])
	}
	assertSanitizedJSON(t, page, "credential-must-not-leak", "private/path", "raw_config")

	if _, err := h.service.ListAuditEvents(context.Background(), testActor(), ListAuditEventsRequest{Limit: domainsandbox.MaxPageLimit + 1}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("ListAuditEvents(invalid) error = %v", err)
	}
	untrusted := testActor()
	untrusted.SystemAdmin = false
	if _, err := h.service.ListAuditEvents(context.Background(), untrusted, ListAuditEventsRequest{Limit: 1}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ListAuditEvents(untrusted) error = %v", err)
	}

	zero := newControlPlaneHarness(t)
	if _, err := zero.service.ListAuditEvents(context.Background(), testActor(), ListAuditEventsRequest{Limit: 1}); !errors.Is(err, domainsandbox.ErrInvalidInput) || zero.audits.listCalls != 0 {
		t.Fatalf("ListAuditEvents(zero provider) error/calls = %v/%d", err, zero.audits.listCalls)
	}
	limited := newControlPlaneHarness(t)
	provider := testProvider(92, 1)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	limited.providers.providers[provider.ID] = provider
	actor := testActor()
	actor.AllowedScopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	if _, err := limited.service.ListAuditEvents(context.Background(), actor, ListAuditEventsRequest{ProviderID: provider.ID, Limit: 1}); !errors.Is(err, ErrPermissionDenied) || limited.audits.listCalls != 0 {
		t.Fatalf("ListAuditEvents(scope denied) error/calls = %v/%d", err, limited.audits.listCalls)
	}
}

func TestSandboxAuditFailureRollsBackMutationAndReturnsSanitizedError(t *testing.T) {
	h := newControlPlaneHarness(t)
	h.audits.appendErr = errors.New("audit mysql URL credential-must-not-leak")

	_, err := h.service.Create(context.Background(), testActor(), validCreateRequest())
	if !errors.Is(err, domainsandbox.ErrUnavailable) || strings.Contains(err.Error(), "mysql") ||
		strings.Contains(err.Error(), "credential-must-not-leak") {
		t.Fatalf("Create(audit failure) error = %v", err)
	}
	if h.uow.committed || len(h.providers.providers) != 0 || len(h.audits.events) != 0 {
		t.Fatalf("audit failure did not roll back: committed=%v providers=%d audits=%d", h.uow.committed, len(h.providers.providers), len(h.audits.events))
	}
}

func TestSandboxAuditFailureRollsBackEveryControlPlaneMutation(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*controlPlaneHarness) int64
		mutate func(*controlPlaneHarness, int64) error
	}{
		{
			name: "update",
			setup: func(h *controlPlaneHarness) int64 { p := testProvider(201, 4); h.providers.providers[p.ID] = p; return p.ID },
			mutate: func(h *controlPlaneHarness, id int64) error {
				p := h.providers.providers[id]
				_, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{ProviderID: id, ExpectedVersion: p.Version, Name: "changed", Scopes: p.Scopes, Policy: p.Policy})
				return err
			},
		},
		{
			name: "disable",
			setup: func(h *controlPlaneHarness) int64 { p := testProvider(202, 2); p.Status = domainsandbox.ProviderStatusEnabled; h.providers.providers[p.ID] = p; return p.ID },
			mutate: func(h *controlPlaneHarness, id int64) error {
				_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{ProviderID: id, ExpectedVersion: 2, Status: domainsandbox.ProviderStatusDisabled})
				return err
			},
		},
		{
			name: "set default",
			setup: func(h *controlPlaneHarness) int64 {
				p := testProvider(203, 3)
				p.Status = domainsandbox.ProviderStatusEnabled
				p.Health = domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent}, ReasonCode: healthCodeOK, CheckedAt: time.Date(2026, 7, 15, 0, 59, 0, 0, time.UTC)}
				h.providers.providers[p.ID] = p
				return p.ID
			},
			mutate: func(h *controlPlaneHarness, id int64) error {
				_, err := h.service.SetDefault(context.Background(), testActor(), SetProviderDefaultRequest{ProviderID: id, ProviderExpectedVersion: 3, Scope: domainsandbox.ScopeAgent})
				return err
			},
		},
		{
			name: "delete",
			setup: func(h *controlPlaneHarness) int64 { p := testProvider(204, 4); h.providers.providers[p.ID] = p; return p.ID },
			mutate: func(h *controlPlaneHarness, id int64) error {
				_, err := h.service.Delete(context.Background(), testActor(), DeleteProviderRequest{ProviderID: id, ExpectedVersion: 4})
				return err
			},
		},
		{
			name: "health",
			setup: func(h *controlPlaneHarness) int64 { p := testProvider(205, 5); h.providers.providers[p.ID] = p; return p.ID },
			mutate: func(h *controlPlaneHarness, id int64) error {
				_, err := h.service.HealthCheck(context.Background(), testActor(), HealthCheckRequest{ProviderID: id, ExpectedVersion: 5})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControlPlaneHarness(t)
			id := tt.setup(h)
			before := cloneTestProvider(h.providers.providers[id])
			h.audits.appendErr = errors.New("audit endpoint credential raw body")
			err := tt.mutate(h, id)
			if !errors.Is(err, domainsandbox.ErrUnavailable) || h.uow.committed || len(h.audits.events) != 0 {
				t.Fatalf("mutation error/commit/audits = %v/%v/%d", err, h.uow.committed, len(h.audits.events))
			}
			if !reflect.DeepEqual(h.providers.providers[id], before) || len(h.defaults.defaults) != 0 {
				t.Fatalf("mutation escaped rollback: before=%#v after=%#v defaults=%#v", before, h.providers.providers[id], h.defaults.defaults)
			}
		})
	}
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestControlPlaneServiceCreateListGetSanitizesSecretsAndAudits(t *testing.T) {
	h := newControlPlaneHarness(t)

	if _, err := h.service.Create(context.Background(), testActor(), CreateProviderRequest{
		Name:       " ",
		Type:       domainsandbox.ProviderTypeRemoteHTTP,
		Endpoint:   []byte("https://sandbox.example.test"),
		Credential: []byte("credential-must-not-leak"),
		Scopes:     []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy:     testRuntimePolicy(),
	}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Create(invalid) error = %v", err)
	}
	if h.keyCalls != 0 || len(h.codec.encryptCalls) != 0 || h.uow.calls != 0 {
		t.Fatalf("invalid create reached key/codec/uow: %d/%d/%d", h.keyCalls, len(h.codec.encryptCalls), h.uow.calls)
	}

	created, err := h.service.Create(context.Background(), testActor(), CreateProviderRequest{
		Name:       "Primary Sandbox",
		Type:       domainsandbox.ProviderTypeRemoteHTTP,
		Endpoint:   []byte("https://sandbox.example.test"),
		Credential: []byte("credential-must-not-leak"),
		Scopes:     []domainsandbox.Scope{domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAgent},
		Policy:     testRuntimePolicy(),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	stored := h.providers.providers[created.ID]
	if stored == nil || stored.ProviderKey != testProviderKey || stored.EndpointSecret != "encrypted:endpoint" ||
		stored.CredentialSecret != "encrypted:credential" || stored.CredentialFingerprint != testFingerprint {
		t.Fatalf("stored provider = %#v", stored)
	}
	if created.EndpointHint != "https://***.test" ||
		!created.CredentialConfigured || created.CredentialFingerprint != testFingerprint || !created.Active || created.NeedsRewrap {
		t.Fatalf("created projection = %#v", created)
	}
	if h.uow.calls != 1 || h.uow.providerCreateCalls != 1 || !h.uow.committed || len(h.audits.events) != 1 {
		t.Fatalf("create transaction/audit = calls:%d provider-create:%d committed:%v audits:%d", h.uow.calls, h.uow.providerCreateCalls, h.uow.committed, len(h.audits.events))
	}
	if event := h.audits.events[0]; event.Action != auditActionCreate || event.ActorUserID != testActor().UserID ||
		event.Metadata[domainsandbox.AuditMetadataKeyVersion] != "1" {
		t.Fatalf("create audit = %#v", event)
	}

	h.providers.listOrder = []int64{created.ID}
	page, err := h.service.List(context.Background(), testActor(), ListProvidersRequest{Limit: 10})
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("List() = %#v, %v", page, err)
	}
	got, err := h.service.Get(context.Background(), testActor(), created.ID)
	if err != nil || got.ID != created.ID {
		t.Fatalf("Get() = %#v, %v", got, err)
	}

	assertSanitizedJSON(t, []any{created, page, got, h.audits.events},
		"credential-must-not-leak", "sandbox.example.test", "encrypted:endpoint", "encrypted:credential", testProviderKey)
	encoded, _ := json.Marshal(created)
	for _, field := range []string{"endpoint_hint", "credential_configured", "credential_fingerprint", "active", "needs_rewrap"} {
		if !strings.Contains(string(encoded), `"`+field+`"`) {
			t.Fatalf("secret projection omitted %q: %s", field, encoded)
		}
	}
	if strings.Contains(string(encoded), "credential_present") {
		t.Fatalf("legacy secret projection leaked into DTO: %s", encoded)
	}
}

func TestControlPlaneServiceRejectsUntrustedActorScopeAndDuplicateKey(t *testing.T) {
	h := newControlPlaneHarness(t)
	request := validCreateRequest()

	untrusted := testActor()
	untrusted.SystemAdmin = false
	if _, err := h.service.Create(context.Background(), untrusted, request); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Create(untrusted) error = %v", err)
	}
	if h.uow.calls != 0 || len(h.codec.encryptCalls) != 0 {
		t.Fatal("untrusted create reached dependencies")
	}

	scopeLimited := testActor()
	scopeLimited.AllowedScopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	request.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	if _, err := h.service.Create(context.Background(), scopeLimited, request); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Create(scope denied) error = %v", err)
	}

	if _, err := h.service.List(context.Background(), testActor(), ListProvidersRequest{Limit: domainsandbox.MaxPageLimit + 1}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("List(invalid limit) error = %v", err)
	}
	if h.providers.listCalls != 0 {
		t.Fatal("invalid list reached repository")
	}

	h.providers.createErr = domainsandbox.ErrProviderAlreadyExists
	if _, err := h.service.Create(context.Background(), testActor(), validCreateRequest()); !errors.Is(err, domainsandbox.ErrProviderAlreadyExists) || err.Error() != domainsandbox.ErrProviderAlreadyExists.Error() {
		t.Fatalf("Create(duplicate) error = %v", err)
	}
}

func TestControlPlaneServiceUpdateUsesCASAndSecretMutationSemantics(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(41, 4)
	h.providers.providers[provider.ID] = provider

	policy := testRuntimePolicy()
	policy.TimeoutSeconds = 90
	policy.MaxConcurrency = 8
	updated, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID:      provider.ID,
		ExpectedVersion: 4,
		Name:            "Updated Sandbox",
		Scopes:          []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio},
		Policy:          policy,
		Endpoint: SecretMutation{
			Mode:  SecretMutationReplace,
			Value: []byte("https://rotated.example.test"),
		},
		Credential: SecretMutation{Mode: SecretMutationReplace, Value: []byte("rotated-secret")},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	stored := h.providers.providers[provider.ID]
	if stored.ProviderKey != provider.ProviderKey || stored.Version != 5 || stored.EndpointSecret != "encrypted:endpoint" ||
		stored.CredentialSecret != "encrypted:credential" || stored.CredentialFingerprint != testFingerprint {
		t.Fatalf("updated provider = %#v", stored)
	}
	if updated.CredentialFingerprint != testFingerprint {
		t.Fatalf("updated projection = %#v", updated)
	}
	wantChanged := "credential,endpoint,name,policy,scopes"
	if got := h.audits.events[0].Metadata[domainsandbox.AuditMetadataKeyChangedFields]; got != wantChanged {
		t.Fatalf("changed_fields = %q, want %q", got, wantChanged)
	}
	assertSanitizedJSON(t, []any{updated, h.audits.events[0]}, "rotated-secret", "rotated.example.test", "encrypted:endpoint", "encrypted:credential")

	cleared, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID:      provider.ID,
		ExpectedVersion: 5,
		Name:            stored.Name,
		Scopes:          stored.Scopes,
		Policy:          stored.Policy,
		Credential:      SecretMutation{Mode: SecretMutationClear},
	})
	if err != nil {
		t.Fatalf("Update(clear credential) error = %v", err)
	}
	if cleared.CredentialConfigured || cleared.CredentialFingerprint != "" || h.providers.providers[provider.ID].CredentialSecret != "" {
		t.Fatalf("cleared credential projection/storage = %#v / %#v", cleared, h.providers.providers[provider.ID])
	}
	if got := h.audits.events[1].Metadata[domainsandbox.AuditMetadataKeyChangedFields]; got != "credential" {
		t.Fatalf("clear changed_fields = %q", got)
	}

	if _, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID:      provider.ID,
		ExpectedVersion: 5,
		Name:            stored.Name,
		Scopes:          stored.Scopes,
		Policy:          stored.Policy,
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("Update(stale) error = %v", err)
	}

	h.providers.providers[provider.ID].Status = domainsandbox.ProviderStatusEnabled
	if _, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID:      provider.ID,
		ExpectedVersion: 6,
		Name:            stored.Name,
		Scopes:          stored.Scopes,
		Policy:          stored.Policy,
		Credential:      SecretMutation{Mode: SecretMutationClear},
	}); !errors.Is(err, domainsandbox.ErrProviderInUse) {
		t.Fatalf("Update(clear enabled credential) error = %v", err)
	}
}

func TestControlPlaneServiceExecutionChangesInvalidateHealth(t *testing.T) {
	healthy := func() domainsandbox.HealthSnapshot {
		return domainsandbox.HealthSnapshot{
			Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
			ReasonCode: healthCodeOK, LatencyMillis: 10,
			CheckedAt: time.Date(2026, 7, 15, 0, 59, 0, 0, time.UTC),
		}
	}

	displayHarness := newControlPlaneHarness(t)
	displayProvider := testProvider(42, 4)
	normalizedDisplayPolicy, normalizeErr := domainsandbox.NormalizeRuntimePolicy(displayProvider.Policy)
	if normalizeErr != nil {
		t.Fatalf("NormalizeRuntimePolicy() error = %v", normalizeErr)
	}
	displayProvider.Policy = normalizedDisplayPolicy
	displayProvider.Health = healthy()
	displayHarness.providers.providers[displayProvider.ID] = displayProvider
	displayResult, err := displayHarness.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID: displayProvider.ID, ExpectedVersion: displayProvider.Version, Name: "Display Name",
		Scopes: displayProvider.Scopes, Policy: displayProvider.Policy,
	})
	if err != nil || displayResult.Health.Status != domainsandbox.HealthStatusHealthy ||
		displayResult.Health.CheckedAt.IsZero() {
		t.Fatalf("display-only update health = %#v, %v", displayResult, err)
	}

	tests := []struct {
		name   string
		mutate func(*UpdateProviderRequest)
	}{
		{name: "endpoint", mutate: func(request *UpdateProviderRequest) {
			request.Endpoint = SecretMutation{Mode: SecretMutationReplace, Value: []byte("https://changed.example.test")}
		}},
		{name: "credential", mutate: func(request *UpdateProviderRequest) {
			request.Credential = SecretMutation{Mode: SecretMutationReplace, Value: []byte("changed-credential")}
		}},
		{name: "policy", mutate: func(request *UpdateProviderRequest) {
			request.Policy.MaxConcurrency++
		}},
		{name: "scopes", mutate: func(request *UpdateProviderRequest) {
			request.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev}
		}},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newControlPlaneHarness(t)
			provider := testProvider(int64(43+index), 4)
			provider.Health = healthy()
			h.providers.providers[provider.ID] = provider
			request := UpdateProviderRequest{
				ProviderID: provider.ID, ExpectedVersion: provider.Version, Name: provider.Name,
				Scopes: provider.Scopes, Policy: provider.Policy,
			}
			tt.mutate(&request)
			result, err := h.service.Update(context.Background(), testActor(), request)
			if err != nil || result.Health.Status != domainsandbox.HealthStatusUnknown ||
				!result.Health.CheckedAt.IsZero() || result.Health.LatencyBucket != healthLatencyBucketUnknown {
				t.Fatalf("execution update health = %#v, %v", result, err)
			}
			stored := h.providers.providers[provider.ID]
			stored.Status = domainsandbox.ProviderStatusEnabled
			if _, err := h.service.SetDefault(context.Background(), testActor(), SetProviderDefaultRequest{
				ProviderID: stored.ID, ProviderExpectedVersion: stored.Version, Scope: domainsandbox.ScopeAgent,
			}); !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
				t.Fatalf("SetDefault(after execution update) error = %v", err)
			}
		})
	}

	conflictHarness := newControlPlaneHarness(t)
	concurrent := testProvider(49, 5)
	concurrent.Health = healthy()
	conflictHarness.providers.providers[concurrent.ID] = concurrent
	if _, err := conflictHarness.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID: concurrent.ID, ExpectedVersion: 4, Name: concurrent.Name,
		Scopes: concurrent.Scopes, Policy: concurrent.Policy,
		Credential: SecretMutation{Mode: SecretMutationReplace, Value: []byte("stale-credential")},
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("Update(stale health version) error = %v", err)
	}
	if conflictHarness.providers.providers[concurrent.ID].Health.Status != domainsandbox.HealthStatusHealthy {
		t.Fatal("stale update overwrote concurrent health")
	}
}

func TestControlPlaneServiceSetStatusUsesActiveLeaseGuardAndAudit(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(51, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider

	h.leases.active = true
	if _, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: 2, Status: domainsandbox.ProviderStatusDisabled,
	}); !errors.Is(err, domainsandbox.ErrProviderInUse) {
		t.Fatalf("SetStatus(active lease) error = %v", err)
	}
	if h.providers.providers[provider.ID].Version != 2 || len(h.audits.events) != 0 || h.uow.committed {
		t.Fatal("active-lease denial mutated state")
	}

	h.leases.active = false
	h.leases.err = errors.New("redis endpoint and lease token must not leak")
	if _, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: 2, Status: domainsandbox.ProviderStatusDisabled,
	}); !errors.Is(err, domainsandbox.ErrUnavailable) || strings.Contains(err.Error(), "redis") {
		t.Fatalf("SetStatus(lease unknown) error = %v", err)
	}

	h.leases.err = nil
	disabled, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: 2, Status: domainsandbox.ProviderStatusDisabled,
	})
	if err != nil || disabled.Status != domainsandbox.ProviderStatusDisabled || disabled.Version != 3 {
		t.Fatalf("SetStatus(disable) = %#v, %v", disabled, err)
	}
	audit := h.audits.events[0]
	if audit.Metadata[domainsandbox.AuditMetadataKeyPreviousStatus] != "enabled" ||
		audit.Metadata[domainsandbox.AuditMetadataKeyNewStatus] != "disabled" ||
		audit.Metadata[domainsandbox.AuditMetadataKeyVersion] != "3" {
		t.Fatalf("status audit = %#v", audit)
	}

	leaseCalls := h.leases.calls
	enabled, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: 3, Status: domainsandbox.ProviderStatusEnabled,
	})
	if err != nil || enabled.Status != domainsandbox.ProviderStatusEnabled || enabled.Version != 4 {
		t.Fatalf("SetStatus(enable) = %#v, %v", enabled, err)
	}
	if h.leases.calls != leaseCalls {
		t.Fatal("enable unexpectedly queried active leases")
	}
}

func TestControlPlaneServiceDrainFenceCompensatesLifecycleFailures(t *testing.T) {
	t.Run("active disable restores active at the new epoch", func(t *testing.T) {
		h := newControlPlaneHarness(t)
		provider := testProvider(52, 2)
		provider.Status = domainsandbox.ProviderStatusEnabled
		h.providers.providers[provider.ID] = provider
		h.leases.active = true
		_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
		})
		if !errors.Is(err, domainsandbox.ErrProviderInUse) || h.leases.beginCalls != 1 ||
			h.leases.restoreCalls != 1 || h.leases.draining || h.leases.activeGeneration == "" {
			t.Fatalf("active disable error/drain = %v/%#v", err, h.leases)
		}
	})

	t.Run("audit failure restores active at the new epoch", func(t *testing.T) {
		h := newControlPlaneHarness(t)
		provider := testProvider(53, 2)
		provider.Status = domainsandbox.ProviderStatusEnabled
		h.providers.providers[provider.ID] = provider
		h.audits.appendErr = errors.New("audit unavailable")
		_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
		})
		if !errors.Is(err, domainsandbox.ErrUnavailable) || h.leases.restoreCalls != 1 || h.leases.draining ||
			h.leases.activeGeneration == "" ||
			h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusEnabled {
			t.Fatalf("audit failure error/drain/provider = %v/%#v/%#v", err, h.leases, h.providers.providers[provider.ID])
		}
	})

	t.Run("restore failure stays fail closed", func(t *testing.T) {
		h := newControlPlaneHarness(t)
		provider := testProvider(54, 2)
		provider.Status = domainsandbox.ProviderStatusEnabled
		h.providers.providers[provider.ID] = provider
		h.leases.active = true
		h.leases.restoreErr = errors.New("redis unavailable")
		_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
		})
		if !errors.Is(err, domainsandbox.ErrUnavailable) || !h.leases.draining {
			t.Fatalf("restore failure error/drain = %v/%#v", err, h.leases)
		}
	})

	t.Run("delete failure and success both retain the latest drain", func(t *testing.T) {
		h := newControlPlaneHarness(t)
		provider := testProvider(55, 4)
		h.providers.providers[provider.ID] = provider
		h.audits.appendErr = errors.New("audit unavailable")
		_, err := h.service.Delete(context.Background(), testActor(), DeleteProviderRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version,
		})
		if !errors.Is(err, domainsandbox.ErrUnavailable) || h.leases.retainCalls != 1 || !h.leases.draining {
			t.Fatalf("Delete(audit failure) error/drain = %v/%#v", err, h.leases)
		}
		h.audits.appendErr = nil
		result, err := h.service.Delete(context.Background(), testActor(), DeleteProviderRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version,
		})
		if err != nil || result.Version != provider.Version+1 || !h.leases.draining {
			t.Fatalf("Delete(success) = %#v, %v, drain=%#v", result, err, h.leases)
		}
	})
}

func TestControlPlaneServiceEnableActivationFailureKeepsDatabaseDisabledAndCanRetry(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(56, 2)
	h.providers.providers[provider.ID] = provider
	h.leases.draining = true
	h.leases.fence = "existing-drain-fence"
	h.leases.activateErr = errors.New("redis unavailable")
	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled ||
		h.providers.providers[provider.ID].Version != provider.Version || !h.leases.draining || h.uow.calls != 0 || len(h.audits.events) != 0 {
		t.Fatalf("enable activation failure = %v, provider=%#v drain=%#v", err, h.providers.providers[provider.ID], h.leases)
	}
	h.leases.activateErr = nil
	result, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if err != nil || result.Status != domainsandbox.ProviderStatusEnabled || result.Version != 3 ||
		h.leases.draining || h.leases.activateCalls != 2 || len(h.audits.events) != 1 {
		t.Fatalf("enable activation retry = %#v, %v, drain=%#v", result, err, h.leases)
	}
}

func TestControlPlaneServiceRepeatedDisableReplacesDrainGate(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(561, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider

	first, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	if err != nil || first.Status != domainsandbox.ProviderStatusDisabled || !h.leases.draining {
		t.Fatalf("first disable = %#v, %v, drain=%#v", first, err, h.leases)
	}
	firstFence := h.leases.fence
	second, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: first.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	if err != nil || second.Version != first.Version || second.Status != domainsandbox.ProviderStatusDisabled ||
		!h.leases.draining || h.leases.fence == firstFence || h.leases.beginCalls != 2 {
		t.Fatalf("repeated disable = %#v, %v, drain=%#v", second, err, h.leases)
	}
}

func TestControlPlaneServiceDisableThenDeleteReplacesRetainedDrain(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(562, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider

	disabled, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	if err != nil {
		t.Fatalf("disable error = %v", err)
	}
	disableFence := h.leases.fence
	deleted, err := h.service.Delete(context.Background(), testActor(), DeleteProviderRequest{
		ProviderID: provider.ID, ExpectedVersion: disabled.Version,
	})
	if err != nil || deleted.Version != disabled.Version+1 || h.providers.providers[provider.ID].DeletedAt == nil ||
		!h.leases.draining || h.leases.fence == disableFence {
		t.Fatalf("delete after disable = %#v, %v, drain=%#v", deleted, err, h.leases)
	}
}

func TestControlPlaneServiceStaleSameStatusEnableDoesNotTouchDrain(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(563, 4)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	h.leases.draining = true
	h.leases.fence = "retained-current-gate"

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version - 1, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrVersionConflict) || h.leases.beginCalls != 0 || h.leases.activateCalls != 0 ||
		!h.leases.draining || h.leases.fence != "retained-current-gate" {
		t.Fatalf("stale same-status enable = %v, drain=%#v", err, h.leases)
	}
}

func TestControlPlaneServiceEnableActivateMismatchKeepsConcurrentDrain(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(564, 2)
	h.providers.providers[provider.ID] = provider
	var concurrentFence DrainFence
	h.leases.beforeActivate = func(guard *testLeaseActivityChecker) {
		guard.sequence++
		concurrentFence = DrainFence(fmt.Sprintf("concurrent-drain-%d", guard.sequence))
		guard.draining = true
		guard.fence = concurrentFence
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled ||
		!h.leases.draining || h.leases.fence != concurrentFence || h.leases.activateCalls != 1 {
		t.Fatalf("enable activate mismatch = %v, provider=%#v, drain=%#v", err, h.providers.providers[provider.ID], h.leases)
	}
}

func TestControlPlaneServiceEnableRequiresCanonicalActiveRemoteSecrets(t *testing.T) {
	setTask9CapabilityEnvironment(t, true)
	h := newControlPlaneHarness(t)
	provider := testProvider(57, 4)
	h.providers.providers[provider.ID] = provider
	cleared, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Name: provider.Name,
		Scopes: provider.Scopes, Policy: provider.Policy,
		Credential: SecretMutation{Mode: SecretMutationClear},
	})
	if err != nil {
		t.Fatalf("Update(clear credential) error = %v", err)
	}
	if _, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: cleared.Version, Status: domainsandbox.ProviderStatusEnabled,
	}); !errors.Is(err, domainsandbox.ErrCredentialInvalid) {
		t.Fatalf("SetStatus(enable missing credential) error = %v", err)
	}
	restored, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID: provider.ID, ExpectedVersion: cleared.Version, Name: provider.Name,
		Scopes: provider.Scopes, Policy: provider.Policy,
		Credential: SecretMutation{Mode: SecretMutationReplace, Value: []byte("restored-credential")},
	})
	if err != nil {
		t.Fatalf("Update(restore credential) error = %v", err)
	}
	if _, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: restored.Version, Status: domainsandbox.ProviderStatusEnabled,
	}); err != nil {
		t.Fatalf("SetStatus(enable restored credential) error = %v", err)
	}

	malformed := newControlPlaneHarness(t)
	malformedProvider := testProvider(58, 2)
	malformed.providers.providers[malformedProvider.ID] = malformedProvider
	malformed.codec.inspectErr = errors.New("malformed envelope")
	if _, err := malformed.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: malformedProvider.ID, ExpectedVersion: malformedProvider.Version, Status: domainsandbox.ProviderStatusEnabled,
	}); !errors.Is(err, domainsandbox.ErrCredentialInvalid) {
		t.Fatalf("SetStatus(enable malformed envelope) error = %v", err)
	}

	local := newControlPlaneHarness(t)
	localProvider := testProvider(59, 1)
	localProvider.Type = domainsandbox.ProviderTypeLocalDebug
	localProvider.EndpointSecret = ""
	localProvider.EndpointHint = ""
	localProvider.CredentialSecret = ""
	localProvider.CredentialFingerprint = ""
	local.providers.providers[localProvider.ID] = localProvider
	if _, err := local.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: localProvider.ID, ExpectedVersion: localProvider.Version, Status: domainsandbox.ProviderStatusEnabled,
	}); err != nil {
		t.Fatalf("SetStatus(enable local debug) error = %v", err)
	}
}

func TestControlPlaneServiceSetDefaultMaintainsEligibilityInvariant(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(61, 7)
	provider.Status = domainsandbox.ProviderStatusEnabled
	provider.Health = domainsandbox.HealthSnapshot{
		Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		ReasonCode: healthCodeOK, CheckedAt: time.Date(2026, 7, 15, 0, 59, 0, 0, time.UTC),
	}
	h.providers.providers[provider.ID] = provider

	result, err := h.service.SetDefault(context.Background(), testActor(), SetProviderDefaultRequest{
		ProviderID: provider.ID, ProviderExpectedVersion: 7, Scope: domainsandbox.ScopeAgent, ExpectedVersion: 0,
	})
	if err != nil || result.ProviderID != provider.ID || result.Scope != domainsandbox.ScopeAgent || result.Version != 1 {
		t.Fatalf("SetDefault() = %#v, %v", result, err)
	}
	if current := h.defaults.defaults[domainsandbox.ScopeAgent]; current == nil || current.ProviderID != provider.ID {
		t.Fatalf("default state = %#v", current)
	}
	if audit := h.audits.events[0]; audit.Metadata[domainsandbox.AuditMetadataKeyScope] != "agent" ||
		audit.Metadata[domainsandbox.AuditMetadataKeyVersion] != "1" {
		t.Fatalf("default audit = %#v", audit)
	}
	if h.providers.forUpdateCalls == 0 {
		t.Fatal("SetDefault did not lock provider before eligibility validation")
	}

	tests := []struct {
		name   string
		mutate func(*domainsandbox.Provider)
		want   error
	}{
		{name: "disabled", mutate: func(p *domainsandbox.Provider) { p.Status = domainsandbox.ProviderStatusDisabled }, want: domainsandbox.ErrProviderDisabled},
		{name: "unsupported scope", mutate: func(p *domainsandbox.Provider) { p.Scopes = []domainsandbox.Scope{domainsandbox.ScopeMCPStdio} }, want: domainsandbox.ErrScopeUnsupported},
		{name: "unhealthy", mutate: func(p *domainsandbox.Provider) { p.Health.Status = domainsandbox.HealthStatusUnhealthy }, want: domainsandbox.ErrProviderUnhealthy},
		{name: "stale", mutate: func(p *domainsandbox.Provider) { p.Health.CheckedAt = time.Date(2026, 7, 15, 0, 54, 59, 0, time.UTC) }, want: domainsandbox.ErrProviderUnhealthy},
		{name: "future", mutate: func(p *domainsandbox.Provider) { p.Health.CheckedAt = time.Date(2026, 7, 15, 1, 0, 0, 1, time.UTC) }, want: domainsandbox.ErrProviderUnhealthy},
		{name: "unchecked", mutate: func(p *domainsandbox.Provider) { p.Health.CheckedAt = time.Time{} }, want: domainsandbox.ErrProviderUnhealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseHarness := newControlPlaneHarness(t)
			candidate := cloneTestProvider(provider)
			tt.mutate(candidate)
			caseHarness.providers.providers[candidate.ID] = candidate
			_, err := caseHarness.service.SetDefault(context.Background(), testActor(), SetProviderDefaultRequest{
				ProviderID: candidate.ID, ProviderExpectedVersion: candidate.Version,
				Scope: domainsandbox.ScopeAgent, ExpectedVersion: 0,
			})
			if !errors.Is(err, tt.want) || len(caseHarness.audits.events) != 0 || caseHarness.uow.committed {
				t.Fatalf("SetDefault() error/audit/commit = %v/%d/%v", err, len(caseHarness.audits.events), caseHarness.uow.committed)
			}
		})
	}
}

func TestControlPlaneServiceDefaultProviderRejectsIneligibleMutation(t *testing.T) {
	setup := func(t *testing.T) (*controlPlaneHarness, *domainsandbox.Provider) {
		t.Helper()
		h := newControlPlaneHarness(t)
		provider := testProvider(62, 3)
		provider.Status = domainsandbox.ProviderStatusEnabled
		provider.Health = domainsandbox.HealthSnapshot{
			Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
			ReasonCode: healthCodeOK, CheckedAt: time.Date(2026, 7, 15, 0, 59, 0, 0, time.UTC),
		}
		h.providers.providers[provider.ID] = provider
		h.defaults.defaults[domainsandbox.ScopeAgent] = &domainsandbox.ProviderDefault{
			Scope: domainsandbox.ScopeAgent, ProviderID: provider.ID, Version: 1,
		}
		return h, provider
	}

	t.Run("disable", func(t *testing.T) {
		h, provider := setup(t)
		_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
		})
		if !errors.Is(err, domainsandbox.ErrProviderInUse) || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusEnabled {
			t.Fatalf("disable default error/provider = %v/%#v", err, h.providers.providers[provider.ID])
		}
	})

	t.Run("execution update", func(t *testing.T) {
		h, provider := setup(t)
		policy := provider.Policy
		policy.MaxConcurrency++
		_, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Name: provider.Name,
			Scopes: provider.Scopes, Policy: policy,
		})
		if !errors.Is(err, domainsandbox.ErrProviderInUse) || h.providers.providers[provider.ID].Version != provider.Version ||
			h.providers.providers[provider.ID].Health.Status != domainsandbox.HealthStatusHealthy {
			t.Fatalf("update default error/provider = %v/%#v", err, h.providers.providers[provider.ID])
		}
	})
}

func TestControlPlaneServiceCanonicalizesActorScopes(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(63, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	provider.Health = domainsandbox.HealthSnapshot{
		Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		ReasonCode: healthCodeOK, CheckedAt: time.Date(2026, 7, 15, 0, 59, 0, 0, time.UTC),
	}
	h.providers.providers[provider.ID] = provider
	actor := testActor()
	actor.AllowedScopes = []domainsandbox.Scope{" agent ", domainsandbox.ScopeAgent}
	got, err := h.service.Get(context.Background(), actor, provider.ID)
	if err != nil || !reflect.DeepEqual(got.Scopes, []domainsandbox.Scope{domainsandbox.ScopeAgent}) {
		t.Fatalf("Get(canonical actor scopes) = %#v, %v", got, err)
	}
	if _, err := h.service.SetDefault(context.Background(), actor, SetProviderDefaultRequest{
		ProviderID: provider.ID, ProviderExpectedVersion: provider.Version, Scope: domainsandbox.ScopeAgent,
	}); err != nil {
		t.Fatalf("SetDefault(canonical actor scopes) error = %v", err)
	}
	if got := h.audits.events[0].Metadata[domainsandbox.AuditMetadataKeyScope]; got != "agent" {
		t.Fatalf("canonical scope audit = %q", got)
	}
}

func TestControlPlaneServiceDeleteUsesSoftDeleteDefaultAndLeaseGuards(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(71, 4)
	h.providers.providers[provider.ID] = provider

	result, err := h.service.Delete(context.Background(), testActor(), DeleteProviderRequest{
		ProviderID: provider.ID, ExpectedVersion: 4,
	})
	if err != nil || result.ProviderID != provider.ID || result.Version != 5 {
		t.Fatalf("Delete() = %#v, %v", result, err)
	}
	if stored := h.providers.providers[provider.ID]; stored == nil || stored.DeletedAt == nil {
		t.Fatalf("delete was not tombstoned: %#v", stored)
	}
	if len(h.audits.events) != 1 || h.audits.events[0].Action != auditActionDelete {
		t.Fatalf("delete audit = %#v", h.audits.events)
	}

	tests := []struct {
		name      string
		configure func(*controlPlaneHarness, *domainsandbox.Provider)
		want      error
	}{
		{name: "enabled", configure: func(_ *controlPlaneHarness, p *domainsandbox.Provider) {
			p.Status = domainsandbox.ProviderStatusEnabled
		}, want: domainsandbox.ErrProviderInUse},
		{name: "default", configure: func(h *controlPlaneHarness, p *domainsandbox.Provider) {
			h.defaults.defaults[domainsandbox.ScopeAgent] = &domainsandbox.ProviderDefault{Scope: domainsandbox.ScopeAgent, ProviderID: p.ID, Version: 1}
		}, want: domainsandbox.ErrProviderInUse},
		{name: "active lease", configure: func(h *controlPlaneHarness, _ *domainsandbox.Provider) { h.leases.active = true }, want: domainsandbox.ErrProviderInUse},
		{name: "lease unknown", configure: func(h *controlPlaneHarness, _ *domainsandbox.Provider) { h.leases.err = errors.New("redis secret") }, want: domainsandbox.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseHarness := newControlPlaneHarness(t)
			candidate := testProvider(72, 4)
			caseHarness.providers.providers[candidate.ID] = candidate
			tt.configure(caseHarness, candidate)
			_, err := caseHarness.service.Delete(context.Background(), testActor(), DeleteProviderRequest{
				ProviderID: candidate.ID, ExpectedVersion: candidate.Version,
			})
			if !errors.Is(err, tt.want) || caseHarness.providers.providers[candidate.ID].DeletedAt != nil || len(caseHarness.audits.events) != 0 {
				t.Fatalf("Delete() error/deleted/audit = %v/%v/%d", err, caseHarness.providers.providers[candidate.ID].DeletedAt, len(caseHarness.audits.events))
			}
		})
	}
}

func TestControlPlaneServiceListPushesFiniteAuthorizedScopesIntoRepository(t *testing.T) {
	h := newControlPlaneHarness(t)
	agent := testProvider(101, 1)
	agent.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	mcp := testProvider(102, 1)
	mcp.Scopes = []domainsandbox.Scope{domainsandbox.ScopeMCPStdio}
	appdev := testProvider(103, 1)
	appdev.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	h.providers.providers[agent.ID] = agent
	h.providers.providers[mcp.ID] = mcp
	h.providers.providers[appdev.ID] = appdev
	h.providers.listOrder = []int64{appdev.ID, mcp.ID, agent.ID}

	actor := testActor()
	actor.AllowedScopes = []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio}
	page, err := h.service.List(context.Background(), actor, ListProvidersRequest{Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != mcp.ID {
		t.Fatalf("List(scope-limited page) = %#v, %v", page, err)
	}
	page, err = h.service.List(context.Background(), actor, ListProvidersRequest{Offset: 1, Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != agent.ID {
		t.Fatalf("List(scope-limited second page) = %#v, %v", page, err)
	}
	listCalls := h.providers.listCalls
	if _, err := h.service.List(context.Background(), actor, ListProvidersRequest{Scope: domainsandbox.ScopeAppDev, Limit: 10}); !errors.Is(err, ErrPermissionDenied) || h.providers.listCalls != listCalls {
		t.Fatalf("List(unauthorized requested scope) error/calls = %v/%d", err, h.providers.listCalls)
	}
	empty := testActor()
	empty.AllowedScopes = nil
	if _, err := h.service.List(context.Background(), empty, ListProvidersRequest{Limit: 10}); !errors.Is(err, ErrPermissionDenied) || h.providers.listCalls != listCalls {
		t.Fatalf("List(empty actor scopes) error/calls = %v/%d", err, h.providers.listCalls)
	}
}

func TestControlPlaneServiceListRequiresProviderScopesToBeAuthorizedSubset(t *testing.T) {
	h := newControlPlaneHarness(t)
	systemOlder := testProvider(105, 1)
	systemOlder.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	workspaceOnly := testProvider(106, 1)
	workspaceOnly.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	mixed := testProvider(107, 1)
	mixed.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev}
	systemNewest := testProvider(108, 1)
	systemNewest.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	for _, provider := range []*domainsandbox.Provider{systemOlder, workspaceOnly, mixed, systemNewest} {
		h.providers.providers[provider.ID] = provider
	}
	h.providers.listOrder = []int64{systemNewest.ID, mixed.ID, workspaceOnly.ID, systemOlder.ID}

	actor := testActor()
	actor.AllowedScopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	first, err := h.service.List(context.Background(), actor, ListProvidersRequest{Limit: 1})
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].ID != systemNewest.ID {
		t.Fatalf("List(first authorized subset page) = %#v, %v", first, err)
	}
	second, err := h.service.List(context.Background(), actor, ListProvidersRequest{Offset: 1, Limit: 1})
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID != systemOlder.ID {
		t.Fatalf("List(second authorized subset page) = %#v, %v", second, err)
	}
	singleScope, err := h.service.List(context.Background(), actor, ListProvidersRequest{
		Scope: domainsandbox.ScopeAgent,
		Limit: 10,
	})
	if err != nil || singleScope.Total != 2 || len(singleScope.Items) != 2 ||
		singleScope.Items[0].ID != systemNewest.ID || singleScope.Items[1].ID != systemOlder.ID {
		t.Fatalf("List(single scope authorized subset) = %#v, %v", singleScope, err)
	}
}

func TestControlPlaneServiceListCombinesRequestedAndAuthorizedScopes(t *testing.T) {
	h := newControlPlaneHarness(t)
	agentOnly := testProvider(109, 1)
	agentOnly.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
	agentAppDev := testProvider(110, 1)
	agentAppDev.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev}
	appDevOnly := testProvider(111, 1)
	appDevOnly.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	agentUnauthorized := testProvider(112, 1)
	agentUnauthorized.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio}
	for _, provider := range []*domainsandbox.Provider{agentOnly, agentAppDev, appDevOnly, agentUnauthorized} {
		h.providers.providers[provider.ID] = provider
	}
	h.providers.listOrder = []int64{agentUnauthorized.ID, appDevOnly.ID, agentAppDev.ID, agentOnly.ID}

	actor := testActor()
	actor.AllowedScopes = []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev}
	first, err := h.service.List(context.Background(), actor, ListProvidersRequest{
		Scope: domainsandbox.ScopeAgent,
		Limit: 1,
	})
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].ID != agentAppDev.ID {
		t.Fatalf("List(first combined scope page) = %#v, %v", first, err)
	}
	second, err := h.service.List(context.Background(), actor, ListProvidersRequest{
		Scope:  domainsandbox.ScopeAgent,
		Offset: 1,
		Limit:  1,
	})
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].ID != agentOnly.ID {
		t.Fatalf("List(second combined scope page) = %#v, %v", second, err)
	}
	withoutRequestedScope, err := h.service.List(context.Background(), actor, ListProvidersRequest{Limit: 10})
	if err != nil || withoutRequestedScope.Total != 3 || len(withoutRequestedScope.Items) != 3 ||
		withoutRequestedScope.Items[0].ID != appDevOnly.ID ||
		withoutRequestedScope.Items[1].ID != agentAppDev.ID ||
		withoutRequestedScope.Items[2].ID != agentOnly.ID {
		t.Fatalf("List(authorized subset without requested scope) = %#v, %v", withoutRequestedScope, err)
	}
}

func TestControlPlaneServiceSecretPreserveDoesNotReencryptOrExposeStoredValues(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(104, 4)
	h.providers.providers[provider.ID] = provider
	result, err := h.service.Update(context.Background(), testActor(), UpdateProviderRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version,
		Name: provider.Name, Scopes: provider.Scopes, Policy: provider.Policy,
		Endpoint: SecretMutation{Mode: SecretMutationKeep}, Credential: SecretMutation{Mode: SecretMutationKeep},
	})
	if err != nil {
		t.Fatalf("Update(preserve secrets) error = %v", err)
	}
	if len(h.codec.encryptCalls) != 0 || !result.CredentialConfigured || result.EndpointHint != "https://***.test" {
		t.Fatalf("preserve re-encrypted or projected incorrectly: calls=%d result=%#v", len(h.codec.encryptCalls), result)
	}
	assertSanitizedJSON(t, result, provider.EndpointSecret, provider.CredentialSecret, "sandbox.example.test")
}

const (
	testProviderKey = "018f0d2e-7b73-7e21-9a89-1a2b3c4d5e6f"
	testFingerprint = "0123456789abcdef0123456789abcdef"
)

type controlPlaneHarness struct {
	service   *Service
	providers *testProviderRepository
	defaults  *testDefaultRepository
	audits    *testAuditRepository
	uow       *testUnitOfWork
	codec     *testCredentialCodec
	leases    *testLeaseActivityChecker
	factory   *testHealthProviderFactory
	keyCalls  int
}

func newControlPlaneHarness(t *testing.T) *controlPlaneHarness {
	t.Helper()
	h := &controlPlaneHarness{
		providers: &testProviderRepository{providers: make(map[int64]*domainsandbox.Provider)},
		defaults:  &testDefaultRepository{defaults: make(map[domainsandbox.Scope]*domainsandbox.ProviderDefault)},
		audits:    &testAuditRepository{},
		codec:     &testCredentialCodec{},
		leases:    &testLeaseActivityChecker{},
		factory: &testHealthProviderFactory{provider: &testHealthProvider{result: infrasandbox.HealthResult{
			ProtocolVersion: infrasandbox.HealthProtocolV1,
			Status:          domainsandbox.HealthStatusHealthy,
			Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAgent},
		}}},
	}
	h.uow = &testUnitOfWork{providers: h.providers, defaults: h.defaults, audits: h.audits}
	service, err := NewService(ServiceOptions{
		Providers:  h.providers,
		UnitOfWork: h.uow,
		Codec:      h.codec,
		Leases:     h.leases,
		Factory:    h.factory,
		ProviderKey: func(Actor) (string, error) {
			h.keyCalls++
			return testProviderKey, nil
		},
		Now: func() time.Time { return time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	h.service = service
	return h
}

func testActor() Actor {
	return Actor{
		UserID:        840582614,
		RequestID:     "request-task-6",
		SystemAdmin:   true,
		AllowedScopes: []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAppDev},
	}
}

func validCreateRequest() CreateProviderRequest {
	return CreateProviderRequest{
		Name:       "Primary Sandbox",
		Type:       domainsandbox.ProviderTypeRemoteHTTP,
		Endpoint:   []byte("https://sandbox.example.test"),
		Credential: []byte("credential-must-not-leak"),
		Scopes:     []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy:     testRuntimePolicy(),
	}
}

func testRuntimePolicy() domainsandbox.RuntimePolicy {
	return domainsandbox.RuntimePolicy{
		TimeoutSeconds: 60,
		MemoryLimitMB:  512,
		CPULimit:       1,
		MaxOutputBytes: 4096,
		MaxConcurrency: 5,
		AllowNetwork:   false,
	}
}

func testProvider(id int64, version uint64) *domainsandbox.Provider {
	return &domainsandbox.Provider{
		ID:                    id,
		ProviderKey:           testProviderKey,
		Name:                  "Primary Sandbox",
		Type:                  domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret:        "ciphertext-endpoint-must-not-leak",
		EndpointHint:          "https://***.test",
		CredentialSecret:      "ciphertext-credential-must-not-leak",
		CredentialFingerprint: testFingerprint,
		Scopes:                []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy:                testRuntimePolicy(),
		Status:                domainsandbox.ProviderStatusDisabled,
		Health:                domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusUnknown},
		Version:               version,
		CreatedBy:             testActor().UserID,
		UpdatedBy:             testActor().UserID,
		CreatedAt:             time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		UpdatedAt:             time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
	}
}

func assertSanitizedJSON(t *testing.T, value any, prohibited ...string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(encoded)
	for _, secret := range prohibited {
		if secret != "" && strings.Contains(text, secret) {
			t.Fatalf("JSON leaked %q: %s", secret, text)
		}
	}
}

type testCredentialCodec struct {
	encryptCalls []testEncryptCall
	encryptErr   error
	inspectErr   error
	metadata     *infrasandbox.CredentialEnvelopeMetadata
}

type testEncryptCall struct {
	providerKey string
	field       infrasandbox.CredentialField
	plaintext   string
}

func (c *testCredentialCodec) Encrypt(providerKey string, field infrasandbox.CredentialField, plaintext []byte) (string, error) {
	c.encryptCalls = append(c.encryptCalls, testEncryptCall{providerKey: providerKey, field: field, plaintext: string(plaintext)})
	if c.encryptErr != nil {
		return "", c.encryptErr
	}
	return "encrypted:" + string(field), nil
}

func (c *testCredentialCodec) FingerprintCredential(_ []byte) (string, error) {
	if c.encryptErr != nil {
		return "", c.encryptErr
	}
	return testFingerprint, nil
}

func (c *testCredentialCodec) Inspect(envelope string) (infrasandbox.CredentialEnvelopeMetadata, error) {
	if c.inspectErr != nil {
		return infrasandbox.CredentialEnvelopeMetadata{}, c.inspectErr
	}
	if envelope == "" {
		return infrasandbox.CredentialEnvelopeMetadata{}, infrasandbox.ErrSandboxEnvelopeInvalid
	}
	if c.metadata != nil {
		return *c.metadata, nil
	}
	return infrasandbox.CredentialEnvelopeMetadata{Active: true}, nil
}

type testLeaseActivityChecker struct {
	active           bool
	err              error
	restoreErr       error
	retainErr        error
	activateErr      error
	compensateErr    error
	calls            int
	beginCalls       int
	restoreCalls     int
	retainCalls      int
	activateCalls    int
	compensateCalls  int
	draining         bool
	fence            DrainFence
	activeGeneration DrainFence
	sequence         int
	beforeActivate   func(*testLeaseActivityChecker)
	beforeCompensate func(*testLeaseActivityChecker, DrainFence)
}

func (c *testLeaseActivityChecker) BeginDrain(context.Context, string) (DrainHandle, error) {
	c.beginCalls++
	if c.err != nil {
		return DrainHandle{}, c.err
	}
	previous := DrainFence("")
	if c.draining {
		previous = c.fence
	}
	c.sequence++
	c.draining = true
	c.fence = DrainFence(fmt.Sprintf("test-drain-fence-%d", c.sequence))
	activeCount := 0
	if c.active {
		activeCount = 1
	}
	return DrainHandle{Current: c.fence, Previous: previous, ActiveCount: activeCount}, nil
}

func (c *testLeaseActivityChecker) RestoreActive(
	_ context.Context,
	_ string,
	current DrainFence,
) error {
	c.restoreCalls++
	if c.restoreErr != nil {
		return c.restoreErr
	}
	if !c.draining {
		if current != "" && c.activeGeneration == current {
			return nil
		}
		return domainsandbox.ErrExecutionForbidden
	}
	if current == "" || current != c.fence {
		return domainsandbox.ErrExecutionForbidden
	}
	c.draining = false
	c.fence = ""
	c.activeGeneration = current
	return nil
}

func (c *testLeaseActivityChecker) RetainDrain(
	_ context.Context,
	_ string,
	current DrainFence,
) error {
	c.retainCalls++
	if c.retainErr != nil {
		return c.retainErr
	}
	if !c.draining || current == "" || current != c.fence {
		return domainsandbox.ErrExecutionForbidden
	}
	return nil
}

func (c *testLeaseActivityChecker) Activate(_ context.Context, _ string, current DrainFence) error {
	c.activateCalls++
	if c.beforeActivate != nil {
		hook := c.beforeActivate
		c.beforeActivate = nil
		hook(c)
	}
	if c.activateErr != nil {
		return c.activateErr
	}
	if !c.draining || current == "" || current != c.fence {
		return domainsandbox.ErrExecutionForbidden
	}
	c.draining = false
	c.fence = ""
	c.activeGeneration = current
	return nil
}

func (c *testLeaseActivityChecker) CompensateActivation(
	_ context.Context,
	_ string,
	current DrainFence,
) (ActivationCompensationResult, error) {
	c.compensateCalls++
	if c.beforeCompensate != nil {
		hook := c.beforeCompensate
		c.beforeCompensate = nil
		hook(c, current)
	}
	if c.compensateErr != nil {
		return ActivationCompensationStale, c.compensateErr
	}
	if c.draining && c.fence == current && c.activeGeneration == "" {
		return ActivationCompensationAlreadyApplied, nil
	}
	if c.draining || current == "" || c.activeGeneration != current {
		return ActivationCompensationStale, nil
	}
	c.activeGeneration = ""
	c.draining = true
	c.fence = current
	return ActivationCompensationApplied, nil
}

func (c *testLeaseActivityChecker) HasActiveLeases(context.Context, string) (bool, error) {
	c.calls++
	return c.active, c.err
}

type testProviderRepository struct {
	providers      map[int64]*domainsandbox.Provider
	listOrder      []int64
	nextID         int64
	listCalls      int
	createErr      error
	updateErr      error
	statusErr      error
	healthErr      error
	deleteErr      error
	forUpdateCalls int
}

func (r *testProviderRepository) CreateProvider(_ context.Context, input domainsandbox.CreateProviderInput) (*domainsandbox.Provider, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	normalized, err := domainsandbox.NormalizeCreateProviderInput(input)
	if err != nil {
		return nil, err
	}
	for _, provider := range r.providers {
		if provider.ProviderKey == normalized.ProviderKey {
			return nil, domainsandbox.ErrProviderAlreadyExists
		}
	}
	r.nextID++
	provider := &domainsandbox.Provider{
		ID: r.nextID, ProviderKey: normalized.ProviderKey, Name: normalized.Name, Type: normalized.Type,
		EndpointSecret: normalized.EndpointSecret, EndpointHint: normalized.EndpointHint,
		CredentialSecret: normalized.CredentialSecret, CredentialFingerprint: normalized.CredentialFingerprint,
		Scopes: append([]domainsandbox.Scope(nil), normalized.Scopes...), Policy: normalized.Policy,
		Status: domainsandbox.ProviderStatusDisabled, Health: domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusUnknown},
		Version: 1, CreatedBy: normalized.ActorUserID, UpdatedBy: normalized.ActorUserID,
	}
	r.providers[provider.ID] = cloneTestProvider(provider)
	return cloneTestProvider(provider), nil
}

func (r *testProviderRepository) GetProvider(_ context.Context, providerID int64) (*domainsandbox.Provider, error) {
	provider := r.providers[providerID]
	if provider == nil || provider.DeletedAt != nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	return cloneTestProvider(provider), nil
}

func (r *testProviderRepository) GetProviderForUpdate(ctx context.Context, providerID int64) (*domainsandbox.Provider, error) {
	r.forUpdateCalls++
	return r.GetProvider(ctx, providerID)
}

func (r *testProviderRepository) GetProviderByKey(_ context.Context, providerKey string) (*domainsandbox.Provider, error) {
	for _, provider := range r.providers {
		if provider.ProviderKey == providerKey && provider.DeletedAt == nil {
			return cloneTestProvider(provider), nil
		}
	}
	return nil, domainsandbox.ErrProviderNotFound
}

func (r *testProviderRepository) ListProviders(_ context.Context, request domainsandbox.ProviderListRequest) ([]*domainsandbox.Provider, int64, error) {
	r.listCalls++
	normalized, err := domainsandbox.NormalizeProviderListRequest(request)
	if err != nil {
		return nil, 0, err
	}
	ids := append([]int64(nil), r.listOrder...)
	if len(ids) == 0 {
		for id := range r.providers {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	}
	filtered := make([]*domainsandbox.Provider, 0, len(ids))
	for _, id := range ids {
		provider := r.providers[id]
		if provider == nil || provider.DeletedAt != nil || normalized.Type != "" && provider.Type != normalized.Type ||
			normalized.Status != "" && provider.Status != normalized.Status ||
			normalized.Scope != "" && !testContainsScope(provider.Scopes, normalized.Scope) ||
			len(normalized.AuthorizedScopes) > 0 && !testProviderScopesAreSubset(provider.Scopes, normalized.AuthorizedScopes) ||
			normalized.Keyword != "" && !strings.Contains(strings.ToLower(provider.Name), strings.ToLower(normalized.Keyword)) {
			continue
		}
		filtered = append(filtered, cloneTestProvider(provider))
	}
	total := int64(len(filtered))
	start := normalized.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + normalized.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (r *testProviderRepository) UpdateProvider(_ context.Context, input domainsandbox.UpdateProviderInput) (*domainsandbox.Provider, error) {
	if r.updateErr != nil {
		return nil, r.updateErr
	}
	provider := r.providers[input.ProviderID]
	if provider == nil || provider.DeletedAt != nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	if provider.Version != input.ExpectedVersion {
		return nil, domainsandbox.ErrVersionConflict
	}
	normalized, err := domainsandbox.NormalizeUpdateProviderInput(input)
	if err != nil {
		return nil, err
	}
	provider.Name = normalized.Name
	provider.Type = normalized.Type
	provider.EndpointSecret = normalized.EndpointSecret
	provider.EndpointHint = normalized.EndpointHint
	provider.CredentialSecret = normalized.CredentialSecret
	provider.CredentialFingerprint = normalized.CredentialFingerprint
	provider.Scopes = append([]domainsandbox.Scope(nil), normalized.Scopes...)
	provider.Policy = normalized.Policy
	if normalized.ResetHealth {
		provider.Health = domainsandbox.HealthSnapshot{Status: domainsandbox.HealthStatusUnknown}
	}
	provider.UpdatedBy = normalized.ActorUserID
	provider.Version++
	return cloneTestProvider(provider), nil
}

func (r *testProviderRepository) UpdateProviderStatus(_ context.Context, input domainsandbox.UpdateProviderStatusInput) (uint64, error) {
	if r.statusErr != nil {
		return 0, r.statusErr
	}
	provider := r.providers[input.ProviderID]
	if provider == nil || provider.DeletedAt != nil {
		return 0, domainsandbox.ErrProviderNotFound
	}
	if provider.Version != input.ExpectedVersion {
		return 0, domainsandbox.ErrVersionConflict
	}
	normalized, err := domainsandbox.NormalizeUpdateProviderStatusInput(input)
	if err != nil {
		return 0, err
	}
	provider.Status = normalized.Status
	provider.UpdatedBy = normalized.ActorUserID
	provider.Version++
	return provider.Version, nil
}

func (r *testProviderRepository) UpdateProviderHealth(_ context.Context, input domainsandbox.UpdateProviderHealthInput) (uint64, error) {
	if r.healthErr != nil {
		return 0, r.healthErr
	}
	provider := r.providers[input.ProviderID]
	if provider == nil || provider.DeletedAt != nil {
		return 0, domainsandbox.ErrProviderNotFound
	}
	if provider.Version != input.ExpectedVersion {
		return 0, domainsandbox.ErrVersionConflict
	}
	normalized, err := domainsandbox.NormalizeUpdateProviderHealthInput(input, provider.Scopes)
	if err != nil {
		return 0, err
	}
	provider.Health = normalized.Health
	provider.UpdatedBy = normalized.ActorUserID
	provider.Version++
	return provider.Version, nil
}

func (r *testProviderRepository) DeleteProvider(_ context.Context, input domainsandbox.DeleteProviderInput) (uint64, error) {
	if r.deleteErr != nil {
		return 0, r.deleteErr
	}
	if err := domainsandbox.ValidateDeleteProviderInput(input); err != nil {
		return 0, err
	}
	provider := r.providers[input.ProviderID]
	if provider == nil || provider.DeletedAt != nil {
		return 0, domainsandbox.ErrProviderNotFound
	}
	if provider.Version != input.ExpectedVersion {
		return 0, domainsandbox.ErrVersionConflict
	}
	provider.Version++
	now := time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC)
	provider.DeletedAt = &now
	return provider.Version, nil
}

type testDefaultRepository struct {
	defaults map[domainsandbox.Scope]*domainsandbox.ProviderDefault
}

func (r *testDefaultRepository) GetProviderDefault(_ context.Context, scope domainsandbox.Scope) (*domainsandbox.ProviderDefault, error) {
	value := r.defaults[scope]
	if value == nil {
		return nil, domainsandbox.ErrDefaultMissing
	}
	clone := *value
	return &clone, nil
}

func (r *testDefaultRepository) ListProviderDefaults(context.Context) ([]*domainsandbox.ProviderDefault, error) {
	result := make([]*domainsandbox.ProviderDefault, 0, len(r.defaults))
	for _, scope := range []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev, domainsandbox.ScopeMCPStdio} {
		if value := r.defaults[scope]; value != nil {
			clone := *value
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (r *testDefaultRepository) SetProviderDefault(_ context.Context, input domainsandbox.SetProviderDefaultInput) (*domainsandbox.ProviderDefault, error) {
	current := r.defaults[input.Scope]
	if current == nil {
		if input.ExpectedVersion != 0 {
			return nil, domainsandbox.ErrDefaultMissing
		}
		current = &domainsandbox.ProviderDefault{Scope: input.Scope, Version: 1}
	} else {
		if input.ExpectedVersion == 0 || current.Version != input.ExpectedVersion {
			return nil, domainsandbox.ErrVersionConflict
		}
		current.Version++
	}
	current.ProviderID = input.ProviderID
	current.UpdatedBy = input.ActorUserID
	r.defaults[input.Scope] = current
	clone := *current
	return &clone, nil
}

type testAuditRepository struct {
	events    []*domainsandbox.ProviderAuditEvent
	appendErr error
	listCalls int
}

func (r *testAuditRepository) AppendProviderAuditEvent(_ context.Context, input domainsandbox.AppendProviderAuditEventInput) (*domainsandbox.ProviderAuditEvent, error) {
	if r.appendErr != nil {
		return nil, r.appendErr
	}
	normalized, err := domainsandbox.NormalizeAppendProviderAuditEventInput(input)
	if err != nil {
		return nil, err
	}
	event, err := domainsandbox.NewProviderAuditEvent(normalized)
	if err != nil {
		return nil, err
	}
	event.ID = int64(len(r.events) + 1)
	event.CreatedAt = time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	r.events = append(r.events, cloneTestAudit(event))
	return cloneTestAudit(event), nil
}

func (r *testAuditRepository) ListProviderAuditEvents(_ context.Context, request domainsandbox.ProviderAuditListRequest) ([]*domainsandbox.ProviderAuditEvent, int64, error) {
	r.listCalls++
	normalized, err := domainsandbox.NormalizeProviderAuditListRequest(request)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]*domainsandbox.ProviderAuditEvent, 0, len(r.events))
	for index := len(r.events) - 1; index >= 0; index-- {
		event := r.events[index]
		if normalized.ProviderID != 0 && event.ProviderID != normalized.ProviderID ||
			normalized.Action != "" && event.Action != normalized.Action || normalized.Result != "" && event.Result != normalized.Result {
			continue
		}
		filtered = append(filtered, cloneTestAudit(event))
	}
	total := int64(len(filtered))
	start := normalized.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + normalized.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

type testUnitOfWork struct {
	providers           *testProviderRepository
	defaults            *testDefaultRepository
	audits              *testAuditRepository
	calls               int
	providerCreateCalls int
	committed           bool
}

func (u *testUnitOfWork) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	u.providerCreateCalls++
	return u.WithinTransaction(ctx, callback)
}

func (u *testUnitOfWork) WithinTransaction(ctx context.Context, callback func(context.Context, domainsandbox.TransactionRepositories) error) error {
	u.calls++
	u.committed = false
	providersSnapshot := cloneTestProviderMap(u.providers.providers)
	defaultsSnapshot := cloneTestDefaultMap(u.defaults.defaults)
	auditsSnapshot := cloneTestAudits(u.audits.events)
	err := callback(ctx, domainsandbox.TransactionRepositories{Providers: u.providers, Defaults: u.defaults, Audits: u.audits})
	if err != nil {
		u.providers.providers = providersSnapshot
		u.defaults.defaults = defaultsSnapshot
		u.audits.events = auditsSnapshot
		return err
	}
	u.committed = true
	return nil
}

type testHealthProviderFactory struct {
	provider HealthProvider
	err      error
	calls    int
	input    *domainsandbox.Provider
}

func (f *testHealthProviderFactory) Build(_ context.Context, provider *domainsandbox.Provider) (HealthProvider, error) {
	f.calls++
	f.input = cloneTestProvider(provider)
	return f.provider, f.err
}

type testHealthProvider struct {
	result     infrasandbox.HealthResult
	err        error
	closeErr   error
	calls      int
	closeCalls int
}

func (p *testHealthProvider) Health(context.Context) (infrasandbox.HealthResult, error) {
	p.calls++
	return p.result, p.err
}

func (p *testHealthProvider) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.closeCalls++
	return p.closeErr
}

func cloneTestProvider(input *domainsandbox.Provider) *domainsandbox.Provider {
	if input == nil {
		return nil
	}
	clone := *input
	clone.Scopes = append([]domainsandbox.Scope(nil), input.Scopes...)
	clone.Policy.NetworkAllowlist = append([]string(nil), input.Policy.NetworkAllowlist...)
	clone.Health.Capabilities = append([]domainsandbox.Scope(nil), input.Health.Capabilities...)
	if input.DeletedAt != nil {
		deletedAt := *input.DeletedAt
		clone.DeletedAt = &deletedAt
	}
	return &clone
}

func cloneTestAudit(input *domainsandbox.ProviderAuditEvent) *domainsandbox.ProviderAuditEvent {
	if input == nil {
		return nil
	}
	clone := *input
	clone.Metadata = make(map[string]string, len(input.Metadata))
	for key, value := range input.Metadata {
		clone.Metadata[key] = value
	}
	return &clone
}

func cloneTestProviderMap(input map[int64]*domainsandbox.Provider) map[int64]*domainsandbox.Provider {
	result := make(map[int64]*domainsandbox.Provider, len(input))
	for id, provider := range input {
		result[id] = cloneTestProvider(provider)
	}
	return result
}

func cloneTestDefaultMap(input map[domainsandbox.Scope]*domainsandbox.ProviderDefault) map[domainsandbox.Scope]*domainsandbox.ProviderDefault {
	result := make(map[domainsandbox.Scope]*domainsandbox.ProviderDefault, len(input))
	for scope, value := range input {
		clone := *value
		result[scope] = &clone
	}
	return result
}

func cloneTestAudits(input []*domainsandbox.ProviderAuditEvent) []*domainsandbox.ProviderAuditEvent {
	result := make([]*domainsandbox.ProviderAuditEvent, len(input))
	for index, event := range input {
		result[index] = cloneTestAudit(event)
	}
	return result
}

func testContainsScope(scopes []domainsandbox.Scope, target domainsandbox.Scope) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func testProviderScopesAreSubset(providerScopes, allowedScopes []domainsandbox.Scope) bool {
	if len(providerScopes) == 0 {
		return false
	}
	for _, providerScope := range providerScopes {
		if !testContainsScope(allowedScopes, providerScope) {
			return false
		}
	}
	return true
}

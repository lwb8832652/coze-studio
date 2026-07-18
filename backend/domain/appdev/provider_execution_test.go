// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestProviderExecutionObservedStateIsMonotonic(t *testing.T) {
	allowed := []struct {
		from ProviderExecutionObservedState
		to   ProviderExecutionObservedState
	}{
		{ProviderExecutionObservedPending, ProviderExecutionObservedRunning},
		{ProviderExecutionObservedRunning, ProviderExecutionObservedFailed},
		{ProviderExecutionObservedFailed, ProviderExecutionObservedCleanupPending},
		{ProviderExecutionObservedCleanupPending, ProviderExecutionObservedCleanupComplete},
	}
	for _, transition := range allowed {
		if !CanAdvanceProviderExecutionObserved(transition.from, transition.to) {
			t.Fatalf("transition %s -> %s was rejected", transition.from, transition.to)
		}
	}
	for _, transition := range []struct {
		from ProviderExecutionObservedState
		to   ProviderExecutionObservedState
	}{
		{ProviderExecutionObservedFailed, ProviderExecutionObservedRunning},
		{ProviderExecutionObservedSucceeded, ProviderExecutionObservedPending},
		{ProviderExecutionObservedCleanupComplete, ProviderExecutionObservedRunning},
	} {
		if CanAdvanceProviderExecutionObserved(transition.from, transition.to) {
			t.Fatalf("terminal transition %s -> %s was accepted", transition.from, transition.to)
		}
	}
}

func TestProviderExecutionOwnerTokenIsOpaqueAndRedacted(t *testing.T) {
	secret := bytes.Repeat([]byte{0x5a}, ProviderExecutionOwnerTokenBytes)
	token, err := NewProviderExecutionOwnerToken(bytes.NewReader(secret))
	if err != nil || token.IsZero() || token.OwnerIdentityHash().IsZero() {
		t.Fatalf("NewProviderExecutionOwnerToken() = %#v, %v", token, err)
	}
	for _, formatted := range []string{
		fmt.Sprintf("%v", token), fmt.Sprintf("%+v", token), fmt.Sprintf("%#v", token), token.String(), token.GoString(),
	} {
		if strings.Contains(formatted, "5a") || !strings.Contains(formatted, "REDACTED") {
			t.Fatalf("owner token formatting leaked or was not redacted: %q", formatted)
		}
	}
	if encoded, err := json.Marshal(token); err == nil || len(encoded) != 0 {
		t.Fatalf("owner token JSON = %q, %v", encoded, err)
	}
}

func TestProviderExecutionCleanupOperationHashBindsExecutionIdentity(t *testing.T) {
	token, err := NewProviderExecutionOwnerToken(bytes.NewReader(bytes.Repeat([]byte{0x4c}, ProviderExecutionOwnerTokenBytes)))
	if err != nil {
		t.Fatal(err)
	}
	baseCAS := ProviderExecutionOwnerCAS{
		SpaceID: "1001", ProjectID: "project-a", Generation: 7, ExpectedVersion: 11,
		ProviderKey: "provider-a", ProviderScope: "appdev", OwnerHash: token.OwnerIdentityHash(), OwnerEpoch: 3,
	}
	const operationID = "cleanup-operation-stable"
	baseHash, err := HashProviderExecutionCleanupOperationID(operationID, baseCAS)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*ProviderExecutionOwnerCAS)
	}{
		{name: "space", mutate: func(cas *ProviderExecutionOwnerCAS) { cas.SpaceID = "1002" }},
		{name: "project", mutate: func(cas *ProviderExecutionOwnerCAS) { cas.ProjectID = "project-b" }},
		{name: "generation", mutate: func(cas *ProviderExecutionOwnerCAS) { cas.Generation++ }},
		{name: "provider key", mutate: func(cas *ProviderExecutionOwnerCAS) { cas.ProviderKey = "provider-b" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := baseCAS
			test.mutate(&candidate)
			if candidate.OwnerHash != baseCAS.OwnerHash || candidate.OwnerEpoch != baseCAS.OwnerEpoch {
				t.Fatal("test mutation changed owner capability")
			}
			candidateHash, err := HashProviderExecutionCleanupOperationID(operationID, candidate)
			if err != nil {
				t.Fatal(err)
			}
			if candidateHash.Equal(baseHash) {
				t.Fatalf("cleanup operation hash did not bind %s", test.name)
			}
		})
	}
}

func TestNormalizeProviderExecutionPreviewRoute(t *testing.T) {
	for _, valid := range []string{"", "/preview/app", "/preview/app/index.html"} {
		if got, err := NormalizeProviderExecutionPreviewRoute(valid); err != nil || got != valid {
			t.Fatalf("NormalizeProviderExecutionPreviewRoute(%q) = %q, %v", valid, got, err)
		}
	}
	for _, invalid := range []string{"https://example.com/preview", "//example.com/x", "/a/../secret", "/x?token=secret", "relative/path"} {
		if _, err := NormalizeProviderExecutionPreviewRoute(invalid); err == nil {
			t.Fatalf("unsafe preview route %q was accepted", invalid)
		}
	}
}

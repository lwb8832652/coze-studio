// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSessionFeaturesAreIndependentOptionalExtensions(t *testing.T) {
	tests := []struct {
		name     string
		features []ProviderFeature
	}{
		{name: "legacy empty"},
		{name: "legacy features", features: []ProviderFeature{
			ProviderFeatureQueueStatusV1,
			ProviderFeatureSignedExecutionContext,
		}},
		{name: "session only", features: []ProviderFeature{ProviderFeatureSandboxSessionV1}},
		{name: "session identity only", features: []ProviderFeature{ProviderFeatureSignedSessionContextV2}},
		{name: "both session extensions", features: []ProviderFeature{
			ProviderFeatureSandboxSessionV1,
			ProviderFeatureSignedSessionContextV2,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeProviderFeatures(tt.features)
			if err != nil {
				t.Fatalf("NormalizeProviderFeatures() error = %v", err)
			}
			if len(got) != len(tt.features) {
				t.Fatalf("NormalizeProviderFeatures() length = %d, want %d", len(got), len(tt.features))
			}
		})
	}

	if ProviderFeatureSandboxSessionV1 == ProviderFeatureSignedSessionContextV2 ||
		ProviderFeatureSandboxSessionV1 == ProviderFeatureSignedExecutionContext ||
		ProviderFeatureSignedSessionContextV2 == ProviderFeatureSignedExecutionContext {
		t.Fatal("session, session identity v2, and execution identity v1 features must remain independent")
	}
	for _, features := range [][]ProviderFeature{
		{ProviderFeatureSandboxSessionV1, ProviderFeatureSandboxSessionV1},
		{ProviderFeatureSignedSessionContextV2, ProviderFeatureSignedSessionContextV2},
		{"sandbox_session_v2"},
	} {
		if _, err := NormalizeProviderFeatures(features); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeProviderFeatures(%q) error = %v, want ErrInvalidInput", features, err)
		}
	}
}

func TestSessionProfileValidationSeparatesValueSupportFromPhaseOneAdmission(t *testing.T) {
	for _, profile := range []SessionProfile{SessionProfileCore, SessionProfileInteractive} {
		got, err := NormalizeSessionProfile(profile)
		if err != nil || got != profile {
			t.Fatalf("NormalizeSessionProfile(%q) = %q, %v", profile, got, err)
		}
	}
	for _, profile := range []SessionProfile{"", "CORE", "browser", " core"} {
		if _, err := NormalizeSessionProfile(profile); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeSessionProfile(%q) error = %v, want ErrInvalidInput", profile, err)
		}
	}
	if err := ValidatePhase1SessionProfile(SessionProfileCore); err != nil {
		t.Fatalf("ValidatePhase1SessionProfile(core) error = %v", err)
	}
	if err := ValidatePhase1SessionProfile(SessionProfileInteractive); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ValidatePhase1SessionProfile(interactive) error = %v, want ErrInvalidInput", err)
	}
}

func TestSessionStableKeyRequiresCompleteCanonicalIdentity(t *testing.T) {
	valid := SessionKey{
		DeploymentID: "runner-dev-a",
		ProviderID:   41,
		SpaceID:      42,
		UserID:       43,
		ThreadID:     "thread_01HZX7Y2P0",
		Profile:      SessionProfileCore,
	}
	got, err := NormalizeSessionKey(valid)
	if err != nil || got != valid {
		t.Fatalf("NormalizeSessionKey() = %#v, %v", got, err)
	}

	tests := []struct {
		name   string
		mutate func(*SessionKey)
	}{
		{name: "deployment missing", mutate: func(key *SessionKey) { key.DeploymentID = "" }},
		{name: "deployment noncanonical", mutate: func(key *SessionKey) { key.DeploymentID = "runner/dev" }},
		{name: "provider missing", mutate: func(key *SessionKey) { key.ProviderID = 0 }},
		{name: "space missing", mutate: func(key *SessionKey) { key.SpaceID = 0 }},
		{name: "user missing", mutate: func(key *SessionKey) { key.UserID = 0 }},
		{name: "thread missing", mutate: func(key *SessionKey) { key.ThreadID = "" }},
		{name: "thread traversal", mutate: func(key *SessionKey) { key.ThreadID = "../thread" }},
		{name: "thread slash", mutate: func(key *SessionKey) { key.ThreadID = "thread/child" }},
		{name: "thread whitespace", mutate: func(key *SessionKey) { key.ThreadID = " thread" }},
		{name: "thread too long", mutate: func(key *SessionKey) { key.ThreadID = strings.Repeat("a", MaxSessionIdentifierBytes+1) }},
		{name: "profile missing", mutate: func(key *SessionKey) { key.Profile = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.mutate(&input)
			if _, err := NormalizeSessionKey(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeSessionKey() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSessionLifecycleTransitionsAreIdempotent(t *testing.T) {
	assertTransition := func(from SessionState, action SessionAction, want SessionState, wantChanged bool) SessionState {
		t.Helper()
		got, changed, err := TransitionSessionState(from, action)
		if err != nil || got != want || changed != wantChanged {
			t.Fatalf("TransitionSessionState(%q, %q) = %q, %t, %v; want %q, %t", from, action, got, changed, err, want, wantChanged)
		}
		return got
	}

	state := assertTransition("", SessionActionAcquire, SessionStateActive, true)
	state = assertTransition(state, SessionActionGet, SessionStateActive, false)
	state = assertTransition(state, SessionActionAcquire, SessionStateActive, false)
	state = assertTransition(state, SessionActionRelease, SessionStateReleased, true)
	state = assertTransition(state, SessionActionGet, SessionStateReleased, false)
	state = assertTransition(state, SessionActionRelease, SessionStateReleased, false)
	state = assertTransition(state, SessionActionAcquire, SessionStateActive, true)
	state = assertTransition(state, SessionActionMarkRecovering, SessionStateRecovering, true)
	state = assertTransition(state, SessionActionGet, SessionStateRecovering, false)
	state = assertTransition(state, SessionActionMarkRecovering, SessionStateRecovering, false)
	state = assertTransition(state, SessionActionRecover, SessionStateActive, true)
	state = assertTransition(state, SessionActionRecover, SessionStateActive, false)
	state = assertTransition(state, SessionActionMarkRecovering, SessionStateRecovering, true)
	state = assertTransition(state, SessionActionDestroy, SessionStateDestroyed, true)
	state = assertTransition(state, SessionActionGet, SessionStateDestroyed, false)
	_ = assertTransition(state, SessionActionDestroy, SessionStateDestroyed, false)

	for _, action := range []SessionAction{SessionActionAcquire, SessionActionRelease, SessionActionRecover} {
		if _, _, err := TransitionSessionState(SessionStateDestroyed, action); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("TransitionSessionState(destroyed, %q) error = %v, want ErrInvalidInput", action, err)
		}
	}
	for _, input := range []struct {
		state  SessionState
		action SessionAction
	}{{"", SessionActionGet}, {SessionState("unknown"), SessionActionGet}, {SessionStateActive, SessionAction("archive")}} {
		if _, _, err := TransitionSessionState(input.state, input.action); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("TransitionSessionState(%q, %q) error = %v, want ErrInvalidInput", input.state, input.action, err)
		}
	}
}

func TestSessionPublicFormattingRedactsStableIdentity(t *testing.T) {
	key := SessionKey{
		DeploymentID: "deployment-secret",
		ProviderID:   41,
		SpaceID:      42,
		UserID:       43,
		ThreadID:     "thread-secret",
		Profile:      SessionProfileCore,
	}
	ref := SessionRef{SessionID: "01J5D3N8A0BCDEFGHJKMNPQRST", Key: key, RuntimeGeneration: 7}
	session := RuntimeSession{Ref: ref, State: SessionStateActive, UpstreamShellID: "upstream-secret"}

	for name, value := range map[string]any{"key": key, "ref": ref, "session": session} {
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%#v", value), fmt.Sprintf("%+v", value)} {
			for _, secret := range []string{"deployment-secret", "thread-secret", "01J5D3N8A0BCDEFGHJKMNPQRST", "upstream-secret", "41", "42", "43"} {
				if strings.Contains(rendered, secret) {
					t.Fatalf("%s formatting leaked %q in %q", name, secret, rendered)
				}
			}
		}
	}
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
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
	generation := AIOGenerationState{DeploymentID: "deployment-secret", Generation: 7, SentinelID: "newx-generation-0123456789abcdef0123456789abcdef"}
	casInput := CompareAndReplaceAIOSentinelInput{
		DeploymentID:        "deployment-secret",
		ExpectedSentinelID:  "newx-generation-0123456789abcdef0123456789abcdef",
		CandidateSentinelID: "newx-generation-fedcba9876543210fedcba9876543210",
	}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	acquireInput := AcquireRuntimeSessionInput{
		Key: key, CandidateSessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942",
		RuntimeGeneration: 7, ExpiresAt: now.Add(time.Minute), Now: now,
	}
	bindInput := BindRuntimeSessionInput{
		Ref: ref, ExpectedVersion: 1, UpstreamShellID: "upstream-secret",
		ExpiresAt: now.Add(time.Minute), Now: now,
	}
	transitionInput := TransitionRuntimeSessionInput{
		Ref: ref, ExpectedVersion: 1, Action: SessionActionRecover, NextRuntimeGeneration: 8,
		UpstreamShellID: "upstream-secret", ExpiresAt: now.Add(time.Minute), Now: now,
	}

	for name, value := range map[string]any{
		"key": key, "ref": ref, "session": session, "generation": generation, "cas_input": casInput,
		"acquire_input": acquireInput, "bind_input": bindInput, "transition_input": transitionInput,
	} {
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%#v", value), fmt.Sprintf("%+v", value)} {
			for _, secret := range []string{
				"deployment-secret", "thread-secret", "01J5D3N8A0BCDEFGHJKMNPQRST", "upstream-secret",
				"newx-generation-0123456789abcdef0123456789abcdef",
				"newx-generation-fedcba9876543210fedcba9876543210", "41", "42", "43",
			} {
				if strings.Contains(rendered, secret) {
					t.Fatalf("%s formatting leaked %q in %q", name, secret, rendered)
				}
			}
		}
	}
}

func TestRuntimeSessionPersistenceContractHasNoIdentityAllocation(t *testing.T) {
	if _, exists := reflect.TypeOf(RuntimeSession{}).FieldByName("IdentityID"); exists {
		t.Fatal("RuntimeSession must derive workspace from SessionRef instead of persisting IdentityID")
	}

	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	key := SessionKey{DeploymentID: "runner-dev-a", ProviderID: 41, SpaceID: 42, UserID: 43, ThreadID: "thread_01HZX7Y2P0", Profile: SessionProfileCore}
	acquire, err := NormalizeAcquireRuntimeSessionInput(AcquireRuntimeSessionInput{
		Key: key, CandidateSessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942", RuntimeGeneration: 7,
		ExpiresAt: now.Add(20 * time.Minute), Now: now,
	})
	if err != nil || acquire.Key != key || acquire.Now.Location() != time.UTC {
		t.Fatalf("NormalizeAcquireRuntimeSessionInput() = %#v, %v", acquire, err)
	}

	transition, err := NormalizeTransitionRuntimeSessionInput(TransitionRuntimeSessionInput{
		Ref:             SessionRef{SessionID: acquire.CandidateSessionID, Key: key, RuntimeGeneration: 7},
		ExpectedVersion: 1, Action: SessionActionRelease, Now: now.Add(time.Minute),
	})
	if err != nil || transition.ExpectedVersion != 1 {
		t.Fatalf("NormalizeTransitionRuntimeSessionInput() = %#v, %v", transition, err)
	}

	for name, input := range map[string]AcquireRuntimeSessionInput{
		"bad UUID":           {Key: key, CandidateSessionID: "session-1", RuntimeGeneration: 7, ExpiresAt: now.Add(time.Minute), Now: now},
		"nil UUID":           {Key: key, CandidateSessionID: "00000000-0000-0000-0000-000000000000", RuntimeGeneration: 7, ExpiresAt: now.Add(time.Minute), Now: now},
		"zero generation":    {Key: key, CandidateSessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942", ExpiresAt: now.Add(time.Minute), Now: now},
		"expired":            {Key: key, CandidateSessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942", RuntimeGeneration: 7, ExpiresAt: now, Now: now},
		"interactive phase1": {Key: func() SessionKey { invalid := key; invalid.Profile = SessionProfileInteractive; return invalid }(), CandidateSessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942", RuntimeGeneration: 7, ExpiresAt: now.Add(time.Minute), Now: now},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeAcquireRuntimeSessionInput(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeAcquireRuntimeSessionInput() error = %v, want ErrInvalidInput", err)
			}
		})
	}
	ref := SessionRef{SessionID: acquire.CandidateSessionID, Key: key, RuntimeGeneration: 7}
	for name, shellID := range map[string]string{
		"exact sentinel":     "newx-generation-0123456789abcdef0123456789abcdef",
		"reserved namespace": "newx-generation-business-shell",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeBindRuntimeSessionInput(BindRuntimeSessionInput{
				Ref: ref, ExpectedVersion: 1, UpstreamShellID: shellID,
				ExpiresAt: now.Add(time.Minute), Now: now,
			}); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeBindRuntimeSessionInput() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestAIOGenerationCASInputIsCanonicalAndOpaque(t *testing.T) {
	valid := CompareAndReplaceAIOSentinelInput{
		DeploymentID:        "runner-dev-a",
		ExpectedSentinelID:  "newx-generation-0123456789abcdef0123456789abcdef",
		CandidateSentinelID: "newx-generation-fedcba9876543210fedcba9876543210",
	}
	if got, err := NormalizeCompareAndReplaceAIOSentinelInput(valid); err != nil || got != valid {
		t.Fatalf("NormalizeCompareAndReplaceAIOSentinelInput() = %#v, %v", got, err)
	}
	initial := valid
	initial.ExpectedSentinelID = ""
	if _, err := NormalizeCompareAndReplaceAIOSentinelInput(initial); err != nil {
		t.Fatalf("initial sentinel CAS error = %v", err)
	}
	for name, mutate := range map[string]func(*CompareAndReplaceAIOSentinelInput){
		"missing deployment": func(input *CompareAndReplaceAIOSentinelInput) { input.DeploymentID = "" },
		"missing candidate":  func(input *CompareAndReplaceAIOSentinelInput) { input.CandidateSentinelID = "" },
		"same sentinel":      func(input *CompareAndReplaceAIOSentinelInput) { input.CandidateSentinelID = input.ExpectedSentinelID },
		"tenant fact":        func(input *CompareAndReplaceAIOSentinelInput) { input.CandidateSentinelID = "space-42/thread-9" },
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := NormalizeCompareAndReplaceAIOSentinelInput(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeCompareAndReplaceAIOSentinelInput() error = %v, want ErrInvalidInput", err)
			}
		})
	}
	maxed := AIOGenerationState{
		DeploymentID: "runner-dev-a", Generation: ^uint64(0),
		SentinelID: "newx-generation-0123456789abcdef0123456789abcdef",
	}
	if _, err := NormalizeAIOGenerationState(maxed); !errors.Is(err, ErrConfigurationInvalid) {
		t.Fatalf("NormalizeAIOGenerationState(max generation) error = %v, want ErrConfigurationInvalid", err)
	}
}

func TestAIOGenerationStateRejectsPartialOrMalformedPersistentState(t *testing.T) {
	valid := AIOGenerationState{
		DeploymentID: "runner-dev-a", Generation: 7,
		SentinelID: "newx-generation-0123456789abcdef0123456789abcdef",
	}
	for name, input := range map[string]AIOGenerationState{
		"initial": {},
		"active":  valid,
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := NormalizeAIOGenerationState(input); err != nil || got != input {
				t.Fatalf("NormalizeAIOGenerationState() = %#v, %v", got, err)
			}
		})
	}
	for name, mutate := range map[string]func(*AIOGenerationState){
		"missing deployment": func(state *AIOGenerationState) { state.DeploymentID = "" },
		"zero generation":    func(state *AIOGenerationState) { state.Generation = 0 },
		"missing sentinel":   func(state *AIOGenerationState) { state.SentinelID = "" },
		"malformed sentinel": func(state *AIOGenerationState) { state.SentinelID = "newx-generation-UPPER" },
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := NormalizeAIOGenerationState(input); !errors.Is(err, ErrConfigurationInvalid) {
				t.Fatalf("NormalizeAIOGenerationState() error = %v, want ErrConfigurationInvalid", err)
			}
		})
	}
}

func TestSessionNotFoundErrorCodeIsStable(t *testing.T) {
	if got := ErrorCodeOf(fmt.Errorf("wrapped: %w", ErrSessionNotFound)); got != ErrCodeSessionNotFound {
		t.Fatalf("ErrorCodeOf(ErrSessionNotFound) = %q, want %q", got, ErrCodeSessionNotFound)
	}
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxSessionIdentifierBytes = 128

type SessionProfile string

const (
	SessionProfileCore        SessionProfile = "core"
	SessionProfileInteractive SessionProfile = "interactive"
)

func NormalizeSessionProfile(profile SessionProfile) (SessionProfile, error) {
	switch profile {
	case SessionProfileCore, SessionProfileInteractive:
		return profile, nil
	default:
		return "", ErrInvalidInput
	}
}

func ValidatePhase1SessionProfile(profile SessionProfile) error {
	if profile != SessionProfileCore {
		return ErrInvalidInput
	}
	return nil
}

type SessionKey struct {
	DeploymentID string
	ProviderID   int64
	SpaceID      int64
	UserID       int64
	ThreadID     string
	Profile      SessionProfile
}

func NormalizeSessionKey(key SessionKey) (SessionKey, error) {
	if !validSessionIdentifier(key.DeploymentID) || key.ProviderID <= 0 || key.SpaceID <= 0 ||
		key.UserID <= 0 || !validSessionIdentifier(key.ThreadID) {
		return SessionKey{}, ErrInvalidInput
	}
	profile, err := NormalizeSessionProfile(key.Profile)
	if err != nil {
		return SessionKey{}, ErrInvalidInput
	}
	key.Profile = profile
	return key, nil
}

func (SessionKey) String() string   { return "sandbox.SessionKey{identity:<redacted>}" }
func (SessionKey) GoString() string { return "sandbox.SessionKey{identity:<redacted>}" }
func (SessionKey) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.SessionKey{identity:<redacted>}")
}

type SessionRef struct {
	SessionID         string
	Key               SessionKey
	RuntimeGeneration uint64
}

func NormalizeSessionRef(ref SessionRef) (SessionRef, error) {
	key, err := NormalizeSessionKey(ref.Key)
	if err != nil || !validSessionIdentifier(ref.SessionID) || ref.RuntimeGeneration == 0 {
		return SessionRef{}, ErrInvalidInput
	}
	ref.Key = key
	return ref, nil
}

func (SessionRef) String() string   { return "sandbox.SessionRef{identity:<redacted>}" }
func (SessionRef) GoString() string { return "sandbox.SessionRef{identity:<redacted>}" }
func (SessionRef) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.SessionRef{identity:<redacted>}")
}

type SessionState string

const (
	SessionStateActive     SessionState = "active"
	SessionStateRecovering SessionState = "recovering"
	SessionStateReleased   SessionState = "released"
	SessionStateDestroyed  SessionState = "destroyed"
)

type SessionAction string

const (
	SessionActionAcquire        SessionAction = "acquire"
	SessionActionGet            SessionAction = "get"
	SessionActionRelease        SessionAction = "release"
	SessionActionDestroy        SessionAction = "destroy"
	SessionActionMarkRecovering SessionAction = "mark_recovering"
	SessionActionRecover        SessionAction = "recover"
)

func TransitionSessionState(state SessionState, action SessionAction) (SessionState, bool, error) {
	switch action {
	case SessionActionAcquire:
		switch state {
		case "", SessionStateReleased:
			return SessionStateActive, true, nil
		case SessionStateActive:
			return state, false, nil
		}
	case SessionActionGet:
		switch state {
		case SessionStateActive, SessionStateRecovering, SessionStateReleased, SessionStateDestroyed:
			return state, false, nil
		}
	case SessionActionRelease:
		switch state {
		case SessionStateActive:
			return SessionStateReleased, true, nil
		case SessionStateReleased:
			return state, false, nil
		}
	case SessionActionDestroy:
		switch state {
		case SessionStateActive, SessionStateRecovering, SessionStateReleased:
			return SessionStateDestroyed, true, nil
		case SessionStateDestroyed:
			return state, false, nil
		}
	case SessionActionMarkRecovering:
		switch state {
		case SessionStateActive, SessionStateReleased:
			return SessionStateRecovering, true, nil
		case SessionStateRecovering:
			return state, false, nil
		}
	case SessionActionRecover:
		switch state {
		case SessionStateRecovering:
			return SessionStateActive, true, nil
		case SessionStateActive:
			return state, false, nil
		}
	}
	return "", false, ErrInvalidInput
}

type RuntimeSession struct {
	Ref             SessionRef
	State           SessionState
	UpstreamShellID string
	RecoveryReason  string
	Version         uint64
	LastActivityAt  time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const (
	MaxRecoverableRuntimeSessions = 1000
	MaxSessionRecoveryReasonBytes = 64
	MaxSessionLifetime            = 24 * time.Hour
	aioGenerationSentinelPrefix   = "newx-generation-"
)

type AcquireRuntimeSessionInput struct {
	Key                SessionKey
	CandidateSessionID string
	RuntimeGeneration  uint64
	ExpiresAt          time.Time
	Now                time.Time
}

func NormalizeAcquireRuntimeSessionInput(input AcquireRuntimeSessionInput) (AcquireRuntimeSessionInput, error) {
	key, err := NormalizeSessionKey(input.Key)
	if err != nil || ValidatePhase1SessionProfile(key.Profile) != nil ||
		!validCanonicalSessionUUID(input.CandidateSessionID) || input.RuntimeGeneration == 0 ||
		input.Now.IsZero() || input.ExpiresAt.IsZero() {
		return AcquireRuntimeSessionInput{}, ErrInvalidInput
	}
	now := input.Now.UTC()
	expiresAt := input.ExpiresAt.UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(MaxSessionLifetime)) {
		return AcquireRuntimeSessionInput{}, ErrInvalidInput
	}
	input.Key = key
	input.Now = now
	input.ExpiresAt = expiresAt
	return input, nil
}

type BindRuntimeSessionInput struct {
	Ref             SessionRef
	ExpectedVersion uint64
	UpstreamShellID string
	ExpiresAt       time.Time
	Now             time.Time
}

func NormalizeBindRuntimeSessionInput(input BindRuntimeSessionInput) (BindRuntimeSessionInput, error) {
	ref, err := NormalizeSessionRef(input.Ref)
	if err != nil || input.ExpectedVersion < InitialVersion ||
		!validSessionIdentifier(input.UpstreamShellID) || isReservedAIOGenerationID(input.UpstreamShellID) ||
		input.Now.IsZero() || input.ExpiresAt.IsZero() {
		return BindRuntimeSessionInput{}, ErrInvalidInput
	}
	now := input.Now.UTC()
	expiresAt := input.ExpiresAt.UTC()
	if !expiresAt.After(now) || expiresAt.After(now.Add(MaxSessionLifetime)) {
		return BindRuntimeSessionInput{}, ErrInvalidInput
	}
	input.Ref = ref
	input.Now = now
	input.ExpiresAt = expiresAt
	return input, nil
}

type TransitionRuntimeSessionInput struct {
	Ref                   SessionRef
	ExpectedVersion       uint64
	Action                SessionAction
	NextRuntimeGeneration uint64
	UpstreamShellID       string
	RecoveryReason        string
	ExpiresAt             time.Time
	Now                   time.Time
}

func NormalizeTransitionRuntimeSessionInput(input TransitionRuntimeSessionInput) (TransitionRuntimeSessionInput, error) {
	ref, err := NormalizeSessionRef(input.Ref)
	if err != nil || input.ExpectedVersion < InitialVersion || input.Now.IsZero() {
		return TransitionRuntimeSessionInput{}, ErrInvalidInput
	}
	input.Ref = ref
	input.Now = input.Now.UTC()
	switch input.Action {
	case SessionActionRelease, SessionActionDestroy:
		if input.NextRuntimeGeneration != 0 || input.UpstreamShellID != "" ||
			input.RecoveryReason != "" || !input.ExpiresAt.IsZero() {
			return TransitionRuntimeSessionInput{}, ErrInvalidInput
		}
	case SessionActionMarkRecovering:
		if input.NextRuntimeGeneration != 0 || input.UpstreamShellID != "" || !input.ExpiresAt.IsZero() ||
			!validRecoveryReason(input.RecoveryReason) {
			return TransitionRuntimeSessionInput{}, ErrInvalidInput
		}
	case SessionActionRecover:
		if input.NextRuntimeGeneration == 0 || !validSessionIdentifier(input.UpstreamShellID) ||
			isReservedAIOGenerationID(input.UpstreamShellID) || input.RecoveryReason != "" || input.ExpiresAt.IsZero() {
			return TransitionRuntimeSessionInput{}, ErrInvalidInput
		}
		input.ExpiresAt = input.ExpiresAt.UTC()
		if !input.ExpiresAt.After(input.Now) || input.ExpiresAt.After(input.Now.Add(MaxSessionLifetime)) {
			return TransitionRuntimeSessionInput{}, ErrInvalidInput
		}
	default:
		return TransitionRuntimeSessionInput{}, ErrInvalidInput
	}
	return input, nil
}

type ListRecoverableRuntimeSessionsInput struct {
	DeploymentID     string
	BeforeGeneration uint64
	Limit            int
}

func NormalizeListRecoverableRuntimeSessionsInput(input ListRecoverableRuntimeSessionsInput) (ListRecoverableRuntimeSessionsInput, error) {
	if !validSessionIdentifier(input.DeploymentID) || input.BeforeGeneration == 0 ||
		input.Limit < 1 || input.Limit > MaxRecoverableRuntimeSessions {
		return ListRecoverableRuntimeSessionsInput{}, ErrInvalidInput
	}
	return input, nil
}

type AIOGenerationState struct {
	DeploymentID string
	Generation   uint64
	SentinelID   string
}

func NormalizeAIOGenerationState(state AIOGenerationState) (AIOGenerationState, error) {
	if state.DeploymentID == "" && state.Generation == 0 && state.SentinelID == "" {
		return state, nil
	}
	if !validSessionIdentifier(state.DeploymentID) || state.Generation == 0 || state.Generation == ^uint64(0) || !isAIOGenerationSentinel(state.SentinelID) {
		return AIOGenerationState{}, ErrConfigurationInvalid
	}
	return state, nil
}

func (AIOGenerationState) String() string {
	return "sandbox.AIOGenerationState{runtime:<redacted>}"
}
func (AIOGenerationState) GoString() string {
	return "sandbox.AIOGenerationState{runtime:<redacted>}"
}
func (AIOGenerationState) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.AIOGenerationState{runtime:<redacted>}")
}

type CompareAndReplaceAIOSentinelInput struct {
	DeploymentID        string
	ExpectedSentinelID  string
	CandidateSentinelID string
}

func (CompareAndReplaceAIOSentinelInput) String() string {
	return "sandbox.CompareAndReplaceAIOSentinelInput{runtime:<redacted>}"
}
func (CompareAndReplaceAIOSentinelInput) GoString() string {
	return "sandbox.CompareAndReplaceAIOSentinelInput{runtime:<redacted>}"
}
func (CompareAndReplaceAIOSentinelInput) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.CompareAndReplaceAIOSentinelInput{runtime:<redacted>}")
}

func (AcquireRuntimeSessionInput) String() string {
	return "sandbox.AcquireRuntimeSessionInput{identity:<redacted>}"
}
func (AcquireRuntimeSessionInput) GoString() string {
	return "sandbox.AcquireRuntimeSessionInput{identity:<redacted>}"
}
func (AcquireRuntimeSessionInput) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.AcquireRuntimeSessionInput{identity:<redacted>}")
}

func (BindRuntimeSessionInput) String() string {
	return "sandbox.BindRuntimeSessionInput{identity:<redacted>}"
}
func (BindRuntimeSessionInput) GoString() string {
	return "sandbox.BindRuntimeSessionInput{identity:<redacted>}"
}
func (BindRuntimeSessionInput) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.BindRuntimeSessionInput{identity:<redacted>}")
}

func (TransitionRuntimeSessionInput) String() string {
	return "sandbox.TransitionRuntimeSessionInput{identity:<redacted>}"
}
func (TransitionRuntimeSessionInput) GoString() string {
	return "sandbox.TransitionRuntimeSessionInput{identity:<redacted>}"
}
func (TransitionRuntimeSessionInput) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.TransitionRuntimeSessionInput{identity:<redacted>}")
}
func NormalizeCompareAndReplaceAIOSentinelInput(input CompareAndReplaceAIOSentinelInput) (CompareAndReplaceAIOSentinelInput, error) {
	if !validSessionIdentifier(input.DeploymentID) ||
		(input.ExpectedSentinelID != "" && !isAIOGenerationSentinel(input.ExpectedSentinelID)) ||
		!isAIOGenerationSentinel(input.CandidateSentinelID) ||
		input.ExpectedSentinelID == input.CandidateSentinelID {
		return CompareAndReplaceAIOSentinelInput{}, ErrInvalidInput
	}
	return input, nil
}

func NormalizeAIOGenerationDeploymentID(deploymentID string) (string, error) {
	if !validSessionIdentifier(deploymentID) {
		return "", ErrInvalidInput
	}
	return deploymentID, nil
}

func validCanonicalSessionUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}

func validRecoveryReason(value string) bool {
	return value != "" && len(value) <= MaxSessionRecoveryReasonBytes && validSessionIdentifier(value)
}

func isAIOGenerationSentinel(value string) bool {
	if len(value) != len(aioGenerationSentinelPrefix)+32 || !strings.HasPrefix(value, aioGenerationSentinelPrefix) {
		return false
	}
	for _, character := range value[len(aioGenerationSentinelPrefix):] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func isReservedAIOGenerationID(value string) bool {
	return strings.HasPrefix(value, aioGenerationSentinelPrefix)
}

func (RuntimeSession) String() string   { return "sandbox.RuntimeSession{identity:<redacted>}" }
func (RuntimeSession) GoString() string { return "sandbox.RuntimeSession{identity:<redacted>}" }
func (RuntimeSession) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.RuntimeSession{identity:<redacted>}")
}

func validSessionIdentifier(value string) bool {
	if value == "" || len(value) > MaxSessionIdentifierBytes || !utf8.ValidString(value) ||
		strings.TrimSpace(value) != value || value == "." || value == ".." {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

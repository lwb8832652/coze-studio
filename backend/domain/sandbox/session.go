// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
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
	IdentityID      int64
	State           SessionState
	UpstreamShellID string
	RecoveryReason  string
	Version         uint64
	LastActivityAt  time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
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

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
)

const (
	ProviderExecutionOwnerTokenBytes         = 32
	ProviderExecutionInitialVersion   uint64 = 1
	MaxProviderExecutionEnvelopeBytes        = sandboxcontract.MaxExecutionCheckpointEnvelopeBytes
)

var (
	ErrProviderExecutionInvalid            = errors.New("appdev provider execution input is invalid")
	ErrProviderExecutionNotFound           = errors.New("appdev provider execution not found")
	ErrProviderExecutionConflict           = errors.New("appdev provider execution conflict")
	ErrProviderExecutionOwnerConflict      = fmt.Errorf("%w: owner", ErrProviderExecutionConflict)
	ErrProviderExecutionVersionConflict    = fmt.Errorf("%w: version", ErrProviderExecutionConflict)
	ErrProviderExecutionGenerationConflict = fmt.Errorf("%w: generation", ErrProviderExecutionConflict)
	ErrProviderExecutionStateConflict      = fmt.Errorf("%w: state", ErrProviderExecutionConflict)
	ErrProviderExecutionOperationConflict  = fmt.Errorf("%w: operation", ErrProviderExecutionConflict)
	ErrProviderExecutionIDConflict         = fmt.Errorf("%w: provider execution id", ErrProviderExecutionConflict)
	ErrProviderExecutionUnavailable        = errors.New("appdev provider execution repository is unavailable")
	ErrProviderExecutionSecret             = errors.New("appdev provider execution secret cannot be serialized")
)

type ProviderExecutionDesiredState string

const (
	ProviderExecutionDesiredRun  ProviderExecutionDesiredState = "run"
	ProviderExecutionDesiredStop ProviderExecutionDesiredState = "stop"
)

type ProviderExecutionObservedState string

const (
	ProviderExecutionObservedPending         ProviderExecutionObservedState = "pending"
	ProviderExecutionObservedSubmitting      ProviderExecutionObservedState = "submitting"
	ProviderExecutionObservedRunning         ProviderExecutionObservedState = "running"
	ProviderExecutionObservedSucceeded       ProviderExecutionObservedState = "succeeded"
	ProviderExecutionObservedFailed          ProviderExecutionObservedState = "failed"
	ProviderExecutionObservedCanceled        ProviderExecutionObservedState = "canceled"
	ProviderExecutionObservedTimedOut        ProviderExecutionObservedState = "timed_out"
	ProviderExecutionObservedCleanupPending  ProviderExecutionObservedState = "cleanup_pending"
	ProviderExecutionObservedCleanupComplete ProviderExecutionObservedState = "cleanup_complete"
)

type ProviderExecutionLaunchState string

const (
	ProviderExecutionLaunchNone            ProviderExecutionLaunchState = "none"
	ProviderExecutionLaunchPrepared        ProviderExecutionLaunchState = "prepared"
	ProviderExecutionLaunchSubmitted       ProviderExecutionLaunchState = "submitted"
	ProviderExecutionLaunchLegacySubmitted ProviderExecutionLaunchState = "legacy_submitted"
	ProviderExecutionLaunchQuarantined     ProviderExecutionLaunchState = "quarantined"
	ProviderExecutionLaunchComplete        ProviderExecutionLaunchState = "complete"
	ProviderExecutionLaunchAborted         ProviderExecutionLaunchState = "aborted"
)

type ProviderExecutionLaunchRequestDigest [sha256.Size]byte

func (digest ProviderExecutionLaunchRequestDigest) IsZero() bool {
	var zero ProviderExecutionLaunchRequestDigest
	return subtle.ConstantTimeCompare(digest[:], zero[:]) == 1
}

func (digest ProviderExecutionLaunchRequestDigest) Equal(other ProviderExecutionLaunchRequestDigest) bool {
	return subtle.ConstantTimeCompare(digest[:], other[:]) == 1
}

func (digest ProviderExecutionLaunchRequestDigest) EqualBytes(other []byte) bool {
	var fixed [sha256.Size]byte
	copy(fixed[:], other)
	return subtle.ConstantTimeEq(int32(len(other)), sha256.Size)&
		subtle.ConstantTimeCompare(digest[:], fixed[:]) == 1
}

func (digest ProviderExecutionLaunchRequestDigest) Bytes() []byte {
	return append([]byte(nil), digest[:]...)
}

func (ProviderExecutionLaunchRequestDigest) String() string {
	return "[REDACTED launch request digest]"
}
func (ProviderExecutionLaunchRequestDigest) GoString() string {
	return "[REDACTED launch request digest]"
}
func (ProviderExecutionLaunchRequestDigest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED launch request digest]")
}
func (ProviderExecutionLaunchRequestDigest) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderExecutionSecret
}

type ProviderExecutionArtifactStatus string

const (
	ProviderExecutionArtifactNone            ProviderExecutionArtifactStatus = "none"
	ProviderExecutionArtifactBeginPending    ProviderExecutionArtifactStatus = "begin_pending"
	ProviderExecutionArtifactBuilding        ProviderExecutionArtifactStatus = "building"
	ProviderExecutionArtifactDescriptorReady ProviderExecutionArtifactStatus = "descriptor_ready"
	ProviderExecutionArtifactPublishing      ProviderExecutionArtifactStatus = "publishing"
	ProviderExecutionArtifactReady           ProviderExecutionArtifactStatus = "ready"
	ProviderExecutionArtifactFailed          ProviderExecutionArtifactStatus = "failed"
)

type ProviderExecutionArtifactKind string

const (
	ProviderExecutionArtifactKindAppDevBuildArchive ProviderExecutionArtifactKind = "appdev_build_archive"
	MaxProviderExecutionBuildArtifactBytes          int64                         = 100 * 1024 * 1024
)

type ProviderBuildArtifactDescriptor struct {
	Kind   ProviderExecutionArtifactKind
	Digest string
	Size   int64
}

func NormalizeProviderBuildArtifactDescriptor(input ProviderBuildArtifactDescriptor) (ProviderBuildArtifactDescriptor, error) {
	if input.Kind != ProviderExecutionArtifactKindAppDevBuildArchive || input.Size <= 0 ||
		input.Size > MaxProviderExecutionBuildArtifactBytes {
		return ProviderBuildArtifactDescriptor{}, ErrProviderExecutionInvalid
	}
	digest, err := ParseArtifactGrantDigest(input.Digest)
	if err != nil || digest.IsZero() {
		return ProviderBuildArtifactDescriptor{}, ErrProviderExecutionInvalid
	}
	input.Digest = digest.String()
	return input, nil
}

var allProviderExecutionObservedStates = []ProviderExecutionObservedState{
	ProviderExecutionObservedPending,
	ProviderExecutionObservedSubmitting,
	ProviderExecutionObservedRunning,
	ProviderExecutionObservedSucceeded,
	ProviderExecutionObservedFailed,
	ProviderExecutionObservedCanceled,
	ProviderExecutionObservedTimedOut,
	ProviderExecutionObservedCleanupPending,
	ProviderExecutionObservedCleanupComplete,
}

var providerExecutionRecoverableObservedStates = []ProviderExecutionObservedState{
	ProviderExecutionObservedPending,
	ProviderExecutionObservedSubmitting,
	ProviderExecutionObservedRunning,
	ProviderExecutionObservedSucceeded,
	ProviderExecutionObservedFailed,
	ProviderExecutionObservedCanceled,
	ProviderExecutionObservedTimedOut,
	ProviderExecutionObservedCleanupPending,
}

func ProviderExecutionRecoverableObservedStates() []ProviderExecutionObservedState {
	return append([]ProviderExecutionObservedState(nil), providerExecutionRecoverableObservedStates...)
}

func IsProviderExecutionRecoverableObservedState(state ProviderExecutionObservedState) bool {
	return providerExecutionStateIn(state, providerExecutionRecoverableObservedStates)
}

func ProviderExecutionCleanupSourceStates() []ProviderExecutionObservedState {
	states := make([]ProviderExecutionObservedState, 0, len(providerExecutionRecoverableObservedStates)-1)
	for _, state := range providerExecutionRecoverableObservedStates {
		if state != ProviderExecutionObservedCleanupPending {
			states = append(states, state)
		}
	}
	return states
}

func IsProviderExecutionCleanupSourceState(state ProviderExecutionObservedState) bool {
	return state != ProviderExecutionObservedCleanupPending && IsProviderExecutionRecoverableObservedState(state)
}

func providerExecutionStateIn(state ProviderExecutionObservedState, states []ProviderExecutionObservedState) bool {
	for _, candidate := range states {
		if state == candidate {
			return true
		}
	}
	return false
}

type ProviderExecutionOwnerHash [sha256.Size]byte

func (h ProviderExecutionOwnerHash) IsZero() bool {
	var zero ProviderExecutionOwnerHash
	return subtle.ConstantTimeCompare(h[:], zero[:]) == 1
}

func (h ProviderExecutionOwnerHash) Equal(other ProviderExecutionOwnerHash) bool {
	return subtle.ConstantTimeCompare(h[:], other[:]) == 1
}

func (h ProviderExecutionOwnerHash) EqualBytes(other []byte) bool {
	var fixed [sha256.Size]byte
	copy(fixed[:], other)
	lengthMatches := subtle.ConstantTimeEq(int32(len(other)), sha256.Size)
	valueMatches := subtle.ConstantTimeCompare(h[:], fixed[:])
	return lengthMatches&valueMatches == 1
}

func (h ProviderExecutionOwnerHash) Bytes() []byte { return append([]byte(nil), h[:]...) }
func (ProviderExecutionOwnerHash) String() string  { return "[REDACTED owner identity hash]" }
func (ProviderExecutionOwnerHash) GoString() string {
	return "[REDACTED owner identity hash]"
}
func (ProviderExecutionOwnerHash) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED owner identity hash]")
}

type ProviderExecutionOperationHash [sha256.Size]byte

func HashProviderExecutionOperationID(value string) (ProviderExecutionOperationHash, error) {
	if !validProviderExecutionOpaque(value, 128) {
		return ProviderExecutionOperationHash{}, ErrProviderExecutionInvalid
	}
	return ProviderExecutionOperationHash(sha256.Sum256([]byte(value))), nil
}

func HashProviderExecutionReleaseOperationID(value string, ownerHash ProviderExecutionOwnerHash) (ProviderExecutionOperationHash, error) {
	if !validProviderExecutionOpaque(value, 128) || ownerHash.IsZero() {
		return ProviderExecutionOperationHash{}, ErrProviderExecutionInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_execution_release_owner:v1\x00"))
	_, _ = hasher.Write(ownerHash[:])
	_, _ = hasher.Write([]byte(value))
	var result ProviderExecutionOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// HashProviderExecutionLaunchOperationID binds provider submission
// idempotency to the immutable execution identity. Owner identity and version
// are deliberately excluded so a replacement owner can reconcile the same
// launch after a crash.
func HashProviderExecutionLaunchOperationID(value string, cas ProviderExecutionOwnerCAS) (ProviderExecutionOperationHash, error) {
	if !validProviderExecutionOpaque(value, 128) || ValidateProviderExecutionOwnerCAS(cas) != nil {
		return ProviderExecutionOperationHash{}, ErrProviderExecutionInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_execution_launch:v1\x00"))
	_, _ = hasher.Write([]byte(cas.SpaceID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProjectID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(strconv.FormatUint(cas.Generation, 10)))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProviderKey))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProviderScope))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(value))
	var result ProviderExecutionOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// HashProviderExecutionCleanupOperationID binds cleanup idempotency to the
// execution identity and owner capability that authorized the cleanup.
func HashProviderExecutionCleanupOperationID(value string, cas ProviderExecutionOwnerCAS) (ProviderExecutionOperationHash, error) {
	if !validProviderExecutionOpaque(value, 128) || ValidateProviderExecutionOwnerCAS(cas) != nil {
		return ProviderExecutionOperationHash{}, ErrProviderExecutionInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_execution_cleanup:v1\x00"))
	_, _ = hasher.Write([]byte(cas.SpaceID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProjectID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(strconv.FormatUint(cas.Generation, 10)))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProviderKey))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProviderScope))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write(cas.OwnerHash[:])
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(strconv.FormatUint(cas.OwnerEpoch, 10)))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(value))
	var result ProviderExecutionOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

func (h ProviderExecutionOperationHash) IsZero() bool {
	var zero ProviderExecutionOperationHash
	return subtle.ConstantTimeCompare(h[:], zero[:]) == 1
}

func (h ProviderExecutionOperationHash) Equal(other ProviderExecutionOperationHash) bool {
	return subtle.ConstantTimeCompare(h[:], other[:]) == 1
}

func (h ProviderExecutionOperationHash) EqualBytes(other []byte) bool {
	var fixed [sha256.Size]byte
	copy(fixed[:], other)
	lengthMatches := subtle.ConstantTimeEq(int32(len(other)), sha256.Size)
	valueMatches := subtle.ConstantTimeCompare(h[:], fixed[:])
	return lengthMatches&valueMatches == 1
}

func (h ProviderExecutionOperationHash) Bytes() []byte  { return append([]byte(nil), h[:]...) }
func (ProviderExecutionOperationHash) String() string   { return "[REDACTED operation hash]" }
func (ProviderExecutionOperationHash) GoString() string { return "[REDACTED operation hash]" }
func (ProviderExecutionOperationHash) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED operation hash]")
}
func (ProviderExecutionOperationHash) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderExecutionSecret
}

type ProviderExecutionOwnerToken struct {
	secret [ProviderExecutionOwnerTokenBytes]byte
	valid  bool
}

func NewProviderExecutionOwnerToken(random io.Reader) (ProviderExecutionOwnerToken, error) {
	var token ProviderExecutionOwnerToken
	if random == nil {
		return token, ErrProviderExecutionInvalid
	}
	if _, err := io.ReadFull(random, token.secret[:]); err != nil {
		return ProviderExecutionOwnerToken{}, ErrProviderExecutionUnavailable
	}
	token.valid = true
	return token, nil
}

func (t ProviderExecutionOwnerToken) IsZero() bool { return !t.valid }

func (t ProviderExecutionOwnerToken) OwnerIdentityHash() ProviderExecutionOwnerHash {
	if !t.valid {
		return ProviderExecutionOwnerHash{}
	}
	return ProviderExecutionOwnerHash(sha256.Sum256(t.secret[:]))
}

func (ProviderExecutionOwnerToken) String() string   { return "[REDACTED owner token]" }
func (ProviderExecutionOwnerToken) GoString() string { return "[REDACTED owner token]" }
func (ProviderExecutionOwnerToken) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED owner token]")
}
func (ProviderExecutionOwnerToken) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderExecutionSecret
}
func (ProviderExecutionOwnerToken) MarshalText() ([]byte, error) {
	return nil, ErrProviderExecutionSecret
}

type ProviderExecution struct {
	ID                           string
	SpaceID                      string
	ProjectID                    string
	Generation                   uint64
	IdempotencyKey               string
	DesiredState                 ProviderExecutionDesiredState
	ObservedState                ProviderExecutionObservedState
	ProviderKey                  string
	ProviderScope                domainsandbox.Scope
	ProviderExecutionID          string
	SubmissionStartedAt          *time.Time
	LaunchState                  ProviderExecutionLaunchState
	LaunchOperationHash          ProviderExecutionOperationHash
	LaunchProviderOperationID    string
	LaunchRequestDigest          ProviderExecutionLaunchRequestDigest
	LaunchExpiresAt              *time.Time
	CheckpointEnvelope           string
	CheckpointWriteRevision      uint64
	CheckpointWritePending       bool
	CheckpointWriteOperationHash ProviderExecutionOperationHash
	CheckpointWriteExpiresAt     *time.Time
	CheckpointLastOperationHash  ProviderExecutionOperationHash
	CleanupOperationHash         ProviderExecutionOperationHash
	TerminalOperationHash        ProviderExecutionOperationHash
	ReleaseOwnerOperationHash    ProviderExecutionOperationHash
	ProviderLeaseExpiresAt       *time.Time
	OwnerIdentityHash            ProviderExecutionOwnerHash
	OwnerEpoch                   uint64
	OwnerExpiresAt               *time.Time
	PreviewRoute                 string
	ArtifactObjectKey            string
	BuildOperationID             string
	BuildOperationHash           ProviderExecutionOperationHash
	ArtifactStatus               ProviderExecutionArtifactStatus
	ArtifactKind                 ProviderExecutionArtifactKind
	ArtifactDigest               string
	ArtifactSize                 int64
	ArtifactVersion              uint64
	BuildStartedAt               *time.Time
	ArtifactUpdatedAt            *time.Time
	ArtifactSafeErrorCode        string
	ArtifactSafeErrorMessage     string
	SafeErrorCode                string
	SafeErrorMessage             string
	Version                      uint64
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

// HashProviderExecutionBuildOperationID binds build idempotency to immutable
// execution identity. Owner capability is deliberately excluded so an expired
// owner can be replaced without changing the provider idempotency key.
func HashProviderExecutionBuildOperationID(value string, cas ProviderExecutionOwnerCAS, providerExecutionID string) (ProviderExecutionOperationHash, error) {
	if !validProviderExecutionOpaque(value, 128) || ValidateProviderExecutionOwnerCAS(cas) != nil ||
		!ValidProviderExecutionProviderID(providerExecutionID) {
		return ProviderExecutionOperationHash{}, ErrProviderExecutionInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_execution_build:v1\x00"))
	_, _ = hasher.Write([]byte(cas.SpaceID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProjectID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(strconv.FormatUint(cas.Generation, 10)))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProviderKey))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(cas.ProviderScope))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(providerExecutionID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(value))
	var result ProviderExecutionOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

// RecoverableProviderExecution is the only repository list projection. It
// intentionally cannot carry checkpoint ciphertext, object keys, owner hashes,
// idempotency keys, or provider execution IDs.
type RecoverableProviderExecution struct {
	ID                     string
	SpaceID                string
	ProjectID              string
	Generation             uint64
	HasProviderExecution   bool
	HasCheckpoint          bool
	DesiredState           ProviderExecutionDesiredState
	ObservedState          ProviderExecutionObservedState
	ProviderKey            string
	ProviderScope          domainsandbox.Scope
	LaunchState            ProviderExecutionLaunchState
	ProviderLeaseExpiresAt *time.Time
	OwnerEpoch             uint64
	OwnerExpiresAt         *time.Time
	PreviewRoute           string
	SafeErrorCode          string
	SafeErrorMessage       string
	Version                uint64
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (e ProviderExecution) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "ProviderExecution{ID:%q Generation:%d Desired:%q Observed:%q Version:%d [REDACTED internals]}",
		e.ID, e.Generation, e.DesiredState, e.ObservedState, e.Version)
}
func (e ProviderExecution) String() string { return fmt.Sprintf("%v", providerExecutionFormatAlias(e)) }
func (e ProviderExecution) GoString() string {
	return fmt.Sprintf("%v", providerExecutionFormatAlias(e))
}

type providerExecutionFormatAlias ProviderExecution

func (e providerExecutionFormatAlias) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "ProviderExecution{ID:%q Generation:%d Desired:%q Observed:%q Version:%d [REDACTED internals]}",
		e.ID, e.Generation, e.DesiredState, e.ObservedState, e.Version)
}

func IsProviderExecutionObservedTerminal(state ProviderExecutionObservedState) bool {
	switch state {
	case ProviderExecutionObservedSucceeded, ProviderExecutionObservedFailed,
		ProviderExecutionObservedCanceled, ProviderExecutionObservedTimedOut,
		ProviderExecutionObservedCleanupComplete:
		return true
	default:
		return false
	}
}

func CanAdvanceProviderExecutionObserved(from, to ProviderExecutionObservedState) bool {
	if !validProviderExecutionObservedState(from) || !validProviderExecutionObservedState(to) {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case ProviderExecutionObservedPending:
		return to != ProviderExecutionObservedPending
	case ProviderExecutionObservedSubmitting:
		return to != ProviderExecutionObservedPending && to != ProviderExecutionObservedSubmitting
	case ProviderExecutionObservedRunning:
		return IsProviderExecutionObservedTerminal(to) || to == ProviderExecutionObservedCleanupPending
	case ProviderExecutionObservedSucceeded, ProviderExecutionObservedFailed,
		ProviderExecutionObservedCanceled, ProviderExecutionObservedTimedOut:
		return to == ProviderExecutionObservedCleanupPending || to == ProviderExecutionObservedCleanupComplete
	case ProviderExecutionObservedCleanupPending:
		return to == ProviderExecutionObservedCleanupComplete
	case ProviderExecutionObservedCleanupComplete:
		return false
	default:
		return false
	}
}

func ProviderExecutionObservedPredecessors(next ProviderExecutionObservedState) []ProviderExecutionObservedState {
	predecessors := make([]ProviderExecutionObservedState, 0, len(allProviderExecutionObservedStates))
	for _, candidate := range allProviderExecutionObservedStates {
		if CanAdvanceProviderExecutionObserved(candidate, next) {
			predecessors = append(predecessors, candidate)
		}
	}
	return predecessors
}

func NormalizeProviderExecutionPreviewRoute(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\\x00\r\n\t") || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "", ErrProviderExecutionInvalid
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", ErrProviderExecutionInvalid
	}
	if cleaned := path.Clean(parsed.Path); cleaned != parsed.Path || strings.Contains(cleaned, "..") {
		return "", ErrProviderExecutionInvalid
	}
	return value, nil
}

func NormalizeProviderExecutionArtifactObjectKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > 512 || !utf8.ValidString(value) || strings.ContainsAny(value, "\\\x00\r\n\t?#") || strings.HasPrefix(value, "/") {
		return "", ErrProviderExecutionInvalid
	}
	if cleaned := path.Clean(value); cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrProviderExecutionInvalid
	}
	return value, nil
}

func ValidProviderExecutionSpaceID(value string) bool {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == strings.TrimSpace(value)
}

func ValidProviderExecutionProjectID(value string) bool {
	return validProviderExecutionIdentifier(value, 64)
}

func ValidProviderExecutionID(value string) bool {
	return validProviderExecutionIdentifier(value, 64)
}

func ValidProviderExecutionProviderID(value string) bool {
	return validProviderExecutionOpaque(value, 128)
}

func ValidProviderExecutionIdempotencyKey(value string) bool {
	return validProviderExecutionOpaque(value, 128)
}

func ValidProviderExecutionSafeError(code, message string) bool {
	if code != "" && !validProviderExecutionIdentifier(code, 64) {
		return false
	}
	return len(message) <= 255 && utf8.ValidString(message) && !strings.ContainsAny(message, "\x00\r\n\t")
}

func ValidateProviderExecutionOwnerCAS(input ProviderExecutionOwnerCAS) error {
	if !ValidProviderExecutionSpaceID(input.SpaceID) || !ValidProviderExecutionProjectID(input.ProjectID) ||
		input.Generation == 0 || input.ExpectedVersion == 0 || input.OwnerHash.IsZero() || input.OwnerEpoch == 0 {
		return ErrProviderExecutionInvalid
	}
	if input.ProviderKey != "" && domainsandbox.ValidateProviderKey(input.ProviderKey) != nil {
		return ErrProviderExecutionInvalid
	}
	if input.ProviderScope != "" && input.ProviderScope != domainsandbox.ScopeAppDev {
		return ErrProviderExecutionInvalid
	}
	return nil
}

func validProviderExecutionDesiredState(state ProviderExecutionDesiredState) bool {
	return state == ProviderExecutionDesiredRun || state == ProviderExecutionDesiredStop
}

func validProviderExecutionObservedState(state ProviderExecutionObservedState) bool {
	for _, candidate := range allProviderExecutionObservedStates {
		if state == candidate {
			return true
		}
	}
	return false
}

func validProviderExecutionIdentifier(value string, limit int) bool {
	trimmed := strings.TrimSpace(value)
	if value == "" || len(value) > limit || value != trimmed {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validProviderExecutionOpaque(value string, limit int) bool {
	if value == "" || len(value) > limit || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for index := range value {
		if value[index] < 0x21 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func (ProviderExecutionOwnerHash) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderExecutionSecret
}

func (e ProviderExecution) MarshalJSON() ([]byte, error) {
	type safe struct {
		ID            string                         `json:"id"`
		SpaceID       string                         `json:"space_id"`
		ProjectID     string                         `json:"project_id"`
		Generation    uint64                         `json:"generation"`
		DesiredState  ProviderExecutionDesiredState  `json:"desired_state"`
		ObservedState ProviderExecutionObservedState `json:"observed_state"`
		ProviderKey   string                         `json:"provider_key"`
		ProviderScope domainsandbox.Scope            `json:"provider_scope"`
		Version       uint64                         `json:"version"`
	}
	return json.Marshal(safe{
		ID: e.ID, SpaceID: e.SpaceID, ProjectID: e.ProjectID, Generation: e.Generation,
		DesiredState: e.DesiredState, ObservedState: e.ObservedState,
		ProviderKey: e.ProviderKey, ProviderScope: e.ProviderScope, Version: e.Version,
	})
}

func ValidateProviderExecutionEntity(entity *ProviderExecution) error {
	if entity == nil || !ValidProviderExecutionID(entity.ID) || !ValidProviderExecutionSpaceID(entity.SpaceID) ||
		!ValidProviderExecutionProjectID(entity.ProjectID) || entity.Generation == 0 ||
		!ValidProviderExecutionIdempotencyKey(entity.IdempotencyKey) || !validProviderExecutionDesiredState(entity.DesiredState) ||
		!validProviderExecutionObservedState(entity.ObservedState) || domainsandbox.ValidateProviderKey(entity.ProviderKey) != nil ||
		entity.ProviderScope != domainsandbox.ScopeAppDev || entity.Version == 0 || entity.CreatedAt.IsZero() || entity.UpdatedAt.IsZero() {
		return ErrProviderExecutionInvalid
	}
	if entity.ProviderExecutionID != "" && !ValidProviderExecutionProviderID(entity.ProviderExecutionID) {
		return ErrProviderExecutionInvalid
	}
	if len(entity.CheckpointEnvelope) > MaxProviderExecutionEnvelopeBytes ||
		(entity.CheckpointEnvelope != "" && !validProviderExecutionOpaque(entity.CheckpointEnvelope, MaxProviderExecutionEnvelopeBytes)) {
		return ErrProviderExecutionInvalid
	}
	if _, err := NormalizeProviderExecutionPreviewRoute(entity.PreviewRoute); err != nil {
		return err
	}
	if _, err := NormalizeProviderExecutionArtifactObjectKey(entity.ArtifactObjectKey); err != nil {
		return err
	}
	if !ValidProviderExecutionSafeError(entity.SafeErrorCode, entity.SafeErrorMessage) {
		return ErrProviderExecutionInvalid
	}
	if err := validateProviderExecutionBuildState(entity); err != nil {
		return err
	}
	ownerPresent := !entity.OwnerIdentityHash.IsZero()
	if ownerPresent != (entity.OwnerExpiresAt != nil) {
		return ErrProviderExecutionInvalid
	}
	if entity.CheckpointWritePending {
		if entity.CheckpointWriteRevision == 0 || entity.CheckpointWriteOperationHash.IsZero() || entity.CheckpointWriteExpiresAt == nil {
			return ErrProviderExecutionInvalid
		}
	} else if !entity.CheckpointWriteOperationHash.IsZero() || entity.CheckpointWriteExpiresAt != nil {
		return ErrProviderExecutionInvalid
	}
	if entity.CheckpointEnvelope != "" && (entity.CheckpointWriteRevision == 0 || entity.CheckpointLastOperationHash.IsZero()) {
		return ErrProviderExecutionInvalid
	}
	if entity.ProviderExecutionID != "" && entity.SubmissionStartedAt == nil {
		return ErrProviderExecutionInvalid
	}
	launchState := entity.LaunchState
	if launchState == "" {
		launchState = ProviderExecutionLaunchNone
	}
	switch launchState {
	case ProviderExecutionLaunchNone:
		if !entity.LaunchOperationHash.IsZero() || entity.LaunchProviderOperationID != "" ||
			!entity.LaunchRequestDigest.IsZero() || entity.LaunchExpiresAt != nil {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionLaunchPrepared:
		if entity.LaunchOperationHash.IsZero() ||
			!validProviderExecutionOpaque(entity.LaunchProviderOperationID, 128) ||
			entity.LaunchRequestDigest.IsZero() || entity.LaunchExpiresAt == nil ||
			entity.SubmissionStartedAt != nil || entity.ProviderExecutionID != "" ||
			entity.ObservedState != ProviderExecutionObservedPending {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionLaunchSubmitted:
		if entity.LaunchOperationHash.IsZero() ||
			!validProviderExecutionOpaque(entity.LaunchProviderOperationID, 128) ||
			entity.LaunchRequestDigest.IsZero() || entity.LaunchExpiresAt == nil ||
			entity.SubmissionStartedAt == nil ||
			(entity.ObservedState != ProviderExecutionObservedSubmitting &&
				entity.ObservedState != ProviderExecutionObservedRunning) {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionLaunchLegacySubmitted:
		if entity.LaunchOperationHash.IsZero() ||
			!validProviderExecutionOpaque(entity.LaunchProviderOperationID, 128) ||
			!entity.LaunchRequestDigest.IsZero() || entity.LaunchExpiresAt != nil ||
			entity.SubmissionStartedAt == nil || entity.ProviderExecutionID != "" ||
			entity.CheckpointEnvelope != "" ||
			entity.ObservedState != ProviderExecutionObservedSubmitting {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionLaunchQuarantined:
		if !entity.LaunchOperationHash.IsZero() || entity.LaunchProviderOperationID != "" ||
			!entity.LaunchRequestDigest.IsZero() || entity.LaunchExpiresAt != nil ||
			entity.SubmissionStartedAt == nil ||
			(entity.ProviderExecutionID != "" && !ValidProviderExecutionProviderID(entity.ProviderExecutionID)) ||
			entity.ObservedState != ProviderExecutionObservedSubmitting ||
			entity.SafeErrorCode != "legacy_launch_quarantined" ||
			entity.SafeErrorMessage != "provider launch requires operator recovery" {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionLaunchComplete:
		if entity.LaunchOperationHash.IsZero() ||
			!validProviderExecutionOpaque(entity.LaunchProviderOperationID, 128) ||
			entity.LaunchExpiresAt != nil ||
			entity.CheckpointEnvelope == "" {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionLaunchAborted:
		normalAbort := !entity.LaunchOperationHash.IsZero() &&
			validProviderExecutionOpaque(entity.LaunchProviderOperationID, 128)
		disposedQuarantine := entity.LaunchOperationHash.IsZero() &&
			entity.LaunchProviderOperationID == "" &&
			entity.LaunchRequestDigest.IsZero() &&
			entity.DesiredState == ProviderExecutionDesiredStop &&
			entity.CheckpointEnvelope == "" &&
			entity.SafeErrorCode == ProviderExecutionQuarantineDisposedCode &&
			entity.SafeErrorMessage == ProviderExecutionQuarantineDisposedMessage
		if (!normalAbort && !disposedQuarantine) || entity.LaunchExpiresAt != nil ||
			entity.ProviderExecutionID != "" || entity.SubmissionStartedAt != nil {
			return ErrProviderExecutionInvalid
		}
		if normalAbort && entity.ObservedState != ProviderExecutionObservedPending {
			return ErrProviderExecutionInvalid
		}
		if disposedQuarantine &&
			entity.ObservedState != ProviderExecutionObservedPending &&
			entity.ObservedState != ProviderExecutionObservedCleanupPending {
			return ErrProviderExecutionInvalid
		}
	default:
		return ErrProviderExecutionInvalid
	}
	terminal := entity.ObservedState == ProviderExecutionObservedSucceeded ||
		entity.ObservedState == ProviderExecutionObservedFailed ||
		entity.ObservedState == ProviderExecutionObservedCanceled ||
		entity.ObservedState == ProviderExecutionObservedTimedOut
	if entity.ObservedState == ProviderExecutionObservedSubmitting && entity.SubmissionStartedAt == nil {
		return ErrProviderExecutionInvalid
	}
	if (entity.ObservedState == ProviderExecutionObservedRunning || terminal) &&
		(entity.SubmissionStartedAt == nil || entity.ProviderExecutionID == "") {
		return ErrProviderExecutionInvalid
	}
	if terminal && entity.TerminalOperationHash.IsZero() {
		return ErrProviderExecutionInvalid
	}
	if (entity.ObservedState == ProviderExecutionObservedPending || entity.ObservedState == ProviderExecutionObservedSubmitting ||
		entity.ObservedState == ProviderExecutionObservedRunning) && !entity.TerminalOperationHash.IsZero() {
		return ErrProviderExecutionInvalid
	}
	if ownerPresent && !entity.ReleaseOwnerOperationHash.IsZero() {
		return ErrProviderExecutionInvalid
	}
	if entity.ProviderLeaseExpiresAt != nil && entity.ProviderExecutionID == "" {
		return ErrProviderExecutionInvalid
	}
	if entity.ObservedState == ProviderExecutionObservedCleanupComplete {
		if entity.DesiredState != ProviderExecutionDesiredStop || ownerPresent || entity.OwnerExpiresAt != nil ||
			entity.ProviderLeaseExpiresAt != nil || entity.CheckpointEnvelope != "" || entity.CheckpointWritePending ||
			entity.CleanupOperationHash.IsZero() {
			return ErrProviderExecutionInvalid
		}
	} else if !entity.CleanupOperationHash.IsZero() {
		return ErrProviderExecutionInvalid
	}
	if entity.ObservedState == ProviderExecutionObservedCleanupPending && entity.DesiredState != ProviderExecutionDesiredStop {
		return ErrProviderExecutionInvalid
	}
	return nil
}

func HydrateProviderExecution(entity *ProviderExecution) (*ProviderExecution, error) {
	if err := ValidateProviderExecutionEntity(entity); err != nil {
		return nil, err
	}
	copy := *entity
	copy.SubmissionStartedAt = cloneProviderExecutionDomainTime(entity.SubmissionStartedAt)
	copy.LaunchExpiresAt = cloneProviderExecutionDomainTime(entity.LaunchExpiresAt)
	copy.CheckpointWriteExpiresAt = cloneProviderExecutionDomainTime(entity.CheckpointWriteExpiresAt)
	copy.ProviderLeaseExpiresAt = cloneProviderExecutionDomainTime(entity.ProviderLeaseExpiresAt)
	copy.OwnerExpiresAt = cloneProviderExecutionDomainTime(entity.OwnerExpiresAt)
	copy.BuildStartedAt = cloneProviderExecutionDomainTime(entity.BuildStartedAt)
	copy.ArtifactUpdatedAt = cloneProviderExecutionDomainTime(entity.ArtifactUpdatedAt)
	return &copy, nil
}

func validateProviderExecutionBuildState(entity *ProviderExecution) error {
	status := entity.ArtifactStatus
	if status == "" {
		status = ProviderExecutionArtifactNone
	}
	descriptor := ProviderBuildArtifactDescriptor{Kind: entity.ArtifactKind, Digest: entity.ArtifactDigest, Size: entity.ArtifactSize}
	descriptorPresent := descriptor != (ProviderBuildArtifactDescriptor{})
	operationPresent := entity.BuildOperationID != "" || !entity.BuildOperationHash.IsZero()
	timingPresent := entity.BuildStartedAt != nil || entity.ArtifactUpdatedAt != nil || entity.ArtifactVersion != 0
	errorPresent := entity.ArtifactSafeErrorCode != "" || entity.ArtifactSafeErrorMessage != ""
	validOperation := validProviderExecutionOpaque(entity.BuildOperationID, 128) && !entity.BuildOperationHash.IsZero()
	validTiming := entity.ArtifactVersion > 0 && entity.BuildStartedAt != nil && entity.ArtifactUpdatedAt != nil
	switch status {
	case ProviderExecutionArtifactNone:
		if operationPresent || descriptorPresent || timingPresent || errorPresent {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionArtifactBeginPending, ProviderExecutionArtifactBuilding:
		if !validOperation || !validTiming || descriptorPresent || entity.ArtifactObjectKey != "" || errorPresent {
			return ErrProviderExecutionInvalid
		}
	case ProviderExecutionArtifactDescriptorReady, ProviderExecutionArtifactPublishing:
		if !validOperation || !validTiming || entity.ArtifactObjectKey != "" || errorPresent {
			return ErrProviderExecutionInvalid
		}
		if _, err := NormalizeProviderBuildArtifactDescriptor(descriptor); err != nil {
			return err
		}
	case ProviderExecutionArtifactReady:
		if !validOperation || !validTiming || entity.ArtifactObjectKey == "" || errorPresent {
			return ErrProviderExecutionInvalid
		}
		if _, err := NormalizeProviderBuildArtifactDescriptor(descriptor); err != nil {
			return err
		}
	case ProviderExecutionArtifactFailed:
		if !validOperation || !validTiming || entity.ArtifactObjectKey != "" ||
			entity.ArtifactSafeErrorCode == "" || !ValidProviderExecutionSafeError(entity.ArtifactSafeErrorCode, entity.ArtifactSafeErrorMessage) {
			return ErrProviderExecutionInvalid
		}
		if descriptorPresent {
			if _, err := NormalizeProviderBuildArtifactDescriptor(descriptor); err != nil {
				return err
			}
		}
	default:
		return ErrProviderExecutionInvalid
	}
	return nil
}

func cloneProviderExecutionDomainTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

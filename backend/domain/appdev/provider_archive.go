// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

var (
	ErrProjectArchiveInvalid     = errors.New("appdev project archive intent is invalid")
	ErrProjectArchiveConflict    = errors.New("appdev project archive intent conflicts with current state")
	ErrProjectArchiveNotFound    = errors.New("appdev project archive intent was not found")
	ErrProjectArchiveUnavailable = errors.New("appdev project archive intent is unavailable")
	ErrProjectArchiveSecret      = errors.New("appdev project archive internals are not serializable")
)

type ProjectArchiveState string

const (
	ProjectArchiveStateNone      ProjectArchiveState = "none"
	ProjectArchiveStateArchiving ProjectArchiveState = "archiving"
	ProjectArchiveStateArchived  ProjectArchiveState = "archived"
)

func ValidProjectArchiveState(state ProjectArchiveState) bool {
	switch state {
	case ProjectArchiveStateNone, ProjectArchiveStateArchiving, ProjectArchiveStateArchived:
		return true
	default:
		return false
	}
}

type ProjectArchiveOperationHash [sha256.Size]byte

func (hash ProjectArchiveOperationHash) IsZero() bool {
	var zero [sha256.Size]byte
	return subtle.ConstantTimeCompare(hash[:], zero[:]) == 1
}

func (hash ProjectArchiveOperationHash) EqualBytes(value []byte) bool {
	var fixed [sha256.Size]byte
	copy(fixed[:], value)
	return subtle.ConstantTimeEq(int32(len(value)), sha256.Size)&
		subtle.ConstantTimeCompare(hash[:], fixed[:]) == 1
}

func (hash ProjectArchiveOperationHash) Bytes() []byte {
	return append([]byte(nil), hash[:]...)
}

func (ProjectArchiveOperationHash) String() string { return "ProjectArchiveOperationHash{<redacted>}" }
func (ProjectArchiveOperationHash) GoString() string {
	return "ProjectArchiveOperationHash{<redacted>}"
}
func (ProjectArchiveOperationHash) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProjectArchiveOperationHash{<redacted>}")
}
func (ProjectArchiveOperationHash) MarshalJSON() ([]byte, error) {
	return nil, ErrProjectArchiveSecret
}

func ProjectArchiveOperationHashFromBytes(value []byte) (ProjectArchiveOperationHash, bool) {
	var result ProjectArchiveOperationHash
	if len(value) != sha256.Size {
		return result, false
	}
	copy(result[:], value)
	return result, true
}

type ProjectArchiveIntentIdentity struct {
	SpaceID           string
	ProjectID         string
	SourceVersion     int64
	RuntimeGeneration uint64
	IntentVersion     uint64
	OperationID       string
	OperationHash     ProjectArchiveOperationHash
}

func NewProjectArchiveIntentIdentity(
	spaceID string,
	projectID string,
	sourceVersion int64,
	runtimeGeneration uint64,
	intentVersion uint64,
) (ProjectArchiveIntentIdentity, error) {
	if !ValidProviderExecutionSpaceID(spaceID) ||
		!ValidProviderExecutionProjectID(projectID) ||
		sourceVersion <= 0 ||
		intentVersion == 0 {
		return ProjectArchiveIntentIdentity{}, ErrProjectArchiveInvalid
	}
	hasher := sha256.New()
	for _, value := range []string{
		"appdev_project_archive_intent:v1",
		spaceID,
		projectID,
		strconv.FormatInt(sourceVersion, 10),
		strconv.FormatUint(runtimeGeneration, 10),
		strconv.FormatUint(intentVersion, 10),
	} {
		_, _ = hasher.Write([]byte(value))
		_, _ = hasher.Write([]byte{0})
	}
	var operationHash ProjectArchiveOperationHash
	copy(operationHash[:], hasher.Sum(nil))
	return ProjectArchiveIntentIdentity{
		SpaceID: spaceID, ProjectID: projectID, SourceVersion: sourceVersion,
		RuntimeGeneration: runtimeGeneration, IntentVersion: intentVersion,
		OperationID:   "archive-" + hex.EncodeToString(operationHash[:]),
		OperationHash: operationHash,
	}, nil
}

func (identity ProjectArchiveIntentIdentity) Validate() error {
	expected, err := NewProjectArchiveIntentIdentity(
		identity.SpaceID,
		identity.ProjectID,
		identity.SourceVersion,
		identity.RuntimeGeneration,
		identity.IntentVersion,
	)
	if err != nil ||
		identity.OperationID != expected.OperationID ||
		subtle.ConstantTimeCompare(identity.OperationHash[:], expected.OperationHash[:]) != 1 {
		return ErrProjectArchiveInvalid
	}
	return nil
}

type ProjectArchiveIntent struct {
	ProjectArchiveIntentIdentity
	State       ProjectArchiveState
	StartedAt   time.Time
	CompletedAt time.Time
}

func (intent ProjectArchiveIntent) Validate() error {
	if intent.ProjectArchiveIntentIdentity.Validate() != nil ||
		!ValidProjectArchiveState(intent.State) ||
		intent.State == ProjectArchiveStateNone ||
		intent.StartedAt.IsZero() ||
		(intent.State == ProjectArchiveStateArchived && intent.CompletedAt.IsZero()) ||
		(intent.State == ProjectArchiveStateArchiving && !intent.CompletedAt.IsZero()) {
		return ErrProjectArchiveInvalid
	}
	return nil
}

func (intent ProjectArchiveIntent) String() string {
	return fmt.Sprintf(
		"ProjectArchiveIntent{state:%q,intentVersion:%d,sourceVersion:%d,runtimeGeneration:%d [REDACTED capability]}",
		intent.State,
		intent.IntentVersion,
		intent.SourceVersion,
		intent.RuntimeGeneration,
	)
}

func (intent ProjectArchiveIntent) GoString() string { return intent.String() }
func (intent ProjectArchiveIntent) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, intent.String())
}
func (ProjectArchiveIntent) MarshalJSON() ([]byte, error) {
	return nil, ErrProjectArchiveSecret
}

type LoadProjectArchiveInput struct {
	SpaceID   string
	ProjectID string
}

type ReserveProjectArchiveInput struct {
	SpaceID               string
	ProjectID             string
	ExpectedSourceVersion int64
	RuntimeGeneration     uint64
}

type CompleteProjectArchiveInput struct {
	Intent ProjectArchiveIntent
}

func ValidateLoadProjectArchiveInput(input LoadProjectArchiveInput) error {
	if !ValidProviderExecutionSpaceID(input.SpaceID) ||
		!ValidProviderExecutionProjectID(input.ProjectID) {
		return ErrProjectArchiveInvalid
	}
	return nil
}

func ValidateReserveProjectArchiveInput(input ReserveProjectArchiveInput) error {
	if ValidateLoadProjectArchiveInput(LoadProjectArchiveInput{
		SpaceID:   input.SpaceID,
		ProjectID: input.ProjectID,
	}) != nil || input.ExpectedSourceVersion <= 0 {
		return ErrProjectArchiveInvalid
	}
	return nil
}

func ValidateCompleteProjectArchiveInput(input CompleteProjectArchiveInput) error {
	if input.Intent.Validate() != nil ||
		input.Intent.State != ProjectArchiveStateArchiving {
		return ErrProjectArchiveInvalid
	}
	return nil
}

type ProjectArchiveRepository interface {
	LoadProjectArchive(context.Context, LoadProjectArchiveInput) (*ProjectArchiveIntent, error)
	ReserveProjectArchive(context.Context, ReserveProjectArchiveInput) (*ProjectArchiveIntent, error)
	CompleteProjectArchive(context.Context, CompleteProjectArchiveInput) (*ProjectArchiveIntent, error)
}

var (
	_ fmt.Stringer   = ProjectArchiveIntent{}
	_ fmt.GoStringer = ProjectArchiveIntent{}
	_ json.Marshaler = ProjectArchiveIntent{}
)

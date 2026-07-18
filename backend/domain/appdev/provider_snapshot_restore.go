/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package appdev

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrProviderSnapshotRestoreInvalid     = errors.New("appdev provider snapshot restore is invalid")
	ErrProviderSnapshotRestoreConflict    = errors.New("appdev provider snapshot restore conflicts with current state")
	ErrProviderSnapshotRestoreUnavailable = errors.New("appdev provider snapshot restore is unavailable")
	ErrProviderSnapshotRestoreSecret      = errors.New("appdev provider snapshot restore internals are not serializable")
)

type ProviderSnapshotRestorePhase string

const (
	ProviderSnapshotRestorePhasePending   ProviderSnapshotRestorePhase = "pending"
	ProviderSnapshotRestorePhaseRestored  ProviderSnapshotRestorePhase = "restored"
	ProviderSnapshotRestorePhaseStarted   ProviderSnapshotRestorePhase = "started"
	ProviderSnapshotRestorePhaseCompleted ProviderSnapshotRestorePhase = "completed"
	ProviderSnapshotRestorePhaseFailed    ProviderSnapshotRestorePhase = "failed"
)

type ProviderSnapshotRestoreOperationHash [sha256.Size]byte

type ProviderSnapshotRestoreParentOperationHash [sha256.Size]byte

func HashProviderSnapshotRestoreParentOperation(spaceID, projectID, operationID string) (ProviderSnapshotRestoreParentOperationHash, error) {
	if !ValidProviderExecutionSpaceID(spaceID) || !ValidProviderExecutionProjectID(projectID) || !ValidProviderExecutionIdempotencyKey(operationID) {
		return ProviderSnapshotRestoreParentOperationHash{}, ErrProviderSnapshotRestoreInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_snapshot_restore_parent:v1\x00"))
	_, _ = hasher.Write([]byte(spaceID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(projectID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(operationID))
	var result ProviderSnapshotRestoreParentOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

func ProviderSnapshotRestoreParentOperationHashFromBytes(value []byte) (ProviderSnapshotRestoreParentOperationHash, bool) {
	var result ProviderSnapshotRestoreParentOperationHash
	var fixed [sha256.Size]byte
	copy(fixed[:], value)
	valid := subtle.ConstantTimeEq(int32(len(value)), sha256.Size) == 1
	copy(result[:], fixed[:])
	return result, valid
}

func (hash ProviderSnapshotRestoreParentOperationHash) IsZero() bool {
	var zero [sha256.Size]byte
	return subtle.ConstantTimeCompare(hash[:], zero[:]) == 1
}

func (hash ProviderSnapshotRestoreParentOperationHash) Equal(other ProviderSnapshotRestoreParentOperationHash) bool {
	return subtle.ConstantTimeCompare(hash[:], other[:]) == 1
}

func (hash ProviderSnapshotRestoreParentOperationHash) EqualBytes(other []byte) bool {
	var fixed [sha256.Size]byte
	copy(fixed[:], other)
	return subtle.ConstantTimeEq(int32(len(other)), sha256.Size)&subtle.ConstantTimeCompare(hash[:], fixed[:]) == 1
}

func (hash ProviderSnapshotRestoreParentOperationHash) Bytes() []byte {
	return append([]byte(nil), hash[:]...)
}

func (ProviderSnapshotRestoreParentOperationHash) String() string {
	return "ProviderSnapshotRestoreParentOperationHash{<redacted>}"
}
func (ProviderSnapshotRestoreParentOperationHash) GoString() string {
	return "ProviderSnapshotRestoreParentOperationHash{<redacted>}"
}
func (ProviderSnapshotRestoreParentOperationHash) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderSnapshotRestoreParentOperationHash{<redacted>}")
}
func (ProviderSnapshotRestoreParentOperationHash) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderSnapshotRestoreSecret
}

func HashProviderSnapshotRestoreOperation(spaceID, projectID, snapshotID, operationID string) (ProviderSnapshotRestoreOperationHash, error) {
	if !ValidProviderExecutionSpaceID(spaceID) || !ValidProviderExecutionProjectID(projectID) ||
		!validProviderSnapshotRestoreID(snapshotID) || !ValidProviderExecutionIdempotencyKey(operationID) {
		return ProviderSnapshotRestoreOperationHash{}, ErrProviderSnapshotRestoreInvalid
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("appdev_provider_snapshot_restore:v1\x00"))
	_, _ = hasher.Write([]byte(spaceID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(projectID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(snapshotID))
	_, _ = hasher.Write([]byte("\x00"))
	_, _ = hasher.Write([]byte(operationID))
	var result ProviderSnapshotRestoreOperationHash
	copy(result[:], hasher.Sum(nil))
	return result, nil
}

func ProviderSnapshotRestoreOperationHashFromBytes(value []byte) (ProviderSnapshotRestoreOperationHash, bool) {
	var result ProviderSnapshotRestoreOperationHash
	var fixed [sha256.Size]byte
	copy(fixed[:], value)
	valid := subtle.ConstantTimeEq(int32(len(value)), sha256.Size) == 1
	copy(result[:], fixed[:])
	return result, valid
}

func (hash ProviderSnapshotRestoreOperationHash) IsZero() bool {
	var zero [sha256.Size]byte
	return subtle.ConstantTimeCompare(hash[:], zero[:]) == 1
}

func (hash ProviderSnapshotRestoreOperationHash) Equal(other ProviderSnapshotRestoreOperationHash) bool {
	return subtle.ConstantTimeCompare(hash[:], other[:]) == 1
}

func (hash ProviderSnapshotRestoreOperationHash) EqualBytes(other []byte) bool {
	var fixed [sha256.Size]byte
	copy(fixed[:], other)
	return subtle.ConstantTimeEq(int32(len(other)), sha256.Size)&subtle.ConstantTimeCompare(hash[:], fixed[:]) == 1
}

func (hash ProviderSnapshotRestoreOperationHash) Bytes() []byte {
	return append([]byte(nil), hash[:]...)
}

func (ProviderSnapshotRestoreOperationHash) String() string {
	return "ProviderSnapshotRestoreOperationHash{<redacted>}"
}
func (ProviderSnapshotRestoreOperationHash) GoString() string {
	return "ProviderSnapshotRestoreOperationHash{<redacted>}"
}
func (ProviderSnapshotRestoreOperationHash) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "ProviderSnapshotRestoreOperationHash{<redacted>}")
}
func (ProviderSnapshotRestoreOperationHash) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderSnapshotRestoreSecret
}

type ProviderSnapshotRestoreJournal struct {
	SpaceID             string
	ProjectID           string
	SnapshotID          string
	OperationHash       ProviderSnapshotRestoreOperationHash
	ParentOperationHash ProviderSnapshotRestoreParentOperationHash
	Phase               ProviderSnapshotRestorePhase
	RestartRequired     bool
	RuntimeGeneration   uint64
	SourceVersion       int64
	ResultSourceVersion int64
	StartedGeneration   uint64
	SafeErrorCode       string
	SafeErrorMessage    string
	UpdatedAt           time.Time
}

func (journal ProviderSnapshotRestoreJournal) String() string {
	return fmt.Sprintf("ProviderSnapshotRestoreJournal{phase:%q,restartRequired:%t,runtimeGeneration:%d [REDACTED capability]}", journal.Phase, journal.RestartRequired, journal.RuntimeGeneration)
}
func (journal ProviderSnapshotRestoreJournal) GoString() string { return journal.String() }
func (journal ProviderSnapshotRestoreJournal) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, journal.String())
}
func (ProviderSnapshotRestoreJournal) MarshalJSON() ([]byte, error) {
	return nil, ErrProviderSnapshotRestoreSecret
}

type ReserveProviderSnapshotRestoreInput struct {
	SpaceID             string
	ProjectID           string
	SnapshotID          string
	OperationHash       ProviderSnapshotRestoreOperationHash
	ParentOperationHash ProviderSnapshotRestoreParentOperationHash
	RestartRequired     bool
	RuntimeGeneration   uint64
}

type LoadProviderSnapshotRestoreInput struct {
	SpaceID             string
	ProjectID           string
	SnapshotID          string
	OperationHash       ProviderSnapshotRestoreOperationHash
	ParentOperationHash ProviderSnapshotRestoreParentOperationHash
}

type ApplyProviderSnapshotRestoreInput struct {
	SpaceID               string
	ProjectID             string
	SnapshotID            string
	OperationHash         ProviderSnapshotRestoreOperationHash
	ParentOperationHash   ProviderSnapshotRestoreParentOperationHash
	ExpectedSourceVersion int64
}

type FailProviderSnapshotRestoreInput struct {
	SpaceID               string
	ProjectID             string
	SnapshotID            string
	OperationHash         ProviderSnapshotRestoreOperationHash
	ParentOperationHash   ProviderSnapshotRestoreParentOperationHash
	ExpectedSourceVersion int64
	SafeErrorCode         string
	SafeErrorMessage      string
}

type MarkProviderSnapshotRestoreStartedInput struct {
	SpaceID               string
	ProjectID             string
	SnapshotID            string
	OperationHash         ProviderSnapshotRestoreOperationHash
	ParentOperationHash   ProviderSnapshotRestoreParentOperationHash
	ExpectedSourceVersion int64
	StartedGeneration     uint64
}

type CompleteProviderSnapshotRestoreInput struct {
	SpaceID               string
	ProjectID             string
	SnapshotID            string
	OperationHash         ProviderSnapshotRestoreOperationHash
	ParentOperationHash   ProviderSnapshotRestoreParentOperationHash
	ExpectedSourceVersion int64
	StartedGeneration     uint64
}

type ProviderSnapshotRestoreRepository interface {
	LoadProviderSnapshotRestore(context.Context, LoadProviderSnapshotRestoreInput) (*ProviderSnapshotRestoreJournal, error)
	ReserveProviderSnapshotRestore(context.Context, ReserveProviderSnapshotRestoreInput) (*ProviderSnapshotRestoreJournal, error)
	ApplyProviderSnapshotRestore(context.Context, ApplyProviderSnapshotRestoreInput) (*ProviderSnapshotRestoreJournal, error)
	FailProviderSnapshotRestore(context.Context, FailProviderSnapshotRestoreInput) (*ProviderSnapshotRestoreJournal, error)
	MarkProviderSnapshotRestoreStarted(context.Context, MarkProviderSnapshotRestoreStartedInput) (*ProviderSnapshotRestoreJournal, error)
	CompleteProviderSnapshotRestore(context.Context, CompleteProviderSnapshotRestoreInput) (*ProviderSnapshotRestoreJournal, error)
}

func ValidateReserveProviderSnapshotRestoreInput(input ReserveProviderSnapshotRestoreInput) error {
	if !validProviderSnapshotRestoreIdentity(input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash) ||
		(input.RestartRequired && input.RuntimeGeneration == 0) || (!input.RestartRequired && input.RuntimeGeneration != 0) {
		return ErrProviderSnapshotRestoreInvalid
	}
	return nil
}

func ValidateApplyProviderSnapshotRestoreInput(input ApplyProviderSnapshotRestoreInput) error {
	if !validProviderSnapshotRestoreIdentity(input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash) || input.ExpectedSourceVersion <= 0 {
		return ErrProviderSnapshotRestoreInvalid
	}
	return nil
}

func ValidateFailProviderSnapshotRestoreInput(input FailProviderSnapshotRestoreInput) error {
	if !validProviderSnapshotRestoreIdentity(input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash) ||
		input.ExpectedSourceVersion <= 0 || !ValidProviderExecutionSafeError(input.SafeErrorCode, input.SafeErrorMessage) || input.SafeErrorCode == "" {
		return ErrProviderSnapshotRestoreInvalid
	}
	return nil
}

func ValidateMarkProviderSnapshotRestoreStartedInput(input MarkProviderSnapshotRestoreStartedInput) error {
	if !validProviderSnapshotRestoreIdentity(input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash) || input.ExpectedSourceVersion <= 0 || input.StartedGeneration == 0 {
		return ErrProviderSnapshotRestoreInvalid
	}
	return nil
}

func ValidateCompleteProviderSnapshotRestoreInput(input CompleteProviderSnapshotRestoreInput) error {
	if !validProviderSnapshotRestoreIdentity(input.SpaceID, input.ProjectID, input.SnapshotID, input.OperationHash, input.ParentOperationHash) || input.ExpectedSourceVersion <= 0 {
		return ErrProviderSnapshotRestoreInvalid
	}
	return nil
}

func (journal *ProviderSnapshotRestoreJournal) Validate() error {
	return ValidateProviderSnapshotRestoreJournal(journal)
}

func ValidateProviderSnapshotRestoreJournal(journal *ProviderSnapshotRestoreJournal) error {
	if journal == nil || !validProviderSnapshotRestoreIdentity(journal.SpaceID, journal.ProjectID, journal.SnapshotID, journal.OperationHash, journal.ParentOperationHash) ||
		journal.SourceVersion <= 0 || journal.UpdatedAt.IsZero() ||
		(journal.RestartRequired && journal.RuntimeGeneration == 0) || (!journal.RestartRequired && journal.RuntimeGeneration != 0) {
		return ErrProviderSnapshotRestoreInvalid
	}
	switch journal.Phase {
	case ProviderSnapshotRestorePhasePending:
		if journal.ResultSourceVersion != 0 || journal.StartedGeneration != 0 {
			return ErrProviderSnapshotRestoreInvalid
		}
	case ProviderSnapshotRestorePhaseRestored:
		if journal.ResultSourceVersion <= journal.SourceVersion || journal.StartedGeneration != 0 {
			return ErrProviderSnapshotRestoreInvalid
		}
	case ProviderSnapshotRestorePhaseStarted:
		if !journal.RestartRequired || journal.ResultSourceVersion <= journal.SourceVersion || journal.StartedGeneration == 0 {
			return ErrProviderSnapshotRestoreInvalid
		}
	case ProviderSnapshotRestorePhaseCompleted:
		if journal.ResultSourceVersion <= journal.SourceVersion || (journal.RestartRequired && journal.StartedGeneration == 0) || (!journal.RestartRequired && journal.StartedGeneration != 0) {
			return ErrProviderSnapshotRestoreInvalid
		}
	case ProviderSnapshotRestorePhaseFailed:
		if journal.ResultSourceVersion != 0 || journal.StartedGeneration != 0 || journal.SafeErrorCode == "" ||
			!ValidProviderExecutionSafeError(journal.SafeErrorCode, journal.SafeErrorMessage) {
			return ErrProviderSnapshotRestoreInvalid
		}
	default:
		return ErrProviderSnapshotRestoreInvalid
	}
	return nil
}

func validProviderSnapshotRestoreIdentity(spaceID, projectID, snapshotID string, operationHash ProviderSnapshotRestoreOperationHash, parentOperationHash ProviderSnapshotRestoreParentOperationHash) bool {
	return ValidProviderExecutionSpaceID(spaceID) && ValidProviderExecutionProjectID(projectID) &&
		validProviderSnapshotRestoreID(snapshotID) && !operationHash.IsZero() && !parentOperationHash.IsZero()
}

func validProviderSnapshotRestoreID(value string) bool {
	return len(value) > 0 && len(value) <= 128 && value == strings.TrimSpace(value) && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t/\\")
}

var _ json.Marshaler = ProviderSnapshotRestoreOperationHash{}
var _ json.Marshaler = ProviderSnapshotRestoreParentOperationHash{}
var _ json.Marshaler = ProviderSnapshotRestoreJournal{}

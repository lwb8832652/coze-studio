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

package entity

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	runIdempotencyMetadataKey = "_idempotency"
	runMessageMetadataKey     = "_message"
)

var ErrRunIdempotencyConflict = errors.New("run idempotency key conflict")

type runIdempotencyContract struct {
	Operation   string `json:"operation"`
	Fingerprint string `json:"fingerprint"`
}

type runMessageReference struct {
	MessageID int64 `json:"message_id"`
}

// MergeRunIdempotencyContract adds a server-owned replay contract without
// decoding and re-encoding unrelated metadata values. Empty values preserve
// legacy callers exactly as they are.
func MergeRunIdempotencyContract(metadata, operation, fingerprint string) (string, error) {
	operation, fingerprint, err := normalizeRunIdempotencyContract(operation, fingerprint)
	if err != nil {
		return "", err
	}
	if operation == "" {
		return metadata, nil
	}
	payload, err := runMetadataObject(metadata)
	if err != nil {
		return "", err
	}
	contract, err := json.Marshal(runIdempotencyContract{
		Operation: operation, Fingerprint: fingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("marshal run idempotency contract: %w", err)
	}
	payload[runIdempotencyMetadataKey] = contract
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal run metadata: %w", err)
	}
	return string(encoded), nil
}

// ValidateRunIdempotencyValues verifies an existing persisted Run against the
// operation and payload fingerprint supplied by a replaying caller.
func ValidateRunIdempotencyValues(metadata, operation, fingerprint string) error {
	operation, fingerprint, err := normalizeRunIdempotencyContract(operation, fingerprint)
	if err != nil {
		return err
	}
	if operation == "" {
		return nil
	}
	payload, err := runMetadataObject(metadata)
	if err != nil {
		return fmt.Errorf("%w: persisted metadata is invalid", ErrRunIdempotencyConflict)
	}
	raw, exists := payload[runIdempotencyMetadataKey]
	if !exists {
		return fmt.Errorf("%w: persisted fingerprint is missing", ErrRunIdempotencyConflict)
	}
	var persisted runIdempotencyContract
	if err := json.Unmarshal(raw, &persisted); err != nil {
		return fmt.Errorf("%w: persisted fingerprint is invalid", ErrRunIdempotencyConflict)
	}
	if persisted.Operation != operation || persisted.Fingerprint != fingerprint {
		return fmt.Errorf("%w: operation or payload differs", ErrRunIdempotencyConflict)
	}
	return nil
}

// ValidateRunIdempotencyReplay applies fingerprint comparison only when the
// incoming Run carries the new replay contract, preserving legacy semantics for
// existing internal callers that do not opt in.
func ValidateRunIdempotencyReplay(existingMetadata, requestedMetadata string) error {
	payload, err := runMetadataObject(requestedMetadata)
	if err != nil {
		return err
	}
	raw, exists := payload[runIdempotencyMetadataKey]
	if !exists {
		return nil
	}
	var requested runIdempotencyContract
	if err := json.Unmarshal(raw, &requested); err != nil {
		return fmt.Errorf("run idempotency request metadata is invalid: %w", err)
	}
	return ValidateRunIdempotencyValues(
		existingMetadata,
		requested.Operation,
		requested.Fingerprint,
	)
}

// MergeRunMessageReference stores the authoritative Message relation allocated
// in the same Run bundle transaction. Callers cannot choose this identifier.
func MergeRunMessageReference(metadata string, messageID int64) (string, error) {
	if messageID <= 0 {
		return "", fmt.Errorf("run message reference is invalid")
	}
	payload, err := runMetadataObject(metadata)
	if err != nil {
		return "", err
	}
	reference, err := json.Marshal(runMessageReference{MessageID: messageID})
	if err != nil {
		return "", fmt.Errorf("marshal run message reference: %w", err)
	}
	payload[runMessageMetadataKey] = reference
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal run metadata: %w", err)
	}
	return string(encoded), nil
}

func normalizeRunIdempotencyContract(operation, fingerprint string) (string, string, error) {
	operation = strings.TrimSpace(operation)
	fingerprint = strings.TrimSpace(fingerprint)
	if operation == "" && fingerprint == "" {
		return "", "", nil
	}
	if operation == "" || len(operation) > 64 {
		return "", "", fmt.Errorf("run idempotency operation is invalid")
	}
	for _, r := range operation {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return "", "", fmt.Errorf("run idempotency operation is invalid")
		}
	}
	if len(fingerprint) != 64 || strings.ToLower(fingerprint) != fingerprint {
		return "", "", fmt.Errorf("run idempotency fingerprint is invalid")
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return "", "", fmt.Errorf("run idempotency fingerprint is invalid")
	}
	return operation, fingerprint, nil
}

func runMetadataObject(metadata string) (map[string]json.RawMessage, error) {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" {
		metadata = `{}`
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metadata), &payload); err != nil || payload == nil {
		if err == nil {
			err = fmt.Errorf("metadata must be an object")
		}
		return nil, fmt.Errorf("decode run metadata: %w", err)
	}
	return payload, nil
}

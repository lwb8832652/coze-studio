// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestExecutionCheckpointProtectorAADTamperAndRotation(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x11}, 32)
	newKey := bytes.Repeat([]byte{0x22}, 32)
	oldRing := checkpointKeyRing(t, map[string][]byte{"old": oldKey}, "old")
	oldProtector, err := NewExecutionCheckpointProtector(oldRing)
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("appdev-provider-execution-row-a")
	plaintext := []byte("checkpoint-plaintext-secret")
	envelope, err := oldProtector.Seal(context.Background(), aad, plaintext)
	if err != nil || !strings.HasPrefix(envelope, "ecp1:old:") || strings.Contains(envelope, string(plaintext)) {
		t.Fatalf("old Seal() = %q, %v", envelope, err)
	}

	rotatedRing := checkpointKeyRing(t, map[string][]byte{"old": oldKey, "new": newKey}, "new")
	rotated, err := NewExecutionCheckpointProtector(rotatedRing)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := rotated.Open(context.Background(), aad, envelope)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("rotated Open() = %q, %v", opened, err)
	}
	if _, err := rotated.Open(context.Background(), []byte("row-b"), envelope); !errors.Is(err, ErrExecutionCheckpointProtection) {
		t.Fatalf("AAD swap error = %v", err)
	}
	newEnvelope, err := rotated.Seal(context.Background(), aad, plaintext)
	if err != nil || !strings.HasPrefix(newEnvelope, "ecp1:new:") {
		t.Fatalf("primary key write = %q, %v", newEnvelope, err)
	}
}

func TestExecutionCheckpointProtectorRejectsTamperTruncateAndUnknownVersionSafely(t *testing.T) {
	protector, err := NewExecutionCheckpointProtector(checkpointKeyRing(t, map[string][]byte{"primary": bytes.Repeat([]byte{0x33}, 32)}, "primary"))
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("bound-row")
	envelope, err := protector.Seal(context.Background(), aad, []byte("must-not-leak-checkpoint"))
	if err != nil {
		t.Fatal(err)
	}
	replacement := "A"
	if strings.HasSuffix(envelope, replacement) {
		replacement = "B"
	}
	tampered := envelope[:len(envelope)-1] + replacement
	cases := []string{"", "ecp9:primary:AA:AA", envelope[:len(envelope)/2], tampered, strings.Repeat("x", MaxExecutionCheckpointEnvelopeBytes+1)}
	for _, candidate := range cases {
		_, err := protector.Open(context.Background(), aad, candidate)
		if !errors.Is(err, ErrExecutionCheckpointProtection) || strings.Contains(err.Error(), "primary") || strings.Contains(err.Error(), "must-not-leak") {
			t.Fatalf("unsafe protector error for %q: %v", candidate, err)
		}
	}
	for _, formatted := range []string{fmt.Sprintf("%v", protector), fmt.Sprintf("%+v", protector), fmt.Sprintf("%#v", protector), protector.GoString()} {
		if strings.Contains(formatted, "primary") || !strings.Contains(formatted, "REDACTED") {
			t.Fatalf("protector formatting leaked key metadata: %q", formatted)
		}
	}
}

func checkpointKeyRing(t *testing.T, keys map[string][]byte, active string) *SandboxKeyRing {
	t.Helper()
	encoded := make(map[string]string, len(keys))
	for keyID, key := range keys {
		encoded[keyID] = base64.StdEncoding.EncodeToString(key)
	}
	payload, err := json.Marshal(encoded)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := ParseSandboxKeyRing(string(payload), active)
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

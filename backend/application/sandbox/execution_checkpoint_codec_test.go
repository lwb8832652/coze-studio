// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type checkpointBindingProtector struct{}

func (checkpointBindingProtector) Seal(_ context.Context, aad, plaintext []byte) (string, error) {
	digest := sha256.Sum256(aad)
	payload := append(digest[:], plaintext...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (checkpointBindingProtector) Open(_ context.Context, aad []byte, envelope string) ([]byte, error) {
	payload, err := base64.RawURLEncoding.Strict().DecodeString(envelope)
	if err != nil || len(payload) < sha256.Size {
		return nil, errors.New("protector rejected envelope")
	}
	digest := sha256.Sum256(aad)
	if !bytes.Equal(payload[:sha256.Size], digest[:]) {
		return nil, errors.New("protector rejected aad")
	}
	return append([]byte(nil), payload[sha256.Size:]...), nil
}

func TestExecutionCheckpointCodecRoundTripAndAADBinding(t *testing.T) {
	codec, err := NewExecutionCheckpointCodec(checkpointBindingProtector{})
	if err != nil {
		t.Fatalf("NewExecutionCheckpointCodec() error = %v", err)
	}
	binding := ExecutionCheckpointBinding{
		SpaceID: "101", ProjectID: "project_a", Generation: 7,
		ProviderKey: "provider-a", Scope: domainsandbox.ScopeAppDev,
	}
	checkpoint := ExecutionCheckpoint{
		providerKey: "provider-a", scope: domainsandbox.ScopeAppDev,
		leaseToken: "lease-token-0123456789", leaseFence: "lease-fence-0123456789",
		leaseExpiryMilli: time.Now().Add(time.Minute).UnixMilli(), executionID: "execution-123",
		queueStatusFeature: true, signedExecutionContextFeature: true, admissionLimit: 23,
	}
	envelope, err := codec.Seal(context.Background(), binding, checkpoint)
	if err != nil || envelope == "" {
		t.Fatalf("Seal() = %q, %v", envelope, err)
	}
	opened, err := codec.Open(context.Background(), binding, envelope)
	if err != nil || opened.providerKey != checkpoint.providerKey || opened.executionID != checkpoint.executionID ||
		opened.leaseToken != checkpoint.leaseToken || opened.leaseFence != checkpoint.leaseFence ||
		opened.queueStatusFeature != checkpoint.queueStatusFeature ||
		opened.signedExecutionContextFeature != checkpoint.signedExecutionContextFeature ||
		opened.admissionLimit != checkpoint.admissionLimit {
		t.Fatalf("Open() = %#v, %v", opened, err)
	}
	swapped := binding
	swapped.ProjectID = "project_b"
	if _, err := codec.Open(context.Background(), swapped, envelope); !errors.Is(err, ErrExecutionCheckpointCodec) {
		t.Fatalf("AAD swap error = %v", err)
	}
}

func TestExecutionCheckpointCodecRejectsMalformedPlaintextSafely(t *testing.T) {
	protector := &checkpointMalformedProtector{plaintext: []byte("truncated-secret-token")}
	codec, err := NewExecutionCheckpointCodec(protector)
	if err != nil {
		t.Fatal(err)
	}
	binding := ExecutionCheckpointBinding{SpaceID: "1", ProjectID: "p", Generation: 1, ProviderKey: "provider-a", Scope: domainsandbox.ScopeAppDev}
	_, err = codec.Open(context.Background(), binding, "opaque")
	if !errors.Is(err, ErrExecutionCheckpointCodec) || bytes.Contains([]byte(err.Error()), protector.plaintext) {
		t.Fatalf("malformed checkpoint error = %v", err)
	}
}

type checkpointMalformedProtector struct{ plaintext []byte }

func (*checkpointMalformedProtector) Seal(context.Context, []byte, []byte) (string, error) {
	return "opaque", nil
}
func (p *checkpointMalformedProtector) Open(context.Context, []byte, string) ([]byte, error) {
	return append([]byte(nil), p.plaintext...), nil
}

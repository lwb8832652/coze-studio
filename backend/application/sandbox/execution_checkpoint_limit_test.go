// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
)

func TestExecutionCheckpointCodecRejectsSharedEnvelopeLimitBeforeOpen(t *testing.T) {
	protector := &checkpointMalformedProtector{plaintext: []byte("unused")}
	codec, err := NewExecutionCheckpointCodec(protector)
	if err != nil {
		t.Fatal(err)
	}
	binding := ExecutionCheckpointBinding{SpaceID: "1", ProjectID: "p", Generation: 1, ProviderKey: "provider-a", Scope: domainsandbox.ScopeAppDev}
	_, err = codec.Open(context.Background(), binding, strings.Repeat("x", sandboxcontract.MaxExecutionCheckpointEnvelopeBytes+1))
	if !errors.Is(err, ErrExecutionCheckpointCodec) {
		t.Fatalf("over-limit envelope error = %v", err)
	}
}

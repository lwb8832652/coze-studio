// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type providerQuarantineControlStub struct{}

func (providerQuarantineControlStub) Dispose(
	context.Context,
	ProviderQuarantineDispositionActor,
	ProviderQuarantineDispositionRequest,
) (*ProviderQuarantineDispositionProjection, error) {
	return &ProviderQuarantineDispositionProjection{}, nil
}

func TestProviderQuarantineDispositionRegistryIsOwnerChecked(t *testing.T) {
	require.Nil(t, CurrentProviderQuarantineDispositionControl())
	first, err := PublishProviderQuarantineDispositionControl(providerQuarantineControlStub{})
	require.NoError(t, err)
	second, err := PublishProviderQuarantineDispositionControl(providerQuarantineControlStub{})
	require.NoError(t, err)
	t.Cleanup(func() { UnpublishProviderQuarantineDispositionControl(second) })

	require.False(t, UnpublishProviderQuarantineDispositionControl(first))
	require.NotNil(t, CurrentProviderQuarantineDispositionControl())
	require.True(t, UnpublishProviderQuarantineDispositionControl(second))
	require.Nil(t, CurrentProviderQuarantineDispositionControl())
}

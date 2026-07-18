// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"
	"sync/atomic"
)

var ErrProviderQuarantineDispositionWiringInvalid = errors.New("provider quarantine disposition wiring is invalid")

type ProviderQuarantineDispositionControl interface {
	Dispose(
		context.Context,
		ProviderQuarantineDispositionActor,
		ProviderQuarantineDispositionRequest,
	) (*ProviderQuarantineDispositionProjection, error)
}

type providerQuarantineDispositionRegistryEntry struct {
	control    ProviderQuarantineDispositionControl
	generation uint64
}

type ProviderQuarantineDispositionPublication struct {
	entry *providerQuarantineDispositionRegistryEntry
}

func (publication ProviderQuarantineDispositionPublication) IsCurrent() bool {
	return publication.entry != nil &&
		providerQuarantineDispositionRegistry.Load() == publication.entry
}

var (
	providerQuarantineDispositionRegistry   atomic.Pointer[providerQuarantineDispositionRegistryEntry]
	providerQuarantineDispositionGeneration atomic.Uint64
)

func PublishProviderQuarantineDispositionControl(
	control ProviderQuarantineDispositionControl,
) (ProviderQuarantineDispositionPublication, error) {
	if control == nil {
		return ProviderQuarantineDispositionPublication{}, ErrProviderQuarantineDispositionWiringInvalid
	}
	entry := &providerQuarantineDispositionRegistryEntry{
		control: control, generation: providerQuarantineDispositionGeneration.Add(1),
	}
	providerQuarantineDispositionRegistry.Store(entry)
	return ProviderQuarantineDispositionPublication{entry: entry}, nil
}

func UnpublishProviderQuarantineDispositionControl(
	publication ProviderQuarantineDispositionPublication,
) bool {
	if publication.entry == nil {
		return false
	}
	return providerQuarantineDispositionRegistry.CompareAndSwap(publication.entry, nil)
}

func CurrentProviderQuarantineDispositionControl() ProviderQuarantineDispositionControl {
	current := providerQuarantineDispositionRegistry.Load()
	if current == nil {
		return nil
	}
	return current.control
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"errors"
	"testing"
)

func TestResourceWatermarkFailsClosedAndRequiresConfiguredReserve(t *testing.T) {
	for name, sample := range map[string]struct {
		available int
		err       error
		allowed   bool
	}{
		"above reserve": {available: 1537, allowed: true},
		"at reserve":    {available: 1536, allowed: true},
		"below reserve": {available: 1535, allowed: false},
		"unavailable":   {err: errors.New("unavailable"), allowed: false},
	} {
		t.Run(name, func(t *testing.T) {
			allowed := ResourceWatermark{Sampler: resourceSamplerFunc(func(context.Context) (int, error) { return sample.available, sample.err }), ReserveMB: 1536}.AllowsDispatch(context.Background())
			if allowed != sample.allowed {
				t.Fatalf("AllowsDispatch() = %t, want %t", allowed, sample.allowed)
			}
		})
	}
}

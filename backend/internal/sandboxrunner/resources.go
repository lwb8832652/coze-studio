// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import "context"

// ResourceSampler reports host memory available to new sandbox work. It must
// not expose raw host details beyond the scheduler process.
type ResourceSampler interface {
	AvailableMemoryMB(context.Context) (int, error)
}

type resourceSamplerFunc func(context.Context) (int, error)

func (function resourceSamplerFunc) AvailableMemoryMB(ctx context.Context) (int, error) {
	return function(ctx)
}

// ResourceWatermark gates only new dispatches. Existing cgroup-constrained
// work is left to its terminal path rather than triggering a host-wide OOM.
type ResourceWatermark struct {
	Sampler   ResourceSampler
	ReserveMB int
}

func (watermark ResourceWatermark) AllowsDispatch(ctx context.Context) bool {
	if watermark.Sampler == nil || watermark.ReserveMB < 1 || ctx == nil {
		return false
	}
	available, err := watermark.Sampler.AvailableMemoryMB(ctx)
	return err == nil && available >= watermark.ReserveMB
}

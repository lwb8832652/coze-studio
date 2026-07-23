// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"
)

const defaultMaintenanceInterval = time.Minute

type MaintenanceWorker struct {
	service  *Service
	interval time.Duration
}

func StartMaintenanceWorkerFromEnv(ctx context.Context, service *Service) *MaintenanceWorker {
	enabled, _ := strconv.ParseBool(os.Getenv("BILLING_MAINTENANCE_WORKER_ENABLED"))
	if !enabled || service == nil {
		return nil
	}
	interval := defaultMaintenanceInterval
	if configured := os.Getenv("BILLING_MAINTENANCE_INTERVAL"); configured != "" {
		if parsed, err := time.ParseDuration(configured); err == nil && parsed >= time.Second {
			interval = parsed
		}
	}
	worker := &MaintenanceWorker{service: service, interval: interval}
	go worker.run(ctx)
	return worker
}

func (w *MaintenanceWorker) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if _, err := w.service.RunMaintenance(ctx); err != nil && ctx.Err() == nil {
			hlog.CtxErrorf(ctx, "[billing-maintenance] run failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

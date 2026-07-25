// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"testing"
	"time"
)

type blockingMaintenanceRunService struct {
	started        chan struct{}
	cancelObserved chan struct{}
	allowReturn    chan struct{}
}

func (s *blockingMaintenanceRunService) runMaintenanceAt(
	ctx context.Context,
	_ time.Time,
	_ time.Duration,
) (*MaintenanceResult, error) {
	close(s.started)
	<-ctx.Done()
	close(s.cancelObserved)
	<-s.allowReturn
	return nil, ctx.Err()
}

func TestMaintenanceWorkerShutdownCancelsAndWaits(t *testing.T) {
	service := &blockingMaintenanceRunService{
		started:        make(chan struct{}),
		cancelObserved: make(chan struct{}),
		allowReturn:    make(chan struct{}),
	}
	worker := &MaintenanceWorker{
		service: service,
		interval: time.Hour,
		now: time.Now,
	}
	worker.Start(context.Background())

	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("maintenance run did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- worker.Shutdown(ctx)
	}()
	select {
	case <-service.cancelObserved:
	case <-ctx.Done():
		t.Fatalf("maintenance run did not observe cancellation: %v", ctx.Err())
	}
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown() returned before runOnce exited: %v", err)
	default:
	}
	close(service.allowReturn)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

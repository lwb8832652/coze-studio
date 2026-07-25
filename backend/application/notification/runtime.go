// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"sync"

	"gorm.io/gorm"

	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type Runtime struct {
	runner workerRunner
	service *Service

	mu             sync.Mutex
	started        bool
	stopClaims     context.CancelFunc
	cancelInFlight context.CancelFunc
	done           chan struct{}
}

type workerRunner interface {
	Run(context.Context, context.Context)
}

func NewRuntime(
	db *gorm.DB,
	idGenerator idgen.IDGenerator,
	resolver RecipientResolver,
) (*Runtime, error) {
	if db == nil || idGenerator == nil {
		return nil, errors.New("notification runtime dependencies are required")
	}
	repository := infranotification.NewMySQLRepository(db, idGenerator)
	service := NewService(repository)
	SetDefaultService(service)
	runtime := newRuntimeWithRunner(
		NewWorker(repository, resolver, DefaultWorkerOptions()),
	)
	runtime.service = service
	return runtime, nil
}

func newRuntimeWithRunner(runner workerRunner) *Runtime {
	return &Runtime{
		runner: runner,
		done:   make(chan struct{}),
	}
}

func (r *Runtime) Service() *Service {
	if r == nil {
		return nil
	}
	return r.service
}

func (r *Runtime) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	claimLoopCtx, stopClaims := context.WithCancel(ctx)
	inFlightCtx, cancelInFlight := context.WithCancel(context.WithoutCancel(ctx))
	r.stopClaims = stopClaims
	r.cancelInFlight = cancelInFlight
	r.started = true
	go func() {
		defer close(r.done)
		r.runner.Run(claimLoopCtx, inFlightCtx)
	}()
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.started {
		r.mu.Unlock()
		return nil
	}
	stopClaims := r.stopClaims
	cancelInFlight := r.cancelInFlight
	done := r.done
	r.mu.Unlock()

	if stopClaims != nil {
		stopClaims()
	}
	select {
	case <-done:
		if cancelInFlight != nil {
			cancelInFlight()
		}
		return nil
	case <-ctx.Done():
		if cancelInFlight != nil {
			cancelInFlight()
		}
		return ctx.Err()
	}
}

func (r *Runtime) ShutdownName() string {
	return "reliable-notification-runtime"
}

/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
)

const (
	defaultADKCancelTimeout    = 10 * time.Second
	defaultADKPendingCancelTTL = time.Minute
)

var ErrADKRunNotActive = errors.New("eino adk run is not active")

type adkCancelWaiter interface {
	Wait() error
}

type adkCancelRequest struct {
	mode      adk.CancelMode
	recursive bool
}

type adkCancelInvocation func(adkCancelRequest) (adkCancelWaiter, bool)

type registeredADKCancel struct {
	generation uint64
	invoke     adkCancelInvocation
}

type pendingADKCancel struct {
	generation uint64
	request    adkCancelRequest
	timer      *time.Timer
}

type ADKCancelRegistry struct {
	mu         sync.Mutex
	generation uint64
	handles    map[int64]registeredADKCancel
	pending    map[int64]pendingADKCancel
	timeout    time.Duration
}

func NewADKCancelRegistry() *ADKCancelRegistry {
	return &ADKCancelRegistry{
		handles: make(map[int64]registeredADKCancel),
		pending: make(map[int64]pendingADKCancel),
		timeout: defaultADKCancelTimeout,
	}
}

func (r *ADKCancelRegistry) Register(runID int64, cancel adk.AgentCancelFunc) func() {
	return r.RegisterWithExecutionCancel(runID, cancel, nil)
}

func (r *ADKCancelRegistry) RegisterWithExecutionCancel(
	runID int64,
	cancel adk.AgentCancelFunc,
	executionCancel context.CancelFunc,
) func() {
	return r.register(runID, func(request adkCancelRequest) (adkCancelWaiter, bool) {
		options := []adk.AgentCancelOption{
			adk.WithAgentCancelMode(request.mode),
			adk.WithAgentCancelTimeout(r.cancelTimeout()),
		}
		if request.recursive {
			options = append(options, adk.WithRecursive())
		}

		waiter, contributed := cancel(options...)
		if request.mode == adk.CancelImmediate && executionCancel != nil {
			executionCancel()
		}

		return waiter, contributed
	})
}

func (r *ADKCancelRegistry) register(runID int64, invoke adkCancelInvocation) func() {
	if r == nil || runID <= 0 || invoke == nil {
		return func() {}
	}

	r.mu.Lock()
	if r.handles == nil {
		r.handles = make(map[int64]registeredADKCancel)
	}
	r.generation++
	generation := r.generation
	r.handles[runID] = registeredADKCancel{
		generation: generation,
		invoke:     invoke,
	}
	pending, hasPending := r.pending[runID]
	if hasPending {
		delete(r.pending, runID)
		if pending.timer != nil {
			pending.timer.Stop()
		}
	}
	r.mu.Unlock()

	if hasPending {
		request := pending.request
		request.mode = adk.CancelImmediate
		r.invokePending(invoke, request)
	}

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		current, exists := r.handles[runID]
		if exists && current.generation == generation {
			delete(r.handles, runID)
		}
	}
}

func (r *ADKCancelRegistry) Cancel(
	ctx context.Context,
	runID int64,
	mode adk.CancelMode,
	recursive bool,
) error {
	if r == nil {
		return ErrADKRunNotActive
	}

	r.mu.Lock()
	registered, exists := r.handles[runID]
	r.mu.Unlock()
	if !exists || registered.invoke == nil {
		return ErrADKRunNotActive
	}

	return invokeADKCancel(ctx, registered.invoke, adkCancelRequest{
		mode:      mode,
		recursive: recursive,
	})
}

// Request records the durable cancellation intent when execution registration
// has not caught up yet. Callers must only use it after the run was canceled in
// persistent state; direct best-effort cancellation should use Cancel instead.
func (r *ADKCancelRegistry) Request(
	ctx context.Context,
	runID int64,
	mode adk.CancelMode,
	recursive bool,
) error {
	if r == nil || runID <= 0 {
		return ErrADKRunNotActive
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	request := adkCancelRequest{mode: mode, recursive: recursive}
	r.mu.Lock()
	registered, exists := r.handles[runID]
	if exists && registered.invoke != nil {
		r.mu.Unlock()
		return invokeADKCancel(ctx, registered.invoke, request)
	}
	if r.pending == nil {
		r.pending = make(map[int64]pendingADKCancel)
	}
	if previous, ok := r.pending[runID]; ok && previous.timer != nil {
		previous.timer.Stop()
	}
	r.generation++
	generation := r.generation
	pending := pendingADKCancel{
		generation: generation,
		request:    request,
	}
	pending.timer = time.AfterFunc(defaultADKPendingCancelTTL, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		current, ok := r.pending[runID]
		if ok && current.generation == generation {
			delete(r.pending, runID)
		}
	})
	r.pending[runID] = pending
	r.mu.Unlock()

	return nil
}

func invokeADKCancel(
	ctx context.Context,
	invoke adkCancelInvocation,
	request adkCancelRequest,
) error {
	if invoke == nil {
		return ErrADKRunNotActive
	}

	waiter, contributed := invoke(request)
	if !contributed {
		return adk.ErrExecutionEnded
	}
	if waiter == nil {
		return fmt.Errorf("eino adk cancel returned empty handle")
	}

	result := make(chan error, 1)
	go func() {
		result <- waiter.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-result:
		return err
	}
}

func (r *ADKCancelRegistry) invokePending(invoke adkCancelInvocation, request adkCancelRequest) {
	waiter, contributed := invoke(request)
	if !contributed || waiter == nil {
		return
	}
	go func() {
		_ = waiter.Wait()
	}()
}

func (r *ADKCancelRegistry) cancelTimeout() time.Duration {
	if r == nil || r.timeout <= 0 {
		return defaultADKCancelTimeout
	}

	return r.timeout
}

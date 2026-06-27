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

const defaultADKCancelTimeout = 10 * time.Second

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

type ADKCancelRegistry struct {
	mu         sync.Mutex
	generation uint64
	handles    map[int64]registeredADKCancel
	timeout    time.Duration
}

func NewADKCancelRegistry() *ADKCancelRegistry {
	return &ADKCancelRegistry{
		handles: make(map[int64]registeredADKCancel),
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
	r.mu.Unlock()

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

	waiter, contributed := registered.invoke(adkCancelRequest{
		mode:      mode,
		recursive: recursive,
	})
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

func (r *ADKCancelRegistry) cancelTimeout() time.Duration {
	if r == nil || r.timeout <= 0 {
		return defaultADKCancelTimeout
	}

	return r.timeout
}

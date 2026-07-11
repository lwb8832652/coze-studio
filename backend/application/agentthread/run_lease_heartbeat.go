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
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultRunLeaseTTL               = 60 * time.Second
	defaultRunLeaseHeartbeatInterval = 20 * time.Second
	defaultRunLeaseRenewTimeout      = 10 * time.Second
)

type RunLeaseTicker interface {
	C() <-chan time.Time
	Stop()
}

type RunLeaseClock interface {
	Now() time.Time
	NewTicker(interval time.Duration) RunLeaseTicker
}

type systemRunLeaseClock struct{}

func (systemRunLeaseClock) Now() time.Time {
	return time.Now()
}

func (systemRunLeaseClock) NewTicker(interval time.Duration) RunLeaseTicker {
	return &systemRunLeaseTicker{ticker: time.NewTicker(interval)}
}

type systemRunLeaseTicker struct {
	ticker *time.Ticker
}

func (t *systemRunLeaseTicker) C() <-chan time.Time {
	if t == nil || t.ticker == nil {
		return nil
	}
	return t.ticker.C
}

func (t *systemRunLeaseTicker) Stop() {
	if t != nil && t.ticker != nil {
		t.ticker.Stop()
	}
}

type runLeaseHeartbeatConfig struct {
	TTL      time.Duration
	Interval time.Duration
	Clock    RunLeaseClock
}

func normalizeRunLeaseHeartbeatConfig(
	ttl time.Duration,
	interval time.Duration,
	clock RunLeaseClock,
) runLeaseHeartbeatConfig {
	if ttl <= 0 {
		ttl = defaultRunLeaseTTL
	}
	if interval <= 0 {
		interval = ttl / 3
	}
	if interval <= 0 || interval >= ttl {
		interval = defaultRunLeaseHeartbeatInterval
		if interval >= ttl {
			interval = ttl / 3
		}
	}
	if interval <= 0 {
		interval = time.Millisecond
	}
	if clock == nil {
		clock = systemRunLeaseClock{}
	}

	return runLeaseHeartbeatConfig{TTL: ttl, Interval: interval, Clock: clock}
}

type runLeaseHeartbeat struct {
	app       *ApplicationService
	run       *RunSummary
	config    runLeaseHeartbeatConfig
	ticker    RunLeaseTicker
	done      chan struct{}
	stop      context.CancelFunc
	cancelRun context.CancelCauseFunc

	stopOnce sync.Once
	mu       sync.Mutex
	stopping bool
	err      error
}

func startRunLeaseHeartbeat(
	ctx context.Context,
	app *ApplicationService,
	run *RunSummary,
	config runLeaseHeartbeatConfig,
) (context.Context, *runLeaseHeartbeat) {
	if ctx == nil {
		ctx = context.Background()
	}
	if app == nil || run == nil || run.RunID <= 0 ||
		strings.TrimSpace(run.LeaseOwner) == "" || strings.TrimSpace(run.LeaseToken) == "" ||
		run.ExecutionGeneration == 0 {
		return ctx, &runLeaseHeartbeat{}
	}

	runCtx, cancelRun := context.WithCancelCause(ctx)
	heartbeatCtx, stop := context.WithCancel(ctx)
	heartbeat := &runLeaseHeartbeat{
		app:       app,
		run:       run,
		config:    config,
		ticker:    config.Clock.NewTicker(config.Interval),
		done:      make(chan struct{}),
		stop:      stop,
		cancelRun: cancelRun,
	}
	go heartbeat.loop(heartbeatCtx)

	return runCtx, heartbeat
}

func (h *runLeaseHeartbeat) loop(ctx context.Context) {
	defer close(h.done)
	if h.ticker == nil {
		h.fail(fmt.Errorf("run lease ticker is required"))
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.ticker.C():
			renewTimeout := h.config.Interval
			if renewTimeout <= 0 || renewTimeout > defaultRunLeaseRenewTimeout {
				renewTimeout = defaultRunLeaseRenewTimeout
			}
			renewCtx, cancel := context.WithTimeout(ctx, renewTimeout)
			_, err := h.app.RenewRunLease(renewCtx, &RenewRunLeaseRequest{
				RunID:               h.run.RunID,
				LeaseOwner:          h.run.LeaseOwner,
				LeaseToken:          h.run.LeaseToken,
				ExecutionGeneration: h.run.ExecutionGeneration,
				Now:                 h.config.Clock.Now().UnixMilli(),
				LeaseTTLMillis:      h.config.TTL.Milliseconds(),
			})
			cancel()
			if err != nil {
				if h.isStopping() && ctx.Err() != nil {
					return
				}
				h.fail(fmt.Errorf("renew run %d lease: %w", h.run.RunID, err))
				return
			}
		}
	}
}

func (h *runLeaseHeartbeat) fail(err error) {
	if h == nil || err == nil {
		return
	}
	h.mu.Lock()
	if h.err == nil {
		h.err = err
	}
	cancelRun := h.cancelRun
	h.mu.Unlock()
	if cancelRun != nil {
		cancelRun(err)
	}
}

func (h *runLeaseHeartbeat) isStopping() bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopping
}

func (h *runLeaseHeartbeat) Stop() error {
	if h == nil || h.done == nil {
		return nil
	}
	h.stopOnce.Do(func() {
		h.mu.Lock()
		h.stopping = true
		h.mu.Unlock()
		if h.ticker != nil {
			h.ticker.Stop()
		}
		if h.stop != nil {
			h.stop()
		}
		<-h.done
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func (h *runLeaseHeartbeat) Close() {
	if h == nil {
		return
	}
	_ = h.Stop()
	if h.cancelRun != nil {
		h.cancelRun(context.Canceled)
	}
}

func stopRunLeaseHeartbeat(ctx context.Context, heartbeat *runLeaseHeartbeat) error {
	if heartbeat != nil {
		if err := heartbeat.Stop(); err != nil {
			return err
		}
	}
	if ctx != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
	}
	return nil
}

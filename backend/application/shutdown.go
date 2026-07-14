// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errApplicationShutdownFailed = errors.New("application shutdown failed")

const (
	defaultApplicationShutdownAttemptTimeout = 5 * time.Second
	defaultApplicationShutdownRetryBackoff   = 100 * time.Millisecond
)

type applicationShutdownHook interface {
	Shutdown(context.Context) error
}

type applicationShutdownRegistry struct {
	mu         sync.Mutex
	shutdownMu sync.Mutex
	hooks      []applicationShutdownHook
}

func newApplicationShutdownRegistry() *applicationShutdownRegistry {
	return &applicationShutdownRegistry{}
}

func (r *applicationShutdownRegistry) Register(hook applicationShutdownHook) error {
	if r == nil || hook == nil {
		return errApplicationShutdownFailed
	}
	r.mu.Lock()
	r.hooks = append(r.hooks, hook)
	r.mu.Unlock()
	return nil
}

func (r *applicationShutdownRegistry) Shutdown(ctx context.Context) error {
	if r == nil || ctx == nil {
		return errApplicationShutdownFailed
	}
	r.shutdownMu.Lock()
	defer r.shutdownMu.Unlock()
	r.mu.Lock()
	hooks := append([]applicationShutdownHook(nil), r.hooks...)
	r.mu.Unlock()
	remaining := make([]applicationShutdownHook, 0, len(hooks))
	failed := false
	for index := len(hooks) - 1; index >= 0; index-- {
		if ctx.Err() != nil || hooks[index].Shutdown(ctx) != nil {
			failed = true
			remaining = append(remaining, hooks[index])
		}
	}
	r.mu.Lock()
	for left, right := 0, len(remaining)-1; left < right; left, right = left+1, right-1 {
		remaining[left], remaining[right] = remaining[right], remaining[left]
	}
	r.hooks = remaining
	r.mu.Unlock()
	if failed {
		return errApplicationShutdownFailed
	}
	return nil
}

func (r *applicationShutdownRegistry) Pending() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.hooks)
}

func shutdownRegistryWithRetry(
	ctx context.Context,
	registry *applicationShutdownRegistry,
	attemptTimeout time.Duration,
	backoff time.Duration,
) error {
	if ctx == nil || registry == nil || attemptTimeout <= 0 || backoff <= 0 {
		return errApplicationShutdownFailed
	}
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		err := registry.Shutdown(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errApplicationShutdownFailed
		}
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return errApplicationShutdownFailed
		}
	}
}

var applicationShutdowns = newApplicationShutdownRegistry()

func Shutdown(ctx context.Context) error {
	return shutdownRegistryWithRetry(
		ctx,
		applicationShutdowns,
		defaultApplicationShutdownAttemptTimeout,
		defaultApplicationShutdownRetryBackoff,
	)
}

func PendingShutdownHooks() int {
	return applicationShutdowns.Pending()
}

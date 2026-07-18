// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
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
	nextID     uint64
	hooks      map[uint64]*applicationShutdownEntry
}

type applicationShutdownEntry struct {
	id   uint64
	name string
	hook applicationShutdownHook
}

type applicationShutdownRegistration struct {
	registry *applicationShutdownRegistry
	id       uint64
}

func newApplicationShutdownRegistry() *applicationShutdownRegistry {
	return &applicationShutdownRegistry{hooks: make(map[uint64]*applicationShutdownEntry)}
}

func (r *applicationShutdownRegistry) Register(hook applicationShutdownHook) (applicationShutdownRegistration, error) {
	if r == nil || hook == nil {
		return applicationShutdownRegistration{}, errApplicationShutdownFailed
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	entry := &applicationShutdownEntry{id: r.nextID, name: applicationShutdownHookName(hook), hook: hook}
	r.hooks[entry.id] = entry
	return applicationShutdownRegistration{registry: r, id: entry.id}, nil
}

func (r *applicationShutdownRegistry) Unregister(registration applicationShutdownRegistration) bool {
	if r == nil || registration.registry != r || registration.id == 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.hooks[registration.id]; !exists {
		return false
	}
	delete(r.hooks, registration.id)
	return true
}

func (r *applicationShutdownRegistry) Shutdown(ctx context.Context) error {
	if r == nil || ctx == nil {
		return errApplicationShutdownFailed
	}
	r.shutdownMu.Lock()
	defer r.shutdownMu.Unlock()
	r.mu.Lock()
	entries := make([]*applicationShutdownEntry, 0, len(r.hooks))
	for _, entry := range r.hooks {
		entries = append(entries, entry)
	}
	r.mu.Unlock()
	sort.Slice(entries, func(left, right int) bool { return entries[left].id > entries[right].id })
	var shutdownErrors []error
	for _, entry := range entries {
		var err error
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			err = entry.hook.Shutdown(ctx)
		}
		r.mu.Lock()
		current, stillRegistered := r.hooks[entry.id]
		if err == nil && stillRegistered && current == entry {
			delete(r.hooks, entry.id)
		}
		r.mu.Unlock()
		if err != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf(
				"shutdown hook %s[%d]: %w",
				entry.name,
				entry.id,
				err,
			))
		}
	}
	if len(shutdownErrors) > 0 {
		return errors.Join(append([]error{errApplicationShutdownFailed}, shutdownErrors...)...)
	}
	return nil
}

func applicationShutdownHookName(hook applicationShutdownHook) string {
	if named, ok := hook.(interface{ ShutdownName() string }); ok {
		if name := named.ShutdownName(); name != "" {
			return name
		}
	}
	if kind := reflect.TypeOf(hook); kind != nil {
		return kind.String()
	}
	return "unknown"
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
			return errors.Join(errApplicationShutdownFailed, err, ctx.Err())
		}
		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return errors.Join(errApplicationShutdownFailed, err, ctx.Err())
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

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"sync"
	"time"
)

type managedSessionResource struct {
	mu sync.Mutex

	client       ProtocolClient
	cleanup      func() error
	clientClosed bool
	cleanupDone  bool
	completed    bool
	retryable    bool
	onCompleted  func()
	onRetryable  func(*managedSessionResource)
}

func (r *managedSessionResource) requestRetry() {
	if r == nil {
		return
	}
	r.markRetryable()
	r.mu.Lock()
	onRetryable := r.onRetryable
	r.mu.Unlock()
	if onRetryable != nil {
		onRetryable(r)
	}
}

func (r *managedSessionResource) markRetryable() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if !r.completed {
		r.retryable = true
	}
	r.mu.Unlock()
}

func (r *managedSessionResource) shouldRetry() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.retryable && !r.completed
}

func (r *managedSessionResource) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.completed {
		r.mu.Unlock()
		return nil
	}
	if !r.clientClosed {
		if r.client == nil || r.client.Close() != nil {
			r.mu.Unlock()
			return ErrSessionUnavailable
		}
		r.clientClosed = true
	}
	if !r.cleanupDone {
		if r.cleanup != nil && r.cleanup() != nil {
			r.mu.Unlock()
			return ErrSessionUnavailable
		}
		r.cleanupDone = true
	}
	r.completed = true
	onCompleted := r.onCompleted
	r.mu.Unlock()
	if onCompleted != nil {
		onCompleted()
	}
	return nil
}

type sessionResourceRetrySupervisor struct {
	options WorkdirCleanupSupervisorOptions

	mu        sync.Mutex
	queue     []*managedSessionResource
	pending   map[*managedSessionResource]struct{}
	active    int
	accepting bool
	drained   chan struct{}
	onAttempt func()
}

func newSessionResourceRetrySupervisor(
	options WorkdirCleanupSupervisorOptions,
	onAttempt func(),
) (*sessionResourceRetrySupervisor, error) {
	normalized, ok := normalizeWorkdirCleanupSupervisorOptions(options)
	if !ok {
		return nil, ErrSessionUnavailable
	}
	drained := make(chan struct{})
	close(drained)
	return &sessionResourceRetrySupervisor{
		options: normalized, pending: make(map[*managedSessionResource]struct{}),
		accepting: true, drained: drained, onAttempt: onAttempt,
	}, nil
}

func (s *sessionResourceRetrySupervisor) enqueue(resource *managedSessionResource) bool {
	if s == nil || resource == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accepting {
		return false
	}
	if _, exists := s.pending[resource]; exists {
		return true
	}
	if len(s.pending) >= s.options.QueueSize {
		return false
	}
	if s.active == 0 && len(s.queue) == 0 {
		s.drained = make(chan struct{})
	}
	s.pending[resource] = struct{}{}
	s.queue = append(s.queue, resource)
	s.startWorkersLocked()
	return true
}

func (s *sessionResourceRetrySupervisor) startWorkersLocked() {
	for s.active < s.options.MaxConcurrent && s.active < len(s.queue) {
		s.active++
		go s.worker()
	}
}

func (s *sessionResourceRetrySupervisor) worker() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.active--
			s.signalDrainedLocked()
			s.mu.Unlock()
			return
		}
		resource := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()

		backoff := s.options.InitialBackoff
		for attempt := 0; attempt < s.options.MaxAttempts; attempt++ {
			timer := time.NewTimer(backoff)
			<-timer.C
			if resource.close() == nil {
				break
			}
			if backoff < s.options.MaxBackoff {
				backoff *= 2
				if backoff > s.options.MaxBackoff {
					backoff = s.options.MaxBackoff
				}
			}
		}

		s.mu.Lock()
		delete(s.pending, resource)
		s.startWorkersLocked()
		s.mu.Unlock()
		if s.onAttempt != nil {
			s.onAttempt()
		}
	}
}

func (s *sessionResourceRetrySupervisor) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return ErrSessionUnavailable
	}
	s.mu.Lock()
	s.accepting = false
	drained := s.drained
	s.signalDrainedLocked()
	s.mu.Unlock()
	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ErrSessionUnavailable
	}
}

func (s *sessionResourceRetrySupervisor) signalDrainedLocked() {
	if s.active != 0 || len(s.queue) != 0 || len(s.pending) != 0 {
		return
	}
	select {
	case <-s.drained:
	default:
		close(s.drained)
	}
}

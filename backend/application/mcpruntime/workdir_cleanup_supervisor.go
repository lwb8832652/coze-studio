// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	defaultWorkdirCleanupQueueSize      = 128
	defaultWorkdirCleanupMaxConcurrent  = 2
	defaultWorkdirCleanupMaxAttempts    = 16
	defaultWorkdirCleanupInitialBackoff = 25 * time.Millisecond
	defaultWorkdirCleanupMaxBackoff     = 2 * time.Second
	maximumWorkdirCleanupQueueSize      = 1024
	maximumWorkdirCleanupMaxConcurrent  = 16
	maximumWorkdirCleanupMaxAttempts    = 64
)

type WorkdirCleanupSupervisorOptions struct {
	QueueSize      int
	MaxConcurrent  int
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type workdirCleanupTask struct {
	key      string
	relative string
	scan     bool
	startup  bool
}

type workdirCleanupSupervisor struct {
	manager        *SafeWorkdirManager
	attemptTimeout time.Duration
	options        WorkdirCleanupSupervisorOptions

	mu               sync.Mutex
	queue            []workdirCleanupTask
	pending          map[string]struct{}
	active           int
	accepting        bool
	drained          chan struct{}
	startupDone      chan struct{}
	startupSucceeded bool
	startupOnce      sync.Once
}

func newWorkdirCleanupSupervisor(
	manager *SafeWorkdirManager,
	attemptTimeout time.Duration,
	options WorkdirCleanupSupervisorOptions,
) (*workdirCleanupSupervisor, error) {
	options, ok := normalizeWorkdirCleanupSupervisorOptions(options)
	if manager == nil || attemptTimeout <= 0 || !ok {
		return nil, ErrSafeWorkdirInvalid
	}
	drained := make(chan struct{})
	close(drained)
	supervisor := &workdirCleanupSupervisor{
		manager: manager, attemptTimeout: attemptTimeout, options: options,
		pending: make(map[string]struct{}), accepting: true, drained: drained,
		startupDone: make(chan struct{}),
	}
	if !supervisor.enqueue(workdirCleanupTask{key: "scan", scan: true, startup: true}) {
		return nil, ErrSafeWorkdirUnavailable
	}
	return supervisor, nil
}

func normalizeWorkdirCleanupSupervisorOptions(
	options WorkdirCleanupSupervisorOptions,
) (WorkdirCleanupSupervisorOptions, bool) {
	if options.QueueSize < 0 || options.MaxConcurrent < 0 || options.MaxAttempts < 0 ||
		options.InitialBackoff < 0 || options.MaxBackoff < 0 {
		return WorkdirCleanupSupervisorOptions{}, false
	}
	if options.QueueSize == 0 {
		options.QueueSize = defaultWorkdirCleanupQueueSize
	}
	if options.MaxConcurrent == 0 {
		options.MaxConcurrent = defaultWorkdirCleanupMaxConcurrent
	}
	if options.MaxAttempts == 0 {
		options.MaxAttempts = defaultWorkdirCleanupMaxAttempts
	}
	if options.InitialBackoff == 0 {
		options.InitialBackoff = defaultWorkdirCleanupInitialBackoff
	}
	if options.MaxBackoff == 0 {
		options.MaxBackoff = defaultWorkdirCleanupMaxBackoff
	}
	if options.QueueSize > maximumWorkdirCleanupQueueSize ||
		options.MaxConcurrent > maximumWorkdirCleanupMaxConcurrent ||
		options.MaxAttempts > maximumWorkdirCleanupMaxAttempts ||
		options.InitialBackoff > options.MaxBackoff {
		return WorkdirCleanupSupervisorOptions{}, false
	}
	return options, true
}

func (s *workdirCleanupSupervisor) enqueueRelative(relative string) bool {
	if s == nil || relative == "" {
		return false
	}
	return s.enqueue(workdirCleanupTask{key: "path:" + relative, relative: relative})
}

func (s *workdirCleanupSupervisor) enqueue(task workdirCleanupTask) bool {
	if s == nil || task.key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accepting {
		return false
	}
	if _, exists := s.pending[task.key]; exists {
		return true
	}
	if len(s.pending) >= s.options.QueueSize {
		return false
	}
	if s.active == 0 && len(s.queue) == 0 {
		s.drained = make(chan struct{})
	}
	s.pending[task.key] = struct{}{}
	s.queue = append(s.queue, task)
	s.startWorkersLocked()
	return true
}

func (s *workdirCleanupSupervisor) startWorkersLocked() {
	for s.active < s.options.MaxConcurrent && s.active < len(s.queue) {
		s.active++
		go s.worker()
	}
}

func (s *workdirCleanupSupervisor) worker() {
	for {
		s.mu.Lock()
		if len(s.queue) == 0 {
			s.active--
			s.signalDrainedLocked()
			s.mu.Unlock()
			return
		}
		task := s.queue[0]
		s.queue = s.queue[1:]
		s.mu.Unlock()

		succeeded := s.run(task)

		s.mu.Lock()
		if task.startup {
			s.startupSucceeded = succeeded
			s.startupOnce.Do(func() { close(s.startupDone) })
		}
		delete(s.pending, task.key)
		s.startWorkersLocked()
		s.mu.Unlock()
	}
}

func (s *workdirCleanupSupervisor) run(task workdirCleanupTask) bool {
	backoff := s.options.InitialBackoff
	for attempt := 0; attempt < s.options.MaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), s.attemptTimeout)
		var err error
		if task.scan {
			_, recoverErr := s.manager.RecoverDirectChildren(ctx, managementSafeWorkdirPrefix)
			cleanupErr := s.manager.CleanupQuarantine(ctx)
			if (recoverErr != nil && !errors.Is(recoverErr, ErrSafeWorkdirRecoveryRejected)) || cleanupErr != nil {
				err = ErrSafeWorkdirCleanupRetryable
			}
		} else {
			err = s.manager.Delete(ctx, task.relative)
		}
		cancel()
		if err == nil {
			return true
		}
		if attempt+1 >= s.options.MaxAttempts {
			return false
		}
		timer := time.NewTimer(backoff)
		<-timer.C
		if backoff < s.options.MaxBackoff {
			backoff *= 2
			if backoff > s.options.MaxBackoff {
				backoff = s.options.MaxBackoff
			}
		}
	}
	return false
}

func (s *workdirCleanupSupervisor) WaitStartup(ctx context.Context) bool {
	if s == nil {
		return true
	}
	if ctx == nil {
		return false
	}
	select {
	case <-s.startupDone:
		s.mu.Lock()
		succeeded := s.startupSucceeded
		s.mu.Unlock()
		return succeeded
	case <-ctx.Done():
		return false
	}
}

func (s *workdirCleanupSupervisor) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return ErrSafeWorkdirCleanupRetryable
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
		return ErrSafeWorkdirCleanupRetryable
	}
}

func (s *workdirCleanupSupervisor) signalDrainedLocked() {
	if s.active != 0 || len(s.queue) != 0 || len(s.pending) != 0 {
		return
	}
	select {
	case <-s.drained:
	default:
		close(s.drained)
	}
}

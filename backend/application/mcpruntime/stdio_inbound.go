// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"sync"
	"time"
)

const (
	defaultStdioInboundMaxInflight  = 8
	defaultStdioInboundMaxTotal     = 4096
	defaultStdioInboundMaxPerWindow = 128
	defaultStdioInboundRateWindow   = time.Second
	defaultStdioInboundCloseWait    = 250 * time.Millisecond
)

type stdioInboundLimiter struct {
	mu          sync.Mutex
	closed      bool
	inflight    int
	total       int
	windowStart time.Time
	windowCount int
	idle        chan struct{}
}

func newStdioInboundLimiter() *stdioInboundLimiter {
	idle := make(chan struct{})
	close(idle)
	return &stdioInboundLimiter{idle: idle}
}

func (l *stdioInboundLimiter) tryAcquire(now time.Time) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.inflight >= defaultStdioInboundMaxInflight ||
		l.total >= defaultStdioInboundMaxTotal {
		l.closed = true
		return false
	}
	if l.windowStart.IsZero() || now.Before(l.windowStart) ||
		now.Sub(l.windowStart) >= defaultStdioInboundRateWindow {
		l.windowStart = now
		l.windowCount = 0
	}
	if l.windowCount >= defaultStdioInboundMaxPerWindow {
		l.closed = true
		return false
	}
	l.inflight++
	if l.inflight == 1 {
		l.idle = make(chan struct{})
	}
	l.total++
	l.windowCount++
	return true
}

func (l *stdioInboundLimiter) release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.inflight > 0 {
		l.inflight--
		if l.inflight == 0 {
			close(l.idle)
		}
		l.mu.Unlock()
		return
	}
	l.mu.Unlock()
}

func (l *stdioInboundLimiter) close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
}

func (l *stdioInboundLimiter) inflightCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inflight
}

func (l *stdioInboundLimiter) wait(timeout time.Duration) bool {
	if l == nil {
		return true
	}
	l.close()
	l.mu.Lock()
	idle := l.idle
	l.mu.Unlock()
	if timeout <= 0 {
		timeout = defaultStdioInboundCloseWait
	}
	select {
	case <-idle:
		return true
	case <-time.After(timeout):
		return false
	}
}

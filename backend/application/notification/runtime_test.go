// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingWorkerRunner struct {
	started          chan struct{}
	claimLoopStopped chan struct{}
	inFlightCanceled chan struct{}
	release          chan struct{}
}

func (r *blockingWorkerRunner) Run(claimLoopCtx context.Context, inFlightCtx context.Context) {
	close(r.started)
	<-claimLoopCtx.Done()
	close(r.claimLoopStopped)
	select {
	case <-r.release:
	case <-inFlightCtx.Done():
		close(r.inFlightCanceled)
	}
}

func TestRuntimeShutdownLetsInflightWorkFinishBeforeCancel(t *testing.T) {
	runner := &blockingWorkerRunner{
		started:          make(chan struct{}),
		claimLoopStopped: make(chan struct{}),
		inFlightCanceled: make(chan struct{}),
		release:          make(chan struct{}),
	}
	runtime := newRuntimeWithRunner(runner)
	runtime.Start(context.Background())
	<-runner.started

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- runtime.Shutdown(ctx)
	}()
	<-runner.claimLoopStopped
	close(runner.release)

	require.NoError(t, <-shutdownDone)
	select {
	case <-runner.inFlightCanceled:
		t.Fatal("in-flight context was canceled before the grace window elapsed")
	default:
	}
}

func TestRuntimeShutdownCancelsInflightWorkAfterGraceDeadline(t *testing.T) {
	runner := &blockingWorkerRunner{
		started:          make(chan struct{}),
		claimLoopStopped: make(chan struct{}),
		inFlightCanceled: make(chan struct{}),
		release:          make(chan struct{}),
	}
	runtime := newRuntimeWithRunner(runner)
	runtime.Start(context.Background())
	<-runner.started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, runtime.Shutdown(ctx), context.Canceled)
	<-runner.claimLoopStopped
	<-runner.inFlightCanceled
}

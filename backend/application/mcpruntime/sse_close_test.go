// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	sseSourceReadSecret  = "secret source read failure"
	sseSourceCloseSecret = "secret source close failure"
)

type closeCountingPipeBody struct {
	reader     *io.PipeReader
	closeCalls atomic.Int32
}

func (b *closeCountingPipeBody) Read(destination []byte) (int, error) {
	return b.reader.Read(destination)
}

func (b *closeCountingPipeBody) Close() error {
	b.closeCalls.Add(1)
	_ = b.reader.CloseWithError(errors.New(sseSourceReadSecret))
	return errors.New(sseSourceCloseSecret)
}

func TestBoundedSSEReadCloserConcurrentClose(t *testing.T) {
	pipeReader, pipeWriter := io.Pipe()
	source := &closeCountingPipeBody{reader: pipeReader}
	body := newBoundedSSEReadCloser(source)

	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 32)
		for {
			if _, err := body.Read(buffer); err != nil {
				readDone <- err
				return
			}
		}
	}()

	streamStarted := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		payload := []byte("data: {\"status\":\"running\"}\n\n")
		first := true
		for {
			if _, err := pipeWriter.Write(payload); err != nil {
				return
			}
			if first {
				first = false
				close(streamStarted)
			}
		}
	}()

	select {
	case <-streamStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("continuous SSE stream did not start")
	}

	const closeCount = 32
	startClose := make(chan struct{})
	closeErrors := make([]error, closeCount)
	var closeGroup sync.WaitGroup
	closeGroup.Add(closeCount)
	for index := range closeErrors {
		go func(index int) {
			defer closeGroup.Done()
			<-startClose
			closeErrors[index] = body.Close()
		}(index)
	}
	close(startClose)
	closeGroup.Wait()

	for _, err := range closeErrors {
		if !errors.Is(err, ErrSessionUnavailable) {
			t.Fatalf("expected fixed close error, got %v", err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("close leaked source error: %v", err)
		}
	}
	if got := source.closeCalls.Load(); got != 1 {
		t.Fatalf("source closed %d times, want 1", got)
	}

	select {
	case err := <-readDone:
		if !errors.Is(err, ErrSessionUnavailable) {
			t.Fatalf("expected fixed read error, got %v", err)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatalf("read leaked source error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not unblock SSE Read")
	}

	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not stop the continuous SSE source")
	}
}

func TestBoundedSSEReadCloserEOFRemainsStableAfterConcurrentClose(t *testing.T) {
	source := &closeTrackingSSEBody{Reader: bytes.NewBufferString("data: ok\n\n")}
	body := newBoundedSSEReadCloser(source)
	if _, err := io.ReadAll(body); err != nil {
		t.Fatalf("read finite SSE stream: %v", err)
	}

	const closeCount = 16
	var closeGroup sync.WaitGroup
	closeGroup.Add(closeCount)
	for index := 0; index < closeCount; index++ {
		go func() {
			defer closeGroup.Done()
			if err := body.Close(); err != nil {
				t.Errorf("close finite SSE stream: %v", err)
			}
		}()
	}
	closeGroup.Wait()

	buffer := make([]byte, 1)
	if _, err := body.Read(buffer); !errors.Is(err, io.EOF) {
		t.Fatalf("expected stable EOF after Close, got %v", err)
	}
}

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type closeTrackingSSEBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingSSEBody) Close() error {
	b.closed = true
	return nil
}

func TestBoundedSSEReadCloserEnforcesTotalEventBytes(t *testing.T) {
	const lineContentBytes = 64 * 1024
	line := append([]byte("data:"), bytes.Repeat([]byte{'x'}, lineContentBytes-len("data:"))...)
	line = append(line, '\n')

	buildEvent := func(overLimit bool) []byte {
		var event bytes.Buffer
		for written := 0; written < maxSSEEventBytes; written += lineContentBytes {
			event.Write(line)
		}
		if overLimit {
			event.WriteString(":\n")
		}
		event.WriteByte('\n')
		return event.Bytes()
	}

	t.Run("exact boundary is transparent", func(t *testing.T) {
		input := buildEvent(false)
		source := &closeTrackingSSEBody{Reader: bytes.NewReader(input)}
		body := newBoundedSSEReadCloser(source)

		got, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("read boundary event: %v", err)
		}
		if !bytes.Equal(got, input) {
			t.Fatal("boundary event was not returned transparently")
		}
	})

	t.Run("multiline event over limit is withheld", func(t *testing.T) {
		source := &closeTrackingSSEBody{Reader: bytes.NewReader(buildEvent(true))}
		body := newBoundedSSEReadCloser(source)

		got, err := io.ReadAll(body)
		if !errors.Is(err, ErrSSEEventLimitExceeded) {
			t.Fatalf("expected fixed event limit error, got %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("oversized event leaked %d bytes", len(got))
		}
		if !source.closed {
			t.Fatal("oversized event did not close the response body")
		}
	})
}

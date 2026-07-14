// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bufio"
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

const maxSSEBufferedEventWireBytes = 3*maxSSEEventBytes + 2

// boundedSSEReadCloser withholds each event until all of its lines satisfy the
// shared SSE limits. Downstream parsers therefore never observe a prefix of an
// event that is later found to be oversized.
type boundedSSEReadCloser struct {
	source io.ReadCloser
	reader *bufio.Reader

	pending bytes.Buffer
	ready   bytes.Buffer

	eventBytes int
	dataLines  int
	eof        bool
	terminal   error

	closed      atomic.Bool
	closeFailed atomic.Bool
	closeOnce   sync.Once
}

func newBoundedSSEReadCloser(source io.ReadCloser) io.ReadCloser {
	return &boundedSSEReadCloser{
		source: source,
		reader: bufio.NewReaderSize(source, maxSSELineBytes+2),
	}
}

func (r *boundedSSEReadCloser) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if r.ready.Len() > 0 {
		return r.ready.Read(destination)
	}
	if r.terminal != nil {
		return 0, r.terminal
	}
	if r.eof {
		return 0, io.EOF
	}
	if r.closed.Load() {
		return 0, ErrSessionUnavailable
	}

	if err := r.loadEvent(); err != nil {
		return 0, err
	}
	if r.ready.Len() > 0 {
		return r.ready.Read(destination)
	}
	if r.eof {
		return 0, io.EOF
	}
	return 0, nil
}

func (r *boundedSSEReadCloser) Close() error {
	return r.closeSource()
}

func (r *boundedSSEReadCloser) loadEvent() error {
	for {
		line, readErr := r.reader.ReadSlice('\n')
		if readErr == bufio.ErrBufferFull {
			return r.failLimit()
		}

		if len(line) > 0 {
			content := sseLineContent(line)
			if len(content) > maxSSELineBytes ||
				len(content) > maxSSEEventBytes-r.eventBytes ||
				len(line) > maxSSEBufferedEventWireBytes-r.pending.Len() {
				return r.failLimit()
			}

			if len(content) == 0 {
				r.pending.Write(line)
				r.promoteEvent()
				if readErr == io.EOF {
					r.eof = true
				}
				return nil
			}

			if bytes.HasPrefix(content, []byte("data:")) {
				if r.dataLines >= maxSSEDataLines {
					return r.failLimit()
				}
				r.dataLines++
			}
			r.eventBytes += len(content)
			r.pending.Write(line)
		}

		switch readErr {
		case nil:
			continue
		case io.EOF:
			r.eof = true
			if r.pending.Len() > 0 {
				r.promoteEvent()
				return nil
			}
			return io.EOF
		default:
			r.terminal = ErrSessionUnavailable
			_ = r.closeSource()
			r.pending = bytes.Buffer{}
			return r.terminal
		}
	}
}

func (r *boundedSSEReadCloser) promoteEvent() {
	r.ready, r.pending = r.pending, r.ready
	r.pending.Reset()
	r.eventBytes = 0
	r.dataLines = 0
}

func (r *boundedSSEReadCloser) failLimit() error {
	r.pending = bytes.Buffer{}
	r.ready = bytes.Buffer{}
	r.terminal = ErrSSEEventLimitExceeded
	_ = r.closeSource()
	return r.terminal
}

func (r *boundedSSEReadCloser) closeSource() error {
	r.closed.Store(true)
	r.closeOnce.Do(func() {
		if err := r.source.Close(); err != nil {
			r.closeFailed.Store(true)
		}
	})
	if r.closeFailed.Load() {
		return ErrSessionUnavailable
	}
	return nil
}

func sseLineContent(line []byte) []byte {
	content := line
	if len(content) > 0 && content[len(content)-1] == '\n' {
		content = content[:len(content)-1]
	}
	if len(content) > 0 && content[len(content)-1] == '\r' {
		content = content[:len(content)-1]
	}
	return content
}
